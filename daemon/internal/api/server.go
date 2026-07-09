// Package api exposes the daemon's state to the local dashboard. The
// frontend holds no network logic: everything it renders comes from here.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"bitstall/daemon/internal/bitchat"
	"bitstall/daemon/internal/crypto"
	"bitstall/daemon/internal/geo"
	"bitstall/daemon/internal/mesh"
	"bitstall/daemon/internal/model"
	"bitstall/daemon/internal/store"
)

// bcPrefix distinguishes Bitchat (Nostr gift-wrap) DM peers from local mesh
// (Noise) peers in the unified /api/chats surface, since both are 64-hex keys.
const bcPrefix = "bc:"

type Server struct {
	Identity *crypto.Identity
	Sessions *crypto.Manager
	Store    *store.Store
	Locator  *geo.Locator
	Engine   *mesh.Engine
	Bitchat  *bitchat.Client
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/listings", s.listListings)
	mux.HandleFunc("POST /api/listings", s.createListing)
	mux.HandleFunc("POST /api/location", s.setLocation)
	mux.HandleFunc("GET /api/channel", s.getChannel)
	mux.HandleFunc("POST /api/channel", s.postChannel)
	mux.HandleFunc("POST /api/config", s.setConfig)
	mux.HandleFunc("GET /api/chats", s.listChats)
	mux.HandleFunc("GET /api/chats/{peer}", s.chatHistory)
	mux.HandleFunc("POST /api/chats/{peer}", s.sendChat)
	return cors(mux)
}

// cors lets the localhost dashboard (a different port) talk to the daemon.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	lat, lon := s.Locator.Position()
	writeJSON(w, http.StatusOK, map[string]any{
		"peers":   s.Engine.PeerCount(),
		"geohash": s.Locator.Geohash(),
		"location": map[string]float64{
			"lat": lat,
			"lon": lon,
		},
		"identity": map[string]string{
			"noisePub": s.Identity.PublicKeyHex(),
			"nostrPub": s.Identity.NostrPub,
		},
		"transports": s.Engine.TransportStatus(),
		"bitchat":    s.Bitchat.Status(),
	})
}

type apiListing struct {
	model.Listing
	Mine bool `json:"mine"`
}

func (s *Server) listListings(w http.ResponseWriter, r *http.Request) {
	self := s.Identity.PublicKeyHex()
	listings := s.Store.List("")
	out := make([]apiListing, 0, len(listings))
	for _, l := range listings {
		out = append(out, apiListing{Listing: l, Mine: l.SellerKey == self})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createListing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Price string `json:"price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	l, err := model.NewListing(req.Title, req.Price, s.Identity.PublicKeyHex(), s.Locator.Geohash(), time.Now().Unix())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Store.Put(l)

	// Local Bitstall mesh (BLE / multicast) …
	body, _ := json.Marshal(l)
	s.Engine.PublishListing(body)
	// … and the Bitchat geohash channel over Nostr, so real Bitchat users
	// standing in this geohash see the listing.
	if err := s.Bitchat.PublishListing(l); err != nil {
		log.Printf("api: publish listing to bitchat: %v", err)
	}
	writeJSON(w, http.StatusCreated, apiListing{Listing: l, Mine: true})
}

func (s *Server) setLocation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		writeError(w, http.StatusBadRequest, "coordinates out of range")
		return
	}
	gh := s.Locator.Set(req.Lat, req.Lon)
	// Rejoin the Bitchat channel for the new location.
	if err := s.Bitchat.SetGeohash(gh); err != nil {
		log.Printf("api: bitchat rejoin %s: %v", gh, err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"geohash": gh})
}

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request) {
	msgs := s.Bitchat.Channel()
	if msgs == nil {
		msgs = []bitchat.ChannelMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) postChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if err := s.Bitchat.SendChannelMessage(strings.TrimSpace(req.Text)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

func (s *Server) setConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if n := strings.TrimSpace(req.Nickname); n != "" {
		s.Bitchat.SetNickname(n)
	}
	writeJSON(w, http.StatusOK, map[string]string{"nickname": s.Bitchat.Status().Nickname})
}

// unifiedChat is one conversation across either transport.
type unifiedChat struct {
	Peer          string `json:"peer"`
	Transport     string `json:"transport"` // "mesh" | "bitchat"
	Messages      int    `json:"messages"`
	LastTimestamp int64  `json:"lastTimestamp"`
	Established   bool   `json:"established"`
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	out := []unifiedChat{}
	for _, c := range s.Sessions.Chats() {
		out = append(out, unifiedChat{
			Peer: c.Peer, Transport: "mesh", Messages: c.Messages,
			LastTimestamp: c.LastTimestamp, Established: c.Established,
		})
	}
	for _, c := range s.Bitchat.Chats() {
		out = append(out, unifiedChat{
			Peer: bcPrefix + c.Peer, Transport: "bitchat", Messages: c.Messages,
			LastTimestamp: c.LastTimestamp, Established: true,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// unifiedMessage matches the shape the dashboard already renders.
type unifiedMessage struct {
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Outgoing  bool   `json:"outgoing"`
}

func (s *Server) chatHistory(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	out := []unifiedMessage{}
	if pub, ok := strings.CutPrefix(peer, bcPrefix); ok {
		for _, m := range s.Bitchat.History(pub) {
			from := pub
			if m.Outgoing {
				from = s.Bitchat.Status().SelfPub
			}
			out = append(out, unifiedMessage{From: from, Text: m.Text, Timestamp: m.Ts, Outgoing: m.Outgoing})
		}
	} else {
		for _, m := range s.Sessions.History(peer) {
			out = append(out, unifiedMessage{From: m.From, Text: m.Text, Timestamp: m.Timestamp, Outgoing: m.Outgoing})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) sendChat(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	text := strings.TrimSpace(req.Text)

	// Bitchat DM (Nostr gift wrap).
	if pub, ok := strings.CutPrefix(peer, bcPrefix); ok {
		if err := s.Bitchat.SendDM(pub, text); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
		return
	}

	// Local mesh Noise chat.
	outs, status, err := s.Sessions.SendChat(peer, text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, o := range outs {
		s.Engine.SendChat(o.To, o.Kind, o.Data)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": status})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

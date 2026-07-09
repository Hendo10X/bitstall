// Package mesh implements the gossip engine: packets propagate device to
// device, a Bloom filter stops endless packet bouncing, and pluggable
// transports carry the bytes (UDP multicast for LAN dev, BLE for the field).
package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// Packet is the compact envelope blasted into the mesh. Short JSON keys keep
// it lean for size-constrained BLE frames.
type Packet struct {
	ID      string          `json:"i"`            // dedup key for the Bloom filter
	Type    string          `json:"t"`            // "listing" | "chat"
	Kind    string          `json:"k,omitempty"`  // chat sub-kind: hs1/hs2/hs3/msg
	Geohash string          `json:"g,omitempty"`  // channel scoping
	From    string          `json:"f,omitempty"`  // sender noise pub (hex)
	To      string          `json:"to,omitempty"` // recipient noise pub (hex), chat only
	TTL     int             `json:"ttl"`          // remaining hops
	Body    json.RawMessage `json:"b"`
}

const (
	TypeListing = "listing"
	TypeChat    = "chat"
	TypePing    = "ping" // presence beacon, never relayed
	DefaultTTL  = 7

	presenceInterval = 20 * time.Second
	presenceWindow   = 75 * time.Second
)

// Transport moves raw packet bytes. Implementations must call the receive
// callback for every frame heard from other nodes.
type Transport interface {
	Name() string
	Start(onReceive func([]byte)) error
	Broadcast([]byte) error
	Active() bool
	PeerCount() int
	Stop()
}

// Engine routes packets between transports and the application, deduping
// with an optimized Bloom filter to prevent packet bouncing and battery drain.
type Engine struct {
	mu         sync.Mutex
	seen       *bloom.BloomFilter
	peers      map[string]time.Time // noise pub -> last heard
	transports []Transport
	selfPub    string
	geohash    func() string
	stop       chan struct{}

	OnListing func(body json.RawMessage)
	OnChat    func(from, kind string, body []byte)
}

func NewEngine(selfPub string, geohash func() string) *Engine {
	return &Engine{
		// ~10k packets at 1% false-positive is plenty for a local mesh; a
		// false positive only means one dropped rebroadcast.
		seen:    bloom.NewWithEstimates(10000, 0.01),
		peers:   make(map[string]time.Time),
		selfPub: selfPub,
		geohash: geohash,
		stop:    make(chan struct{}),
	}
}

func (e *Engine) AddTransport(t Transport) {
	e.transports = append(e.transports, t)
}

func (e *Engine) Start() {
	for _, t := range e.transports {
		if err := t.Start(e.receive); err != nil {
			log.Printf("mesh: transport %s unavailable: %v", t.Name(), err)
		} else {
			log.Printf("mesh: transport %s up", t.Name())
		}
	}
	go e.presenceLoop()
}

func (e *Engine) Stop() {
	close(e.stop)
	for _, t := range e.transports {
		t.Stop()
	}
}

// presenceLoop announces this node so peers can count neighbors even when
// no listings are flowing. TTL 1: presence is strictly one hop.
func (e *Engine) presenceLoop() {
	tick := time.NewTicker(presenceInterval)
	defer tick.Stop()
	for {
		// The nonce keeps successive pings distinct under the Bloom filter.
		body := json.RawMessage(fmt.Sprintf(`{"n":%d}`, time.Now().UnixNano()))
		e.send(Packet{Type: TypePing, From: e.selfPub, TTL: 1, Body: body})
		select {
		case <-tick.C:
		case <-e.stop:
			return
		}
	}
}

// PublishListing gossips a listing payload on the current geohash channel.
func (e *Engine) PublishListing(body []byte) {
	e.send(Packet{
		Type:    TypeListing,
		Geohash: e.geohash(),
		From:    e.selfPub,
		TTL:     DefaultTTL,
		Body:    body,
	})
}

// SendChat routes an encrypted chat frame toward a peer. Chat frames ride
// the same gossip flood but are only surfaced by the addressed node.
func (e *Engine) SendChat(to, kind string, data []byte) {
	body, _ := json.Marshal(hex.EncodeToString(data))
	e.send(Packet{
		Type: TypeChat,
		Kind: kind,
		From: e.selfPub,
		To:   to,
		TTL:  DefaultTTL,
		Body: body,
	})
}

func (e *Engine) send(p Packet) {
	p.ID = packetID(p)
	e.markSeen(p.ID)
	raw, err := json.Marshal(p)
	if err != nil {
		log.Printf("mesh: marshal: %v", err)
		return
	}
	e.broadcast(raw)
}

func (e *Engine) broadcast(raw []byte) {
	for _, t := range e.transports {
		if !t.Active() {
			continue
		}
		if err := t.Broadcast(raw); err != nil {
			log.Printf("mesh: %s broadcast: %v", t.Name(), err)
		}
	}
}

func (e *Engine) receive(raw []byte) {
	var p Packet
	if err := json.Unmarshal(raw, &p); err != nil {
		return // garbage on the channel; drop silently
	}
	if p.ID == "" || p.From == e.selfPub {
		return
	}
	if e.alreadySeen(p.ID) {
		return // Bloom filter: stop the bounce
	}

	e.mu.Lock()
	e.peers[p.From] = time.Now()
	e.mu.Unlock()

	// Surface to the application layer.
	switch p.Type {
	case TypeListing:
		// Listings are scoped to our geohash channel; foreign-channel
		// listings still relay (the mesh may bridge cells) but aren't stored.
		if e.OnListing != nil && p.Geohash == e.geohash() {
			e.OnListing(p.Body)
		}
	case TypeChat:
		if p.To == e.selfPub && e.OnChat != nil {
			var hexData string
			if err := json.Unmarshal(p.Body, &hexData); err == nil {
				if data, err := hex.DecodeString(hexData); err == nil {
					e.OnChat(p.From, p.Kind, data)
				}
			}
			return // addressed to us: consume, don't relay
		}
	}

	// Multi-hop relay.
	if p.TTL > 1 {
		p.TTL--
		if relayed, err := json.Marshal(p); err == nil {
			e.broadcast(relayed)
		}
	}
}

func (e *Engine) markSeen(id string) {
	e.mu.Lock()
	e.seen.AddString(id)
	e.mu.Unlock()
}

func (e *Engine) alreadySeen(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.seen.TestString(id) {
		return true
	}
	e.seen.AddString(id)
	return false
}

// PeerCount counts nodes heard gossiping recently, plus BLE-scanned
// neighbors that aren't yet exchanging payloads.
func (e *Engine) PeerCount() int {
	e.mu.Lock()
	n := 0
	now := time.Now()
	for pub, seen := range e.peers {
		if now.Sub(seen) <= presenceWindow {
			n++
		} else {
			delete(e.peers, pub)
		}
	}
	e.mu.Unlock()
	for _, t := range e.transports {
		n += t.PeerCount()
	}
	return n
}

// TransportStatus reports which transports are live, keyed by name.
func (e *Engine) TransportStatus() map[string]bool {
	out := make(map[string]bool, len(e.transports))
	for _, t := range e.transports {
		out[t.Name()] = t.Active()
	}
	return out
}

// packetID hashes the identifying content; TTL is excluded so relayed copies
// dedup correctly.
func packetID(p Packet) string {
	h := sha256.New()
	h.Write([]byte(p.Type + "|" + p.Kind + "|" + p.From + "|" + p.To + "|" + p.Geohash + "|"))
	h.Write(p.Body)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

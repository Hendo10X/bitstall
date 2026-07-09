package bitchat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	nostr "github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"

	"bitstall/daemon/internal/model"
)

// Nostr event kinds for Bitchat geohash channels.
const (
	kindGeohashMessage  = 20000 // public channel chat / listing
	kindGeohashPresence = 20001 // presence heartbeat
)

// evTag carries a compact Bitstall listing payload inside an otherwise plain
// channel message. Bitchat ignores unknown tags, so the human-readable Content
// is what its users see; other Bitstall nodes decode this tag into a listing.
const evTag = "ev"

// ChannelMessage is one line seen in the geohash location channel — either a
// Bitchat user's chatter or an Bitstall listing announcement.
type ChannelMessage struct {
	ID      string `json:"id"`
	Pubkey  string `json:"pubkey"`
	Nick    string `json:"nick"`
	Text    string `json:"text"`
	Ts      int64  `json:"timestamp"`
	Mine    bool   `json:"mine"`
	Listing bool   `json:"listing"` // true if it carried a structured listing
}

// DM is one decrypted private message with a geohash peer.
type DM struct {
	Text     string `json:"text"`
	Ts       int64  `json:"timestamp"`
	Outgoing bool   `json:"outgoing"`
}

// Client is the Bitchat-facing Nostr integration: one per daemon, retargeted
// as the local geohash changes.
type Client struct {
	pool   *nostr.SimplePool
	relays []string // default set, used for gift-wrap DMs
	bridge *Bridge
	dir    *GeoRelayDirectory

	powDifficulty int
	nickname      string

	OnListing func(model.Listing) // structured listing decoded from a channel event

	mu            sync.Mutex
	geohash       string
	self          GeoIdentity
	channelRelays []string // geo-closest set the Bitchat channel actually uses
	cancelSub     context.CancelFunc

	seen    map[string]bool  // event IDs already processed
	channel []ChannelMessage // recent channel feed (ring)
	dmLogs  map[string][]DM  // peer pubkey -> messages
	peers   map[string]int64 // presence pubkey -> last seen
	online  bool
	queued  int
}

const (
	channelCap    = 200
	closestRelays = 5 // Bitchat's GeoRelayDirectory default count
)

func NewClient(bridge *Bridge, relays []string, nickname string, pow int) *Client {
	return &Client{
		pool:          nostr.NewSimplePool(context.Background()),
		relays:        relays,
		bridge:        bridge,
		dir:           NewGeoRelayDirectory(relays),
		nickname:      nickname,
		powDifficulty: pow,
		seen:          make(map[string]bool),
		dmLogs:        make(map[string][]DM),
		peers:         make(map[string]int64),
	}
}

// SetGeohash retargets the client to a geohash channel, deriving the matching
// identity and (re)subscribing. Safe to call repeatedly.
func (c *Client) SetGeohash(geohash string) error {
	if geohash == "" {
		return fmt.Errorf("empty geohash")
	}
	self, err := c.bridge.Derive(geohash)
	if err != nil {
		return err
	}

	// Bitchat reads/writes each geohash on the relays closest to its center;
	// we must use the same set to be seen. Refresh the directory first.
	rctx, rcancel := context.WithTimeout(context.Background(), 15*time.Second)
	c.dir.Refresh(rctx)
	rcancel()
	channelRelays := c.dir.ClosestRelays(geohash, closestRelays)
	if len(channelRelays) == 0 {
		channelRelays = append([]string(nil), c.relays...)
	}

	c.mu.Lock()
	if c.geohash == geohash && c.cancelSub != nil {
		c.mu.Unlock()
		return nil
	}
	if c.cancelSub != nil {
		c.cancelSub()
	}
	c.geohash = geohash
	c.self = self
	c.channelRelays = channelRelays
	// A fresh channel view per location; DM history is kept across moves.
	c.channel = nil
	c.peers = make(map[string]int64)
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelSub = cancel
	dmRelays := append([]string(nil), c.relays...)
	c.mu.Unlock()

	go c.subscribeChannel(ctx, channelRelays, geohash)
	go c.subscribeDMs(ctx, dmRelays, self)
	log.Printf("bitchat: joined #%s as %s… via %d geo-relays: %s",
		geohash, self.Pub[:8], len(channelRelays), strings.Join(channelRelays, ", "))
	return nil
}

func (c *Client) subscribeChannel(ctx context.Context, relays []string, geohash string) {
	since := nostr.Timestamp(time.Now().Add(-1 * time.Hour).Unix())
	filter := nostr.Filter{
		Kinds: []int{kindGeohashMessage, kindGeohashPresence},
		Tags:  nostr.TagMap{"g": []string{geohash}},
		Since: &since,
		Limit: 100,
	}
	for ev := range c.pool.SubscribeMany(ctx, relays, filter) {
		c.markOnline()
		c.handleChannelEvent(ev.Event)
	}
}

func (c *Client) subscribeDMs(ctx context.Context, relays []string, self GeoIdentity) {
	// Gift-wrap timestamps are randomized up to two days back, so look back far.
	since := nostr.Timestamp(time.Now().Add(-49 * time.Hour).Unix())
	filter := nostr.Filter{
		Kinds: []int{kindGiftWrap},
		Tags:  nostr.TagMap{"p": []string{self.Pub}},
		Since: &since,
		Limit: 100,
	}
	for ev := range c.pool.SubscribeMany(ctx, relays, filter) {
		c.markOnline()
		c.handleGiftWrap(ev.Event, self)
	}
}

func (c *Client) handleChannelEvent(ev *nostr.Event) {
	if ev == nil || c.alreadySeen(ev.ID) {
		return
	}
	c.mu.Lock()
	self := c.self
	c.mu.Unlock()

	if ev.Kind == kindGeohashPresence {
		c.mu.Lock()
		c.peers[ev.PubKey] = int64(ev.CreatedAt)
		c.mu.Unlock()
		return
	}

	nick := firstTag(ev, "n")
	msg := ChannelMessage{
		ID:     ev.ID,
		Pubkey: ev.PubKey,
		Nick:   nick,
		Text:   ev.Content,
		Ts:     int64(ev.CreatedAt),
		Mine:   ev.PubKey == self.Pub,
	}

	// Decode a structured Bitstall listing if present.
	if raw := firstTag(ev, evTag); raw != "" {
		if l, err := decodeListing(raw); err == nil {
			msg.Listing = true
			// Trust the event's geohash tag over the embedded one.
			if g := firstTag(ev, "g"); g != "" {
				l.Geohash = g
			}
			if !msg.Mine && c.OnListing != nil {
				c.OnListing(l)
			}
		}
	}

	c.mu.Lock()
	c.channel = append(c.channel, msg)
	if len(c.channel) > channelCap {
		c.channel = c.channel[len(c.channel)-channelCap:]
	}
	c.peers[ev.PubKey] = int64(ev.CreatedAt)
	c.mu.Unlock()
}

func (c *Client) handleGiftWrap(ev *nostr.Event, self GeoIdentity) {
	if ev == nil || c.alreadySeen(ev.ID) {
		return
	}
	sender, text, ts, err := unwrapGiftWrap(*ev, self.Sec)
	if err != nil {
		return // not for us / undecryptable
	}
	c.mu.Lock()
	c.dmLogs[sender] = append(c.dmLogs[sender], DM{Text: text, Ts: ts, Outgoing: false})
	c.mu.Unlock()
	log.Printf("bitchat: DM from %s… in #%s", sender[:8], self.Geohash)
}

// PublishListing announces a listing into the current geohash channel as a
// kind-20000 event that Bitchat clients render as a chat line.
func (c *Client) PublishListing(l model.Listing) error {
	c.mu.Lock()
	self := c.self
	nick := c.nickname
	c.mu.Unlock()
	if self.Pub == "" {
		return fmt.Errorf("no geohash channel joined yet")
	}

	ev := nostr.Event{
		PubKey:    self.Pub,
		CreatedAt: nostr.Now(),
		Kind:      kindGeohashMessage,
		Tags: nostr.Tags{
			{"g", l.Geohash},
			{"n", nick},
			{evTag, encodeListing(l)},
		},
		Content: formatListing(l),
	}
	if err := c.signEvent(&ev, self.Sec); err != nil {
		return err
	}
	// Reflect our own post and mark it seen BEFORE publishing, so the relay's
	// echo (which may arrive mid-publish) is deduped instead of doubling up.
	c.mu.Lock()
	c.seen[ev.ID] = true
	c.channel = append(c.channel, ChannelMessage{
		ID: ev.ID, Pubkey: self.Pub, Nick: nick, Text: ev.Content,
		Ts: int64(ev.CreatedAt), Mine: true, Listing: true,
	})
	c.mu.Unlock()
	return c.publishChannel(ev)
}

// SendChannelMessage posts a plain chat line to the geohash channel (no
// listing payload) — lets the operator talk in the Bitchat room directly.
func (c *Client) SendChannelMessage(text string) error {
	c.mu.Lock()
	self := c.self
	nick := c.nickname
	c.mu.Unlock()
	if self.Pub == "" {
		return fmt.Errorf("no geohash channel joined yet")
	}
	ev := nostr.Event{
		PubKey:    self.Pub,
		CreatedAt: nostr.Now(),
		Kind:      kindGeohashMessage,
		Tags:      nostr.Tags{{"g", self.Geohash}, {"n", nick}},
		Content:   text,
	}
	if err := c.signEvent(&ev, self.Sec); err != nil {
		return err
	}
	c.mu.Lock()
	c.seen[ev.ID] = true
	c.channel = append(c.channel, ChannelMessage{
		ID: ev.ID, Pubkey: self.Pub, Nick: nick, Text: text,
		Ts: int64(ev.CreatedAt), Mine: true,
	})
	c.mu.Unlock()
	return c.publishChannel(ev)
}

// SendDM gift-wraps a private message to a geohash peer (their per-geohash
// pubkey, as seen in the channel) and publishes it.
func (c *Client) SendDM(peerPub, text string) error {
	c.mu.Lock()
	self := c.self
	c.mu.Unlock()
	if self.Sec == "" {
		return fmt.Errorf("no geohash channel joined yet")
	}
	wrap, err := giftWrap(self.Sec, self.Pub, peerPub, text)
	if err != nil {
		return err
	}
	// DMs ride the default gift-wrap-capable relay set, like Bitchat.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := c.publish(ctx, wrap, c.relays); err != nil {
		return err
	}
	c.mu.Lock()
	c.dmLogs[peerPub] = append(c.dmLogs[peerPub], DM{Text: text, Ts: time.Now().Unix(), Outgoing: true})
	c.mu.Unlock()
	return nil
}

// ReannounceListing re-publishes a listing to the channel WITHOUT re-adding it
// to the local feed. Geohash messages are ephemeral (relays don't store them),
// so periodic re-announcement is how a Bitchat user who joins the channel
// later still sees the listing arrive live.
func (c *Client) ReannounceListing(l model.Listing) error {
	c.mu.Lock()
	self := c.self
	nick := c.nickname
	c.mu.Unlock()
	if self.Pub == "" || self.Geohash != l.Geohash {
		return nil // not currently on this listing's channel
	}
	ev := nostr.Event{
		PubKey:    self.Pub,
		CreatedAt: nostr.Now(),
		Kind:      kindGeohashMessage,
		Tags: nostr.Tags{
			{"g", l.Geohash},
			{"n", nick},
			{evTag, encodeListing(l)},
		},
		Content: formatListing(l),
	}
	if err := c.signEvent(&ev, self.Sec); err != nil {
		return err
	}
	c.mu.Lock()
	c.seen[ev.ID] = true // our own echo shouldn't re-enter the feed
	c.mu.Unlock()
	return c.publishChannel(ev)
}

// signEvent applies optional NIP-13 proof-of-work and signs, fixing the
// event ID. Reflect + dedup happens between this and publishSigned.
func (c *Client) signEvent(ev *nostr.Event, sec string) error {
	if c.powDifficulty > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if tag, err := nip13.DoWork(ctx, *ev, c.powDifficulty); err == nil {
			ev.Tags = append(ev.Tags, tag)
		}
		cancel()
	}
	return ev.Sign(sec)
}

// publishChannel sends a geohash-channel event on the geo-closest relays the
// Bitchat channel actually uses.
func (c *Client) publishChannel(ev nostr.Event) error {
	c.mu.Lock()
	relays := append([]string(nil), c.channelRelays...)
	c.mu.Unlock()
	if len(relays) == 0 {
		relays = c.relays
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return c.publish(ctx, ev, relays)
}

func (c *Client) publish(ctx context.Context, ev nostr.Event, relays []string) error {
	var accepted bool
	var lastErr error
	for res := range c.pool.PublishMany(ctx, relays, ev) {
		if res.Error == nil {
			accepted = true
		} else {
			lastErr = res.Error
		}
	}
	if accepted {
		c.markOnline()
		return nil
	}
	c.mu.Lock()
	c.online = false
	c.queued++
	c.mu.Unlock()
	if lastErr == nil {
		lastErr = fmt.Errorf("no relay accepted the event")
	}
	return lastErr
}

func (c *Client) alreadySeen(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen[id] {
		return true
	}
	c.seen[id] = true
	return false
}

func (c *Client) markOnline() {
	c.mu.Lock()
	c.online = true
	c.mu.Unlock()
}

// ── Read models for the API ──────────────────────────────────────────

func (c *Client) Channel() []ChannelMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ChannelMessage, len(c.channel))
	copy(out, c.channel)
	sort.Slice(out, func(i, j int) bool { return out[i].Ts > out[j].Ts })
	return out
}

// DMChat summarizes a private conversation for the dashboard.
type DMChat struct {
	Peer          string `json:"peer"`
	Messages      int    `json:"messages"`
	LastTimestamp int64  `json:"lastTimestamp"`
}

func (c *Client) Chats() []DMChat {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]DMChat, 0, len(c.dmLogs))
	for peer, log := range c.dmLogs {
		var last int64
		if len(log) > 0 {
			last = log[len(log)-1].Ts
		}
		out = append(out, DMChat{Peer: peer, Messages: len(log), LastTimestamp: last})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTimestamp > out[j].LastTimestamp })
	return out
}

func (c *Client) History(peer string) []DM {
	c.mu.Lock()
	defer c.mu.Unlock()
	log := c.dmLogs[peer]
	out := make([]DM, len(log))
	copy(out, log)
	sort.Slice(out, func(i, j int) bool { return out[i].Ts < out[j].Ts })
	return out
}

// Status reports the Bitchat integration state for the dashboard.
type Status struct {
	Geohash      string   `json:"geohash"`
	SelfPub      string   `json:"selfPub"`
	Nickname     string   `json:"nickname"`
	Online       bool     `json:"online"`
	Queued       int      `json:"queued"`
	Participants int      `json:"participants"`
	Relays       []string `json:"relays"`
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Count peers seen within the presence window.
	cutoff := time.Now().Add(-15 * time.Minute).Unix()
	n := 0
	for pub, ts := range c.peers {
		if ts >= cutoff && pub != c.self.Pub {
			n++
		}
	}
	relays := c.channelRelays
	if len(relays) == 0 {
		relays = c.relays
	}
	return Status{
		Geohash:      c.geohash,
		SelfPub:      c.self.Pub,
		Nickname:     c.nickname,
		Online:       c.online,
		Queued:       c.queued,
		Participants: n,
		Relays:       relays,
	}
}

func (c *Client) SetNickname(nick string) {
	c.mu.Lock()
	c.nickname = nick
	c.mu.Unlock()
}

func (c *Client) Stop() {
	c.mu.Lock()
	if c.cancelSub != nil {
		c.cancelSub()
	}
	c.mu.Unlock()
	c.pool.Close("shutdown")
}

// ── Listing encoding ────────────────────────────────────────────────

func formatListing(l model.Listing) string {
	return fmt.Sprintf("FOR SALE: %s · %s · DM to buy · via bitstall", l.Title, l.Price)
}

// encodeListing packs a listing into a compact base64url JSON string for the
// ev tag. Kept tiny so the overall event stays BLE/relay friendly.
func encodeListing(l model.Listing) string {
	raw, _ := json.Marshal(l)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeListing(s string) (model.Listing, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return model.Listing{}, err
	}
	var l model.Listing
	if err := json.Unmarshal(raw, &l); err != nil {
		return model.Listing{}, err
	}
	return l, l.Validate()
}

func firstTag(ev *nostr.Event, name string) string {
	for _, t := range ev.Tags {
		if len(t) >= 2 && t[0] == name {
			return t[1]
		}
	}
	return ""
}

package crypto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flynn/noise"
)

// Chat packet kinds carried inside mesh envelopes. XX is a 3-message
// handshake; "msg" is post-handshake transport ciphertext.
const (
	KindHS1 = "hs1"
	KindHS2 = "hs2"
	KindHS3 = "hs3"
	KindMsg = "msg"
)

// Out is a frame the session layer wants sent to a peer. The mesh layer is
// responsible for wrapping and routing it.
type Out struct {
	Kind string
	To   string // peer noise pub, hex
	Data []byte
}

// Message is a decrypted chat log entry.
type Message struct {
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Outgoing  bool   `json:"outgoing"`
}

type sessionState int

const (
	stateHandshaking sessionState = iota
	stateReady
)

type session struct {
	state     sessionState
	initiator bool
	hs        *noise.HandshakeState
	send      *noise.CipherState
	recv      *noise.CipherState
	pending   []string // plaintexts queued until the handshake completes
}

// Manager runs Noise XX sessions with mesh peers and keeps the decrypted
// chat history. All mesh traffic addressed to us funnels through HandlePacket.
type Manager struct {
	mu       sync.Mutex
	identity *Identity
	sessions map[string]*session
	logs     map[string][]Message
	// nostr pubkeys learned from encrypted handshake payloads, enabling
	// NIP-17 gift-wrapped delivery over relays when the mesh breaks.
	peerNostr map[string]string
}

func NewManager(id *Identity) *Manager {
	return &Manager{
		identity:  id,
		sessions:  make(map[string]*session),
		logs:      make(map[string][]Message),
		peerNostr: make(map[string]string),
	}
}

func (m *Manager) newHandshake(initiator bool) (*noise.HandshakeState, error) {
	return noise.NewHandshakeState(noise.Config{
		CipherSuite:   CipherSuite,
		Pattern:       noise.HandshakeXX,
		Initiator:     initiator,
		StaticKeypair: m.identity.NoiseStatic,
	})
}

// SendChat encrypts text for peer, starting an XX handshake first if no
// session exists. Returns frames to transmit and whether the message went
// out now ("sent") or is queued behind the handshake ("queued").
func (m *Manager) SendChat(peer, text string) ([]Out, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[peer]
	if ok && sess.state == stateReady {
		ct, err := sess.send.Encrypt(nil, nil, []byte(text))
		if err != nil {
			return nil, "", fmt.Errorf("encrypt: %w", err)
		}
		m.appendLog(peer, Message{From: m.identity.PublicKeyHex(), Text: text, Timestamp: time.Now().Unix(), Outgoing: true})
		return []Out{{Kind: KindMsg, To: peer, Data: ct}}, "sent", nil
	}

	if ok { // handshake already in flight
		sess.pending = append(sess.pending, text)
		return nil, "queued", nil
	}

	hs, err := m.newHandshake(true)
	if err != nil {
		return nil, "", err
	}
	// XX message 1 carries no payload: nothing is encrypted yet.
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, "", err
	}
	m.sessions[peer] = &session{state: stateHandshaking, initiator: true, hs: hs, pending: []string{text}}
	return []Out{{Kind: KindHS1, To: peer, Data: msg1}}, "queued", nil
}

// HandlePacket processes an inbound chat frame from peer. It returns any
// frames to send back (handshake replies, flushed queue).
func (m *Manager) HandlePacket(peer, kind string, data []byte) ([]Out, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch kind {
	case KindHS1:
		return m.onHS1(peer, data)
	case KindHS2:
		return m.onHS2(peer, data)
	case KindHS3:
		return m.onHS3(peer, data)
	case KindMsg:
		return nil, m.onMsg(peer, data)
	default:
		return nil, fmt.Errorf("unknown chat kind %q", kind)
	}
}

func (m *Manager) onHS1(peer string, data []byte) ([]Out, error) {
	if existing, ok := m.sessions[peer]; ok {
		// Simultaneous open: the lexically lower pubkey keeps the initiator
		// role, the other side yields and answers as responder.
		if existing.initiator && m.identity.PublicKeyHex() < peer {
			return nil, nil // ignore their hs1; ours is in flight
		}
		if existing.state == stateReady {
			// Peer restarted and lost the session; let them re-handshake.
			delete(m.sessions, peer)
		} else {
			existing.hs = nil // yield: replaced below, pending carries over
		}
	}

	hs, err := m.newHandshake(false)
	if err != nil {
		return nil, err
	}
	if _, _, _, err := hs.ReadMessage(nil, data); err != nil {
		return nil, fmt.Errorf("hs1 read: %w", err)
	}
	// XX message 2 payload is encrypted: share our Nostr pubkey so the peer
	// can reach us over relays (NIP-17) if the physical mesh breaks.
	msg2, _, _, err := hs.WriteMessage(nil, []byte(m.identity.NostrPub))
	if err != nil {
		return nil, err
	}
	sess := &session{state: stateHandshaking, initiator: false, hs: hs}
	if existing, ok := m.sessions[peer]; ok {
		sess.pending = existing.pending
	}
	m.sessions[peer] = sess
	return []Out{{Kind: KindHS2, To: peer, Data: msg2}}, nil
}

func (m *Manager) onHS2(peer string, data []byte) ([]Out, error) {
	sess, ok := m.sessions[peer]
	if !ok || !sess.initiator || sess.state != stateHandshaking {
		return nil, errors.New("unexpected hs2")
	}
	payload, _, _, err := sess.hs.ReadMessage(nil, data)
	if err != nil {
		return nil, fmt.Errorf("hs2 read: %w", err)
	}
	if err := verifyStatic(sess.hs, peer); err != nil {
		delete(m.sessions, peer)
		return nil, err
	}
	m.peerNostr[peer] = string(payload)
	msg3, cs0, cs1, err := sess.hs.WriteMessage(nil, []byte(m.identity.NostrPub))
	if err != nil {
		return nil, err
	}
	sess.state = stateReady
	sess.send, sess.recv = cs0, cs1 // initiator sends on the first CipherState
	out := []Out{{Kind: KindHS3, To: peer, Data: msg3}}
	flushed, err := m.flushLocked(peer, sess)
	if err != nil {
		return out, err
	}
	return append(out, flushed...), nil
}

func (m *Manager) onHS3(peer string, data []byte) ([]Out, error) {
	sess, ok := m.sessions[peer]
	if !ok || sess.initiator || sess.state != stateHandshaking {
		return nil, errors.New("unexpected hs3")
	}
	payload, cs0, cs1, err := sess.hs.ReadMessage(nil, data)
	if err != nil {
		return nil, fmt.Errorf("hs3 read: %w", err)
	}
	if err := verifyStatic(sess.hs, peer); err != nil {
		delete(m.sessions, peer)
		return nil, err
	}
	m.peerNostr[peer] = string(payload)
	sess.state = stateReady
	sess.send, sess.recv = cs1, cs0 // responder sends on the second CipherState
	return m.flushLocked(peer, sess)
}

func (m *Manager) onMsg(peer string, data []byte) error {
	sess, ok := m.sessions[peer]
	if !ok || sess.state != stateReady {
		return errors.New("ciphertext for unestablished session")
	}
	pt, err := sess.recv.Decrypt(nil, nil, data)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	m.appendLog(peer, Message{From: peer, Text: string(pt), Timestamp: time.Now().Unix(), Outgoing: false})
	return nil
}

func (m *Manager) flushLocked(peer string, sess *session) ([]Out, error) {
	var out []Out
	for _, text := range sess.pending {
		ct, err := sess.send.Encrypt(nil, nil, []byte(text))
		if err != nil {
			return out, err
		}
		m.appendLog(peer, Message{From: m.identity.PublicKeyHex(), Text: text, Timestamp: time.Now().Unix(), Outgoing: true})
		out = append(out, Out{Kind: KindMsg, To: peer, Data: ct})
	}
	sess.pending = nil
	return out, nil
}

func (m *Manager) appendLog(peer string, msg Message) {
	m.logs[peer] = append(m.logs[peer], msg)
}

// verifyStatic ensures the authenticated Noise static key matches the mesh
// identity we were addressing — the whole point of "addressed strictly to
// the seller's public key".
func verifyStatic(hs *noise.HandshakeState, expectedHex string) error {
	got := hex.EncodeToString(hs.PeerStatic())
	if got != expectedHex {
		return fmt.Errorf("peer static key mismatch: claimed %s, proved %s", expectedHex, got)
	}
	return nil
}

// Chats summarizes conversations for the dashboard.
type ChatSummary struct {
	Peer          string `json:"peer"`
	Messages      int    `json:"messages"`
	LastTimestamp int64  `json:"lastTimestamp"`
	Established   bool   `json:"established"`
}

func (m *Manager) Chats() []ChatSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool)
	var out []ChatSummary
	add := func(peer string) {
		if seen[peer] {
			return
		}
		seen[peer] = true
		log := m.logs[peer]
		var last int64
		if len(log) > 0 {
			last = log[len(log)-1].Timestamp
		}
		sess, ok := m.sessions[peer]
		out = append(out, ChatSummary{
			Peer:          peer,
			Messages:      len(log),
			LastTimestamp: last,
			Established:   ok && sess.state == stateReady,
		})
	}
	for peer := range m.logs {
		add(peer)
	}
	for peer := range m.sessions {
		add(peer)
	}
	return out
}

func (m *Manager) History(peer string) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	log := m.logs[peer]
	out := make([]Message, len(log))
	copy(out, log)
	return out
}

// PeerNostrKey returns the Nostr pubkey a peer shared during the handshake,
// if any — used for NIP-17 relay fallback.
func (m *Manager) PeerNostrKey(peer string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.peerNostr[peer]
	return k, ok
}

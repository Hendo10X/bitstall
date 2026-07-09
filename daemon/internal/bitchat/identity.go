// Package bitchat makes Bitstall interoperable with the real Bitchat network
// over its Nostr geohash location channels: it derives the same per-geohash
// identities, publishes/subscribes the same kind-20000 channel events, and
// exchanges NIP-17 gift-wrapped DMs — so a listing posted here appears in a
// Bitchat client standing in the same geohash, and vice versa.
package bitchat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync"

	nostr "github.com/nbd-wtf/go-nostr"
)

// GeoIdentity is the ephemeral Nostr identity used within a single geohash
// channel. A fresh one per geohash prevents cross-location correlation.
type GeoIdentity struct {
	Geohash string
	Sec     string // hex secret key
	Pub     string // hex x-only public key
}

// Bridge derives deterministic, unlinkable per-geohash Nostr identities from a
// stable device seed. This mirrors Bitchat's NostrIdentityBridge exactly:
//
//	key material = HMAC-SHA256(deviceSeed, geohashBytes ‖ uint32_be(iteration))
//
// iterating until the bytes form a valid secp256k1 key (they effectively
// always do on the first try), with a seed+geohash SHA-256 as a final fallback.
type Bridge struct {
	seed  []byte
	mu    sync.Mutex
	cache map[string]GeoIdentity
}

func NewBridge(deviceSeed []byte) *Bridge {
	return &Bridge{seed: deviceSeed, cache: make(map[string]GeoIdentity)}
}

func (b *Bridge) Derive(geohash string) (GeoIdentity, error) {
	if geohash == "" {
		return GeoIdentity{}, errors.New("empty geohash")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if id, ok := b.cache[geohash]; ok {
		return id, nil
	}

	msg := []byte(geohash)
	for i := uint32(0); i < 10; i++ {
		mac := hmac.New(sha256.New, b.seed)
		mac.Write(msg)
		var ib [4]byte
		binary.BigEndian.PutUint32(ib[:], i)
		mac.Write(ib[:])
		skHex := hex.EncodeToString(mac.Sum(nil))
		if pk, err := nostr.GetPublicKey(skHex); err == nil {
			id := GeoIdentity{Geohash: geohash, Sec: skHex, Pub: pk}
			b.cache[geohash] = id
			return id, nil
		}
	}

	// Fallback: SHA-256(seed ‖ geohash).
	sum := sha256.Sum256(append(append([]byte{}, b.seed...), msg...))
	skHex := hex.EncodeToString(sum[:])
	pk, err := nostr.GetPublicKey(skHex)
	if err != nil {
		return GeoIdentity{}, err
	}
	id := GeoIdentity{Geohash: geohash, Sec: skHex, Pub: pk}
	b.cache[geohash] = id
	return id, nil
}

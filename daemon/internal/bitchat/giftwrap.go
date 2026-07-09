package bitchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	nostr "github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip44"
)

// NIP-17 / NIP-59 event kinds, matching Bitchat's NostrProtocol.EventKind.
const (
	kindDM       = 14   // rumor (unsigned chat message)
	kindSeal     = 13   // sealed event
	kindGiftWrap = 1059 // gift wrap
)

// giftWrap encrypts text as a NIP-17 private message from sender to recipient:
//
//	rumor (kind 14, unsigned) → seal (kind 13, signed by sender) → wrap (kind 1059, one-time key)
//
// This is byte-for-byte the scheme Bitchat uses for geohash DMs, so a wrap we
// produce opens in Bitchat and vice versa.
func giftWrap(senderSecret, senderPub, recipientPub, text string) (nostr.Event, error) {
	now := nostr.Now()

	rumor := nostr.Event{
		PubKey:    senderPub,
		CreatedAt: now,
		Kind:      kindDM,
		Tags:      nostr.Tags{{"p", recipientPub}},
		Content:   text,
	}
	rumor.ID = rumor.GetID()
	rumorJSON, err := json.Marshal(rumor)
	if err != nil {
		return nostr.Event{}, err
	}

	sealKey, err := nip44.GenerateConversationKey(recipientPub, senderSecret)
	if err != nil {
		return nostr.Event{}, err
	}
	sealed, err := nip44.Encrypt(string(rumorJSON), sealKey)
	if err != nil {
		return nostr.Event{}, err
	}
	seal := nostr.Event{
		PubKey:    senderPub,
		CreatedAt: skew(now),
		Kind:      kindSeal,
		Tags:      nostr.Tags{},
		Content:   sealed,
	}
	if err := seal.Sign(senderSecret); err != nil {
		return nostr.Event{}, err
	}
	sealJSON, err := json.Marshal(seal)
	if err != nil {
		return nostr.Event{}, err
	}

	ephemeralSecret := nostr.GeneratePrivateKey()
	ephemeralPub, err := nostr.GetPublicKey(ephemeralSecret)
	if err != nil {
		return nostr.Event{}, err
	}
	wrapKey, err := nip44.GenerateConversationKey(recipientPub, ephemeralSecret)
	if err != nil {
		return nostr.Event{}, err
	}
	wrapped, err := nip44.Encrypt(string(sealJSON), wrapKey)
	if err != nil {
		return nostr.Event{}, err
	}
	wrap := nostr.Event{
		PubKey:    ephemeralPub,
		CreatedAt: skew(now),
		Kind:      kindGiftWrap,
		Tags:      nostr.Tags{{"p", recipientPub}},
		Content:   wrapped,
	}
	if err := wrap.Sign(ephemeralSecret); err != nil {
		return nostr.Event{}, err
	}
	return wrap, nil
}

// unwrapGiftWrap reverses giftWrap for a wrap addressed to us. It returns the
// authenticated sender pubkey (the seal signer), the plaintext, and the real
// message timestamp (from the rumor, not the randomized wrap).
func unwrapGiftWrap(wrap nostr.Event, recipientSecret string) (senderPub, text string, ts int64, err error) {
	wrapKey, err := nip44.GenerateConversationKey(wrap.PubKey, recipientSecret)
	if err != nil {
		return "", "", 0, err
	}
	sealJSON, err := nip44.Decrypt(wrap.Content, wrapKey)
	if err != nil {
		return "", "", 0, fmt.Errorf("open wrap: %w", err)
	}
	var seal nostr.Event
	if err := json.Unmarshal([]byte(sealJSON), &seal); err != nil {
		return "", "", 0, err
	}
	if seal.Kind != kindSeal {
		return "", "", 0, errors.New("inner event is not a seal")
	}
	// The seal must be validly signed by its author, otherwise the sender is
	// unauthenticated.
	if ok, _ := seal.CheckSignature(); !ok {
		return "", "", 0, errors.New("seal signature invalid")
	}
	sealKey, err := nip44.GenerateConversationKey(seal.PubKey, recipientSecret)
	if err != nil {
		return "", "", 0, err
	}
	rumorJSON, err := nip44.Decrypt(seal.Content, sealKey)
	if err != nil {
		return "", "", 0, fmt.Errorf("open seal: %w", err)
	}
	var rumor nostr.Event
	if err := json.Unmarshal([]byte(rumorJSON), &rumor); err != nil {
		return "", "", 0, err
	}
	// The rumor's claimed author must be the party that signed the seal.
	if rumor.PubKey != seal.PubKey {
		return "", "", 0, errors.New("rumor pubkey does not match seal signer")
	}
	return seal.PubKey, rumor.Content, int64(rumor.CreatedAt), nil
}

// skew randomizes timestamps up to two days into the past per NIP-59, to
// frustrate traffic analysis.
func skew(now nostr.Timestamp) nostr.Timestamp {
	return now - nostr.Timestamp(rand.Int63n(int64((48*time.Hour).Seconds())))
}

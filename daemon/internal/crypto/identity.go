// Package crypto owns key material and the Noise Protocol sessions used for
// encrypted buyer/seller negotiations.
package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flynn/noise"
	nostr "github.com/nbd-wtf/go-nostr"
)

// CipherSuite matches Bitchat's Noise stack: Curve25519 + ChaChaPoly + SHA256.
var CipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// Identity is the daemon's persistent key material. The Noise static key is
// the vendor's public identity on the mesh (the `sellerKey` in listings);
// the Nostr key signs relay events for the internet fallback path.
type Identity struct {
	NoiseStatic noise.DHKey
	NostrSecret string
	NostrPub    string
	// DeviceSeed is stable secret material from which unlinkable per-geohash
	// Nostr identities are derived, matching Bitchat's NostrIdentityBridge.
	DeviceSeed []byte
}

type identityFile struct {
	NoisePriv   string `json:"noisePriv"`
	NoisePub    string `json:"noisePub"`
	NostrSecret string `json:"nostrSecret"`
	DeviceSeed  string `json:"deviceSeed"`
}

// LoadOrCreate reads the identity from dataDir, generating and persisting a
// fresh one on first run.
func LoadOrCreate(dataDir string) (*Identity, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "identity.json")

	if raw, err := os.ReadFile(path); err == nil {
		var f identityFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("corrupt identity file %s: %w", path, err)
		}
		priv, err := hex.DecodeString(f.NoisePriv)
		if err != nil {
			return nil, err
		}
		pub, err := hex.DecodeString(f.NoisePub)
		if err != nil {
			return nil, err
		}
		pubKey, err := nostr.GetPublicKey(f.NostrSecret)
		if err != nil {
			return nil, err
		}
		seed, err := hex.DecodeString(f.DeviceSeed)
		if err != nil || len(seed) != 32 {
			// Older identity files predate the device seed: mint one and persist.
			seed = make([]byte, 32)
			if _, err := rand.Read(seed); err != nil {
				return nil, err
			}
			f.DeviceSeed = hex.EncodeToString(seed)
			if raw, err := json.MarshalIndent(f, "", "  "); err == nil {
				_ = os.WriteFile(path, raw, 0o600)
			}
		}
		return &Identity{
			NoiseStatic: noise.DHKey{Private: priv, Public: pub},
			NostrSecret: f.NostrSecret,
			NostrPub:    pubKey,
			DeviceSeed:  seed,
		}, nil
	}

	static, err := CipherSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, err
	}
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return nil, err
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}

	f := identityFile{
		NoisePriv:   hex.EncodeToString(static.Private),
		NoisePub:    hex.EncodeToString(static.Public),
		NostrSecret: sk,
		DeviceSeed:  hex.EncodeToString(seed),
	}
	raw, _ := json.MarshalIndent(f, "", "  ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	return &Identity{NoiseStatic: static, NostrSecret: sk, NostrPub: pk, DeviceSeed: seed}, nil
}

// PublicKeyHex is the mesh-facing vendor identity.
func (id *Identity) PublicKeyHex() string {
	return hex.EncodeToString(id.NoiseStatic.Public)
}

// Package geo translates GPS coordinates into the dynamic geohash channel
// that scopes both BLE broadcasts and Nostr fallback syncs.
package geo

import (
	"sync"

	"github.com/mmcloughlin/geohash"
)

// DefaultPrecision 5 ≈ 4.9km x 4.9km cell — Bitchat's "city" geohash level.
// Bitchat channel levels: block=7, neighborhood=6, city=5, province=4, region=2.
const DefaultPrecision = 5

// Locator holds the daemon's current position. Desktop hardware rarely has a
// GPS receiver, so position is fed in via the API (or a default) and the
// geohash is derived from it. The geohash can also be pinned to an explicit
// value — useful for matching the exact channel string a Bitchat client shows.
type Locator struct {
	mu        sync.RWMutex
	lat, lon  float64
	precision uint
	pinned    bool
	geohash   string
}

func NewLocator(lat, lon float64, precision uint) *Locator {
	if precision == 0 {
		precision = DefaultPrecision
	}
	l := &Locator{precision: precision}
	l.Set(lat, lon)
	return l
}

// Set updates the position and recomputes the geohash channel (unless pinned).
func (l *Locator) Set(lat, lon float64) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lat, l.lon = lat, lon
	if !l.pinned {
		l.geohash = geohash.EncodeWithPrecision(lat, lon, l.precision)
	}
	return l.geohash
}

// Pin forces the geohash channel to an explicit value, overriding lat/lon
// derivation until changed.
func (l *Locator) Pin(gh string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pinned = true
	l.geohash = gh
}

func (l *Locator) Geohash() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.geohash
}

func (l *Locator) Position() (lat, lon float64) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lat, l.lon
}

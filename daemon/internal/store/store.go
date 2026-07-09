// Package store keeps the in-memory listing inventory with TTL expiry.
package store

import (
	"sort"
	"sync"
	"time"

	"bitstall/daemon/internal/model"
)

// Store is a thread-safe in-memory inventory. Listings expire after TTL so
// stale offers fall out of the mesh naturally.
type Store struct {
	mu       sync.RWMutex
	listings map[string]model.Listing
	ttl      time.Duration
}

func New(ttl time.Duration) *Store {
	s := &Store{
		listings: make(map[string]model.Listing),
		ttl:      ttl,
	}
	go s.sweep()
	return s
}

// Put inserts a listing. Returns true if it was new (callers use this to
// decide whether to rebroadcast).
func (s *Store) Put(l model.Listing) bool {
	if s.expired(l) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.listings[l.ID]; ok {
		return false
	}
	s.listings[l.ID] = l
	return true
}

// List returns listings newest-first, optionally filtered by geohash prefix.
func (s *Store) List(geohashPrefix string) []model.Listing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Listing, 0, len(s.listings))
	for _, l := range s.listings {
		if s.expired(l) {
			continue
		}
		if geohashPrefix != "" && !hasPrefix(l.Geohash, geohashPrefix) {
			continue
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out
}

func (s *Store) expired(l model.Listing) bool {
	return time.Since(time.Unix(l.Timestamp, 0)) > s.ttl
}

func (s *Store) sweep() {
	for range time.Tick(time.Minute) {
		s.mu.Lock()
		for id, l := range s.listings {
			if s.expired(l) {
				delete(s.listings, id)
			}
		}
		s.mu.Unlock()
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

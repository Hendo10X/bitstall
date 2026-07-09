// Package model defines the wire-level marketplace payloads.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxTitleLen is the PRD-mandated ceiling so listings stay BLE-friendly.
const MaxTitleLen = 40

// Listing is the payload gossiped through the mesh. Field names match the
// PRD payload specification exactly; keep this struct lean because it is
// serialized into size-constrained BLE/mesh packets.
type Listing struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Price     string `json:"price"`
	SellerKey string `json:"sellerKey"`
	Geohash   string `json:"geohash"`
	Timestamp int64  `json:"timestamp"`
}

// NewListing builds a listing with a deterministic short hash ID.
func NewListing(title, price, sellerKey, geohash string, ts int64) (Listing, error) {
	title = strings.TrimSpace(title)
	price = strings.TrimSpace(price)
	if title == "" {
		return Listing{}, errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > MaxTitleLen {
		return Listing{}, fmt.Errorf("title exceeds %d characters", MaxTitleLen)
	}
	if price == "" {
		return Listing{}, errors.New("price is required")
	}
	if sellerKey == "" {
		return Listing{}, errors.New("sellerKey is required")
	}
	l := Listing{
		Title:     title,
		Price:     price,
		SellerKey: sellerKey,
		Geohash:   geohash,
		Timestamp: ts,
	}
	l.ID = l.hash()
	return l, nil
}

// Validate checks a listing received from the mesh before it is stored.
func (l Listing) Validate() error {
	if l.ID == "" || l.Title == "" || l.Price == "" || l.SellerKey == "" {
		return errors.New("listing missing required fields")
	}
	if utf8.RuneCountInString(l.Title) > MaxTitleLen {
		return fmt.Errorf("title exceeds %d characters", MaxTitleLen)
	}
	return nil
}

func (l Listing) hash() string {
	h := sha256.Sum256([]byte(l.SellerKey + "|" + l.Title + "|" + l.Price + "|" + fmt.Sprint(l.Timestamp)))
	return hex.EncodeToString(h[:])[:12]
}

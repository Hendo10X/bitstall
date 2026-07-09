package bitchat

import (
	"context"
	"encoding/csv"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmcloughlin/geohash"
	nostr "github.com/nbd-wtf/go-nostr"
)

// geoRelayCSV is Bitchat's published directory of Nostr relays with
// approximate GPS locations. Bitchat routes every geohash channel to the
// relays closest to that geohash's center — "publishers and subscribers must
// agree on relays" — so to be seen by a Bitchat client we must publish and
// subscribe on the same closest set, not a fixed default set.
const geoRelayCSV = "https://raw.githubusercontent.com/permissionlesstech/georelays/refs/heads/main/nostr_relays.csv"

type geoRelay struct {
	url      string
	lat, lon float64
}

// GeoRelayDirectory loads the georelays CSV and picks the relays closest to a
// geohash, mirroring Bitchat's GeoRelayDirectory.closestRelays.
type GeoRelayDirectory struct {
	mu       sync.RWMutex
	entries  []geoRelay
	fetched  time.Time
	fallback []string
}

func NewGeoRelayDirectory(fallback []string) *GeoRelayDirectory {
	return &GeoRelayDirectory{fallback: fallback}
}

// Refresh fetches the directory (cached for a day). Safe to call repeatedly.
func (d *GeoRelayDirectory) Refresh(ctx context.Context) {
	d.mu.RLock()
	fresh := time.Since(d.fetched) < 24*time.Hour && len(d.entries) > 0
	d.mu.RUnlock()
	if fresh {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geoRelayCSV, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("bitchat: georelay directory fetch failed (using fallback relays): %v", err)
		return
	}
	defer resp.Body.Close()
	entries, err := parseGeoRelays(resp.Body)
	if err != nil || len(entries) == 0 {
		log.Printf("bitchat: georelay directory parse failed: %v", err)
		return
	}
	d.mu.Lock()
	d.entries = entries
	d.fetched = time.Now()
	d.mu.Unlock()
	log.Printf("bitchat: loaded %d geo-relays from directory", len(entries))
}

func parseGeoRelays(r io.Reader) ([]geoRelay, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []geoRelay
	for i, row := range rows {
		if i == 0 || len(row) < 3 {
			continue // header / malformed
		}
		host := strings.TrimSpace(row[0])
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if host == "" || err1 != nil || err2 != nil {
			continue
		}
		out = append(out, geoRelay{url: normalizeRelay(host), lat: lat, lon: lon})
	}
	return out, nil
}

// ClosestRelays returns up to count relay URLs closest to the geohash center,
// falling back to the configured default set if the directory is unavailable.
func (d *GeoRelayDirectory) ClosestRelays(gh string, count int) []string {
	lat, lon := geohash.DecodeCenter(gh)

	d.mu.RLock()
	entries := d.entries
	d.mu.RUnlock()

	if len(entries) == 0 {
		return append([]string(nil), d.fallback...)
	}

	sorted := make([]geoRelay, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return haversine(lat, lon, sorted[i].lat, sorted[i].lon) <
			haversine(lat, lon, sorted[j].lat, sorted[j].lon)
	})
	if count > len(sorted) {
		count = len(sorted)
	}
	out := make([]string, 0, count)
	seen := map[string]bool{}
	for _, e := range sorted {
		if seen[e.url] {
			continue
		}
		seen[e.url] = true
		out = append(out, e.url)
		if len(out) == count {
			break
		}
	}
	return out
}

func normalizeRelay(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "wss://") || strings.HasPrefix(host, "ws://") {
		return nostr.NormalizeURL(host)
	}
	return nostr.NormalizeURL("wss://" + host)
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dlat := (lat2 - lat1) * rad
	dlon := (lon2 - lon1) * rad
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

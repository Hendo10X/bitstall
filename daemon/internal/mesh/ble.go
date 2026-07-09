package mesh

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"
)

// bitstallCompanyID tags our manufacturer-data beacon. 0xFFFF is the
// Bluetooth SIG's reserved "internal use / testing" company identifier.
const bitstallCompanyID = 0xFFFF

// beaconMagic prefixes the manufacturer payload so we can recognize other
// Bitstall/Bitchat-family nodes among unrelated BLE chatter.
var beaconMagic = []byte{'E', 'V', '1'}

// BLETransport announces this node over Bluetooth Low Energy and scans for
// nearby mesh nodes. Discovery and presence are implemented; full payload
// exchange over GATT (intercepting Bitchat mesh packets, per roadmap step 1)
// is stubbed behind Broadcast — the gossip engine treats BLE as a
// zero-throughput transport until then, and listings ride other transports.
type BLETransport struct {
	geohash func() string
	active  atomic.Bool

	mu    sync.Mutex
	peers map[string]time.Time // BLE address -> last seen
}

func NewBLETransport(geohash func() string) *BLETransport {
	return &BLETransport{
		geohash: geohash,
		peers:   make(map[string]time.Time),
	}
}

func (t *BLETransport) Name() string { return "ble" }

func (t *BLETransport) Start(onReceive func([]byte)) error {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("enable adapter: %w", err)
	}

	// Advertise a compact presence beacon: magic + current geohash channel.
	adv := adapter.DefaultAdvertisement()
	payload := append(append([]byte{}, beaconMagic...), []byte(t.geohash())...)
	err := adv.Configure(bluetooth.AdvertisementOptions{
		LocalName: "bitstall",
		ManufacturerData: []bluetooth.ManufacturerDataElement{
			{CompanyID: bitstallCompanyID, Data: payload},
		},
	})
	if err != nil {
		log.Printf("mesh: ble advertise configure failed (scan-only mode): %v", err)
	} else if err := adv.Start(); err != nil {
		log.Printf("mesh: ble advertise start failed (scan-only mode): %v", err)
	}

	t.active.Store(true)

	go func() {
		// Scan blocks; it feeds the neighbor table used for PeerCount.
		err := adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
			if !t.isMeshNode(result) {
				return
			}
			t.mu.Lock()
			t.peers[result.Address.String()] = time.Now()
			t.mu.Unlock()
		})
		if err != nil && t.active.Load() {
			log.Printf("mesh: ble scan stopped: %v", err)
			t.active.Store(false)
		}
	}()
	return nil
}

// isMeshNode recognizes Bitstall beacons (and, by LocalName, Bitchat-family
// nodes) in scan results.
func (t *BLETransport) isMeshNode(r bluetooth.ScanResult) bool {
	for _, md := range r.ManufacturerData() {
		if md.CompanyID == bitstallCompanyID && len(md.Data) >= len(beaconMagic) &&
			string(md.Data[:len(beaconMagic)]) == string(beaconMagic) {
			return true
		}
	}
	name := r.LocalName()
	return name == "bitstall" || name == "bitchat"
}

// Broadcast is a no-op until GATT-based payload exchange lands: BLE
// advertisement frames are far too small for listing packets.
func (t *BLETransport) Broadcast([]byte) error { return nil }

func (t *BLETransport) Active() bool { return t.active.Load() }

func (t *BLETransport) PeerCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	now := time.Now()
	for addr, seen := range t.peers {
		if now.Sub(seen) <= presenceWindow {
			n++
		} else {
			delete(t.peers, addr)
		}
	}
	return n
}

func (t *BLETransport) Stop() {
	t.active.Store(false)
	_ = bluetooth.DefaultAdapter.StopScan()
}

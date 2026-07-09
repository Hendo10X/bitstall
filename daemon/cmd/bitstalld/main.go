// Command bitstalld is the Bitstall headless networking daemon: it owns BLE
// and local mesh gossip, cryptographic key material, Noise-encrypted
// negotiations, and — the headline feature — interoperation with the real
// Bitchat network over its Nostr geohash location channels. It serves the
// local REST API the dashboard reads.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"bitstall/daemon/internal/api"
	"bitstall/daemon/internal/bitchat"
	"bitstall/daemon/internal/crypto"
	"bitstall/daemon/internal/geo"
	"bitstall/daemon/internal/mesh"
	"bitstall/daemon/internal/model"
	"bitstall/daemon/internal/store"
)

func main() {
	var (
		port       = flag.Int("port", 4617, "local REST API port")
		dataDir    = flag.String("data", "data", "directory for identity and state")
		lat        = flag.Float64("lat", 37.7749, "initial latitude (until set via API)")
		lon        = flag.Float64("lon", -122.4194, "initial longitude (until set via API)")
		geohashPin = flag.String("geohash", "", "force an exact geohash channel (e.g. copy the one your Bitchat app shows); overrides lat/lon")
		precision  = flag.Uint("precision", geo.DefaultPrecision, "geohash precision: 7=block 6=neighborhood 5=city 4=province 2=region")
		nickname   = flag.String("nick", "bitstall", "nickname shown next to your posts in the Bitchat channel")
		pow        = flag.Int("pow", 8, "NIP-13 proof-of-work difficulty for channel posts (Bitchat mines 8; 0=off)")
		reannounce = flag.Duration("reannounce", 4*time.Minute, "how often to re-broadcast your listings so late-joining Bitchat users see them (0=off)")
		mcastGroup = flag.String("mcast", "239.117.86.1:4618", "UDP multicast group for LAN mesh ('' to disable)")
		peerList   = flag.String("peers", "", "comma-separated unicast peer host:port addrs (for firewalled segments)")
		enableBLE  = flag.Bool("ble", true, "enable the BLE transport")
		listingTTL = flag.Duration("listing-ttl", 24*time.Hour, "inventory expiration")
		relayList  = flag.String("relays", "wss://relay.damus.io,wss://nos.lol,wss://relay.primal.net,wss://offchain.pub", "comma-separated Nostr relays (Bitchat's default set)")
	)
	flag.Parse()

	identity, err := crypto.LoadOrCreate(*dataDir)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}
	log.Printf("identity: noise=%s nostr=%s", identity.PublicKeyHex(), identity.NostrPub)

	locator := geo.NewLocator(*lat, *lon, *precision)
	if *geohashPin != "" {
		locator.Pin(*geohashPin)
	}
	log.Printf("geohash channel: %s", locator.Geohash())

	inventory := store.New(*listingTTL)
	sessions := crypto.NewManager(identity)

	var relays []string
	for _, r := range strings.Split(*relayList, ",") {
		if r = strings.TrimSpace(r); r != "" {
			relays = append(relays, r)
		}
	}

	// Bitchat interop over Nostr geohash channels.
	bridge := bitchat.NewBridge(identity.DeviceSeed)
	bc := bitchat.NewClient(bridge, relays, *nickname, *pow)
	bc.OnListing = func(l model.Listing) {
		if err := l.Validate(); err != nil {
			return
		}
		if inventory.Put(l) {
			log.Printf("bitchat: new listing %s %q (%s)", l.ID, l.Title, l.Price)
		}
	}
	if err := bc.SetGeohash(locator.Geohash()); err != nil {
		log.Printf("bitchat: %v", err)
	}
	defer bc.Stop()

	// Periodically re-broadcast our own listings. Geohash channel messages are
	// ephemeral (relays don't persist them), so without this a Bitchat user who
	// opens the channel after we posted would never see the listing.
	if *reannounce > 0 {
		self := identity.PublicKeyHex()
		go func() {
			for range time.Tick(*reannounce) {
				for _, l := range inventory.List("") {
					if l.SellerKey == self {
						if err := bc.ReannounceListing(l); err != nil {
							log.Printf("bitchat: reannounce %s: %v", l.ID, err)
						}
					}
				}
			}
		}()
	}

	engine := mesh.NewEngine(identity.PublicKeyHex(), locator.Geohash)
	if *mcastGroup != "" {
		var unicastPeers []string
		for _, p := range strings.Split(*peerList, ",") {
			if p = strings.TrimSpace(p); p != "" {
				unicastPeers = append(unicastPeers, p)
			}
		}
		engine.AddTransport(mesh.NewMulticastTransport(*mcastGroup, unicastPeers))
	}
	if *enableBLE {
		engine.AddTransport(mesh.NewBLETransport(locator.Geohash))
	}

	// Inbound mesh listings (from other Bitstall nodes on the local mesh).
	engine.OnListing = func(body json.RawMessage) {
		var l model.Listing
		if err := json.Unmarshal(body, &l); err != nil {
			return
		}
		if err := l.Validate(); err != nil {
			return
		}
		if inventory.Put(l) {
			log.Printf("mesh: new listing %s %q (%s) from %s…", l.ID, l.Title, l.Price, l.SellerKey[:8])
		}
	}

	// Inbound mesh chat frames: run the Noise state machine and send replies.
	engine.OnChat = func(from, kind string, data []byte) {
		outs, err := sessions.HandlePacket(from, kind, data)
		if err != nil {
			log.Printf("chat: %s from %s…: %v", kind, from[:8], err)
			return
		}
		for _, o := range outs {
			engine.SendChat(o.To, o.Kind, o.Data)
		}
	}

	engine.Start()
	defer engine.Stop()

	server := &api.Server{
		Identity: identity,
		Sessions: sessions,
		Store:    inventory,
		Locator:  locator,
		Engine:   engine,
		Bitchat:  bc,
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	httpServer := &http.Server{Addr: addr, Handler: server.Handler()}
	go func() {
		log.Printf("api: listening on http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	log.Println("shutting down")
	httpServer.Close()
}

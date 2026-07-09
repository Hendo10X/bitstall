# Bitstall Protocol

A decentralized, local-first **marketplace for [Bitchat](https://github.com/permissionlesstech/bitchat)**. Bitstall posts listings into Bitchat's geohash location channels over Nostr, so real Bitchat users standing in the same area see your items and can DM you to buy — no accounts, no central server, no app to install beyond Bitchat itself. It also runs a local BLE/LAN mesh for Bitstall-to-Bitstall trading when there's no internet at all.

## How it interoperates with Bitchat

Bitchat carries messages two ways: a **BLE physical mesh** (phones relay to each other with no internet) and, beyond radio range, **Nostr geohash channels** over the internet. Bitstall speaks the second one natively — the same wire format Bitchat uses:

| Bitchat concept | Nostr encoding | Bitstall |
|---|---|---|
| Location channel message | kind **20000**, tag `["g", geohash]`, `["n", nickname]` | Listings & channel chat posted here |
| Channel presence | kind **20001**, `["g", geohash]` | Participant counting |
| Private DM | NIP-17 gift wrap: rumor(14) → seal(13) → wrap(**1059**) | Buyer/seller negotiation |
| Per-geohash identity | `HMAC-SHA256(deviceSeed, geohash‖u32_be(i))` → secp256k1 | Matches `NostrIdentityBridge` |
| Channel proof-of-work | NIP-13, 8 leading zero bits on kind-20000 | Matches `NostrPoW.targetBits` |
| Channel relays | the ~5 relays **closest to the geohash's GPS center**, from Bitchat's [georelays directory](https://github.com/permissionlesstech/georelays) | Matches `GeoRelayDirectory.closestRelays` |
| DM relays | damus / nos.lol / primal / offchain.pub (gift-wrap-capable) | Same default set |

**Relay routing is the crucial detail:** Bitchat does not read geohash channels from a fixed relay list — it routes each geohash to the Nostr relays geographically closest to that location ("publishers and subscribers must agree on relays"). The daemon fetches the same directory and publishes/subscribes on the same closest set, so a listing lands on the exact relays your phone is watching. Because kind-20000 messages are **ephemeral** (relays don't store them), the daemon also re-broadcasts your active listings periodically so a Bitchat user who opens the channel later still sees them arrive.

A listing posted in Bitstall appears in a Bitchat client's location channel as a chat line (`FOR SALE: … · … · DM to buy · via bitstall`), carrying an extra `ev` tag that other Bitstall nodes decode into structured inventory (Bitchat ignores unknown tags). All four directions are verified working against the live public relays: **post listing → phone**, **phone channel chat → dashboard**, **phone DM → dashboard**, **dashboard reply → phone**.

The **pure-offline BLE mesh** side of Bitchat is *not* wire-compatible (its binary BLE packet format and GATT UUIDs aren't published); Bitstall's own BLE/multicast mesh is Bitstall-to-Bitstall only. Internet-backed geohash interop is the Bitchat-facing path.

## Architecture

```
┌────────────────────────┐        ┌────────────────────────────────────────┐
│  web/  (Next.js UI)    │  REST  │  daemon/  (Go, headless)               │
│  shadcn/Tailwind       │◄──────►│  http://127.0.0.1:4617                 │
│  dashboard, no net     │        │                                        │
└────────────────────────┘        │  bitchat client ── Nostr geohash chans │
                                  │    ├─ kind-20000 listings + channel    │
                                  │    ├─ NIP-17 gift-wrap DMs             │
                                  │    └─ per-geohash identity bridge      │
                                  │  mesh engine ── Bloom-filter gossip     │
                                  │    ├─ multicast transport (UDP LAN)    │
                                  │    └─ BLE transport (discovery)        │
                                  │  Noise XX sessions (mesh negotiation)   │
                                  │  geohash locator (channel scoping)      │
                                  └────────────────────────────────────────┘
```

- **The Engine** (`daemon/`) — a Go daemon owning the Bitchat Nostr integration, BLE/mesh access, key material, and a local REST API.
- **The Interface** (`web/`) — a Next.js + shadcn/ui (Mira) dashboard that exclusively reads state from the daemon; it holds no network logic.

## Quick start

```sh
# 1. The daemon
cd daemon
go build -o bitstalld.exe ./cmd/bitstalld
./bitstalld.exe                      # API on http://127.0.0.1:4617

# 2. The dashboard
cd ../web
npm install
npm run dev                          # http://localhost:3000
```

First run generates a persistent identity in `daemon/data/identity.json`: a Noise static keypair (mesh identity), a Nostr key, and a **device seed** from which unlinkable per-geohash Bitchat identities are derived.

## Testing with the real Bitchat app on your phone

1. Open **Bitchat** on your phone and join your **location channel**. Note the geohash it shows (e.g. `9q8yy`). Bitchat channel levels: `block`=7 chars, `neighborhood`=6, `city`=5, `province`=4.
2. Start the daemon pinned to that exact channel:
   ```sh
   ./bitstalld.exe -geohash 9q8yy -nick "your-name"
   ```
   (Or set your real lat/lon with `-lat`/`-lon` and matching `-precision`; `-geohash` forces an exact match, which is easiest for testing.)
3. In the dashboard, post a listing. Within a second or two it appears **in your Bitchat location channel** on the phone as a `FOR SALE:` message.
4. From Bitchat, tap your listing's author and **DM** the seller — the message shows up in the dashboard's chats. Reply from the dashboard; it lands back in Bitchat. Any chatter you type in the Bitchat channel also shows in the dashboard's channel feed.

No jailbreak, no BLE pairing — just Bitchat on the phone and the daemon on your computer, meeting in the same geohash channel over public relays. The daemon automatically routes to the relays closest to your geohash (the ones your phone uses), mines the proof-of-work Bitchat expects, and re-broadcasts your listings, so no tuning should be needed. Keep the Bitchat channel open on your phone while you post — the listing arrives live.

## Daemon flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-port` | `4617` | Local REST API port |
| `-data` | `data` | Identity/state directory |
| `-geohash` | _(derived)_ | Force an exact geohash channel (copy what Bitchat shows) |
| `-precision` | `5` | Geohash precision when derived from lat/lon (7=block…4=province) |
| `-nick` | `bitstall` | Nickname next to your posts in Bitchat |
| `-pow` | `8` | NIP-13 proof-of-work bits for channel posts (Bitchat mines 8) |
| `-reannounce` | `4m` | How often to re-broadcast your listings so late-joiners see them (`0`=off) |
| `-lat`, `-lon` | San Francisco | Position until set via API |
| `-relays` | damus, nos.lol, primal, offchain.pub | DM relays + fallback if the geo-directory is unreachable |
| `-mcast` | `239.117.86.1:4618` | UDP multicast group for the local Bitstall mesh (`""` disables) |
| `-peers` | _(empty)_ | Unicast `host:port` peers for firewalled LAN segments |
| `-ble` | `true` | BLE discovery transport |
| `-listing-ttl` | `24h` | Inventory expiration |

## REST API

| Endpoint | Description |
|----------|-------------|
| `GET /api/status` | Peers, geohash, transports, identity, and `bitchat` (relay online/queued, participants, nickname) |
| `GET /api/listings` | Inventory, newest first (`mine` flags your own) |
| `POST /api/listings` | `{"title","price"}` → local mesh + Bitchat geohash channel (title ≤ 40 chars) |
| `POST /api/location` | `{"lat","lon"}` → recomputes geohash and rejoins the channel |
| `GET /api/channel` | Live Bitchat geohash channel feed |
| `POST /api/channel` | `{"text"}` → post a chat line into the Bitchat channel |
| `POST /api/config` | `{"nickname"}` → set your Bitchat nickname |
| `GET /api/chats` | Conversations across both transports (`transport`: `mesh`\|`bitchat`) |
| `GET /api/chats/{peer}` | Message history (`bitchat` peers are prefixed `bc:`) |
| `POST /api/chats/{peer}` | `{"text"}` → Bitchat gift-wrap DM or local Noise chat, by peer prefix |

## Local Bitstall mesh (offline, no Bitchat)

Two Bitstall daemons also trade directly with no internet, over UDP multicast (LAN) or BLE discovery, using the same Bloom-filtered gossip and Noise XX encrypted negotiation:

```sh
./bitstalld.exe -port 4617 -data ./data-a -relays ""
./bitstalld.exe -port 4620 -data ./data-b -relays ""
curl -X POST http://127.0.0.1:4620/api/listings -H "Content-Type: application/json" \
  -d '{"title":"Vintage ham radio","price":"80 USD"}'
curl http://127.0.0.1:4617/api/listings   # gossiped across within a second
```

### Windows note

Windows Defender Firewall commonly blocks inbound UDP multicast on physical NICs. The transport joins the group on every interface including loopback (same-host nodes always peer) and transmits per-interface; for firewalled LAN segments use `-peers host:4618`.

## Development

```sh
cd daemon && go vet ./... && go build ./...
cd web && npm run build
```

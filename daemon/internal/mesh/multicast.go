package mesh

import (
	"log"
	"net"
	"strings"
	"sync/atomic"

	"golang.org/x/net/ipv4"
)

// MulticastTransport gossips packets over UDP. It stands in for the BLE data
// channel during development and doubles as a bridge when several nodes
// share a network segment — the gossip, Bloom-filter and crypto layers above
// it are transport-agnostic.
//
// The group is joined on every multicast-capable interface (loopback
// included, so same-host daemons peer even when the OS firewall filters the
// physical NICs), and each broadcast is transmitted once per interface.
// Explicit unicast peers can be added for segments where multicast is
// filtered entirely.
type MulticastTransport struct {
	group     string
	peers     []string // optional unicast peer addrs ("host:port")
	conn      *net.UDPConn
	pconn     *ipv4.PacketConn
	groupAddr *net.UDPAddr
	ifaces    []net.Interface
	active    atomic.Bool
}

func NewMulticastTransport(group string, peers []string) *MulticastTransport {
	return &MulticastTransport{group: group, peers: peers}
}

func (t *MulticastTransport) Name() string { return "multicast" }

func (t *MulticastTransport) Start(onReceive func([]byte)) error {
	addr, err := net.ResolveUDPAddr("udp4", t.group)
	if err != nil {
		return err
	}
	t.groupAddr = addr

	// ListenMulticastUDP sets SO_REUSEADDR so several daemons can share the
	// port on one machine.
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	t.conn = conn
	t.pconn = ipv4.NewPacketConn(conn)
	_ = t.pconn.SetMulticastLoopback(true)

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifi := range ifaces {
			if ifi.Flags&net.FlagUp == 0 {
				continue
			}
			if ifi.Flags&(net.FlagMulticast|net.FlagLoopback) == 0 {
				continue
			}
			ifi := ifi
			// Duplicate joins (default interface) error harmlessly.
			if err := t.pconn.JoinGroup(&ifi, &net.UDPAddr{IP: addr.IP}); err == nil || isInUse(err) {
				t.ifaces = append(t.ifaces, ifi)
			}
		}
	}
	if len(t.ifaces) == 0 {
		log.Printf("mesh: multicast joined only the default interface")
	}

	t.active.Store(true)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if t.active.Load() {
					log.Printf("mesh: multicast read: %v", err)
				}
				return
			}
			frame := make([]byte, n)
			copy(frame, buf[:n])
			onReceive(frame)
		}
	}()
	return nil
}

func (t *MulticastTransport) Broadcast(raw []byte) error {
	var lastErr error
	sent := false
	for i := range t.ifaces {
		if err := t.pconn.SetMulticastInterface(&t.ifaces[i]); err != nil {
			lastErr = err
			continue
		}
		if _, err := t.pconn.WriteTo(raw, nil, t.groupAddr); err != nil {
			lastErr = err
		} else {
			sent = true
		}
	}
	if len(t.ifaces) == 0 { // fall back to the OS default interface
		if _, err := t.conn.WriteToUDP(raw, t.groupAddr); err != nil {
			lastErr = err
		} else {
			sent = true
		}
	}
	for _, peer := range t.peers {
		if dst, err := net.ResolveUDPAddr("udp4", peer); err == nil {
			if _, err := t.conn.WriteToUDP(raw, dst); err != nil {
				lastErr = err
			} else {
				sent = true
			}
		}
	}
	if sent {
		return nil
	}
	return lastErr
}

func (t *MulticastTransport) Active() bool { return t.active.Load() }

// PeerCount is 0: multicast peers are counted at the engine level from the
// gossip they send (the transport can't distinguish nodes reliably).
func (t *MulticastTransport) PeerCount() int { return 0 }

func (t *MulticastTransport) Stop() {
	t.active.Store(false)
	if t.conn != nil {
		t.conn.Close()
	}
}

// isInUse reports the "already joined" class of errors from JoinGroup
// (ListenMulticastUDP joins the default interface before we sweep them all).
func isInUse(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "requested address is not valid") ||
		strings.Contains(msg, "Only one usage")
}

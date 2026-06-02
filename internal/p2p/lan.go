package p2p

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"
)

// lanGroup is the IPv4 multicast group ShadowLedger nodes beacon on for
// zero-config LAN discovery (like mDNS/Bonjour, but minimal).
const lanGroup = "239.255.42.99:48999"

// RunLANDiscovery periodically multicasts this node's descriptor on the LAN and
// listens for others', handshaking with any it hears. This lets nodes on the
// same network find each other with NO seeds and NO central server. Internet
// peers still need a seed entry point; LAN discovery is a convenience for
// local clusters. Best-effort: logs and returns if multicast is unavailable.
func (s *Server) RunLANDiscovery(ctx context.Context) {
	gaddr, err := net.ResolveUDPAddr("udp4", lanGroup)
	if err != nil {
		log.Printf("lan discovery: resolve: %v", err)
		return
	}
	// Listener.
	lis, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		log.Printf("lan discovery disabled: %v", err)
		return
	}
	lis.SetReadBuffer(8192)
	go s.lanListen(ctx, lis)

	// Sender.
	out, err := net.DialUDP("udp4", nil, gaddr)
	if err != nil {
		log.Printf("lan discovery: dial: %v", err)
		return
	}
	defer out.Close()
	beacon, _ := json.Marshal(s.store.Self())
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	log.Printf("lan discovery beaconing on %s", lanGroup)
	out.Write(beacon) // announce immediately
	for {
		select {
		case <-ctx.Done():
			lis.Close()
			return
		case <-ticker.C:
			out.Write(beacon)
		}
	}
}

func (s *Server) lanListen(ctx context.Context, lis *net.UDPConn) {
	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		lis.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := lis.ReadFromUDP(buf)
		if err != nil {
			continue // timeout/closed; loop re-checks ctx
		}
		var p Peer
		if json.Unmarshal(buf[:n], &p) != nil {
			continue
		}
		if p.ID == "" || p.ID == s.store.Self().ID {
			continue
		}
		if s.store.Add(p) {
			log.Printf("lan discovered peer %s (%s)", short(p.ID), p.Control)
			go s.sayHello(p.Control) // exchange full peer lists
		}
	}
}

func short(id string) string {
	if len(id) > 10 {
		return id[:10]
	}
	return id
}

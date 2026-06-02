// Package p2p provides ShadowLedger's networking: a control/RPC channel and a
// shard-transfer channel, decentralized peer discovery (seeds + peer exchange +
// optional LAN multicast, NO central server), and the client that reconstructs
// block bodies by pulling K-of-(K+M) shards from rendezvous-ranked holders.
package p2p

import (
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ArubikU/shadowledger/internal/chain"
	"github.com/ArubikU/shadowledger/internal/mempool"
)

// Peer identifies another node and its two endpoints.
type Peer struct {
	ID      string `json:"id"`      // node identity (its address string)
	Control string `json:"control"` // http://host:4004
	Shard   string `json:"shard"`   // http://host:4005
}

// Peerstore is a thread-safe, dynamically-growing set of known peers. There is
// no central registry: peers are learned from seeds, peer-exchange and LAN
// beacons, and the set converges as gossip spreads.
type Peerstore struct {
	mu   sync.RWMutex
	self Peer
	m    map[string]Peer // id -> peer (excludes self)
}

// NewPeerstore seeds the store with this node's own identity.
func NewPeerstore(self Peer) *Peerstore {
	return &Peerstore{self: self, m: make(map[string]Peer)}
}

// Self returns this node's peer descriptor.
func (ps *Peerstore) Self() Peer { return ps.self }

// Add records a peer (ignoring self and incomplete entries). Returns true if new.
func (ps *Peerstore) Add(p Peer) bool {
	if p.ID == "" || p.ID == ps.self.ID || p.Control == "" {
		return false
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	_, existed := ps.m[p.ID]
	ps.m[p.ID] = p
	return !existed
}

// Merge adds many peers, returning how many were newly learned.
func (ps *Peerstore) Merge(ps2 []Peer) int {
	n := 0
	for _, p := range ps2 {
		if ps.Add(p) {
			n++
		}
	}
	return n
}

// Peers returns the known peers (excluding self), sorted by id for determinism.
func (ps *Peerstore) Peers() []Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]Peer, 0, len(ps.m))
	for _, p := range ps.m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MemberIDs returns every node id (self + known peers) for rendezvous hashing.
func (ps *Peerstore) MemberIDs() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	ids := make([]string, 0, len(ps.m)+1)
	ids = append(ids, ps.self.ID)
	for id := range ps.m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Count returns the live network size as this node sees it (self + peers).
func (ps *Peerstore) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.m) + 1
}

// shardURLFor returns the shard endpoint of a node id, or "" if unknown/self.
func (ps *Peerstore) shardURLFor(id string) string {
	if id == ps.self.ID {
		return ""
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if p, ok := ps.m[id]; ok {
		return p.Shard
	}
	return ""
}

// Server hosts both channels and brokers gossip, discovery and shard fetches.
type Server struct {
	store  *Peerstore
	seeds  []string // bootstrap control URLs (entry points, not a central server)
	chain  *chain.Chain
	pool   *mempool.Pool
	client *http.Client
}

// NewServer builds the networking layer over a peerstore.
func NewServer(store *Peerstore, seeds []string, ch *chain.Chain, pool *mempool.Pool) *Server {
	return &Server{
		store: store, seeds: seeds, chain: ch, pool: pool,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

// MemberIDs exposes the rendezvous membership set.
func (s *Server) MemberIDs() []string { return s.store.MemberIDs() }

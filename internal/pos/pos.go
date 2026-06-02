// Package pos implements ShadowLedger's Proof-of-Storage challenge layer.
//
// Nodes assigned a shard (by rendezvous) must, on demand, prove they still hold
// the bytes. A challenger sends a fresh random nonce; the holder answers
// H(nonce || shardBytes). Because the nonce is unpredictable, a node cannot
// precompute the answer — it must have the bytes at challenge time. A holder
// that fails or times out is scored down and (in the multi-validator model)
// loses block-production eligibility and its share of recycled fees.
package pos

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/ArubikU/shadowledger/internal/types"
)

// Challenge is the storage proof: H(nonce || shardBytes).
func Challenge(nonce types.Hash, shard []byte) types.Hash {
	h := sha256.New()
	h.Write(nonce[:])
	h.Write(shard)
	var out types.Hash
	copy(out[:], h.Sum(nil))
	return out
}

// Score tracks one node's challenge history.
type Score struct {
	Pass     uint64 `json:"pass"`
	Miss     uint64 `json:"miss"`
	LastSeen int64  `json:"last_seen"` // unix seconds of last successful proof
}

// Ratio returns the fraction of challenges passed (1.0 if never challenged).
func (s Score) Ratio() float64 {
	total := s.Pass + s.Miss
	if total == 0 {
		return 1.0
	}
	return float64(s.Pass) / float64(total)
}

// Scoreboard aggregates per-node storage scores. Thread-safe.
type Scoreboard struct {
	mu  sync.RWMutex
	m   map[string]*Score
	now func() int64
}

// NewScoreboard returns an empty scoreboard. nowFn supplies unix-seconds (so
// callers/tests can inject a clock); pass nil for time.Now.
func NewScoreboard(nowFn func() int64) *Scoreboard {
	if nowFn == nil {
		nowFn = func() int64 { return time.Now().Unix() }
	}
	return &Scoreboard{m: make(map[string]*Score), now: nowFn}
}

func (b *Scoreboard) entry(id string) *Score {
	s := b.m[id]
	if s == nil {
		s = &Score{}
		b.m[id] = s
	}
	return s
}

// Pass records a successful proof by node id.
func (b *Scoreboard) Pass(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.entry(id)
	s.Pass++
	s.LastSeen = b.now()
}

// Miss records a failed/absent proof by node id.
func (b *Scoreboard) Miss(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entry(id).Miss++
}

// Eligible reports whether a node's pass ratio meets minRatio (validators below
// this lose block-production rights in the multi-validator model).
func (b *Scoreboard) Eligible(id string, minRatio float64) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := b.m[id]
	if s == nil {
		return true // never challenged yet — benefit of the doubt
	}
	return s.Ratio() >= minRatio
}

// Snapshot returns a copy of all scores keyed by node id.
func (b *Scoreboard) Snapshot() map[string]Score {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]Score, len(b.m))
	for id, s := range b.m {
		out[id] = *s
	}
	return out
}

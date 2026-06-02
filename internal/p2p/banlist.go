package p2p

import (
	"sync"
	"time"
)

// Banlist is LOCAL, per-node DoS hygiene — not consensus. Misbehaving peers
// (bad blocks, lying shards) accrue strikes; past a threshold they are
// temp-banned with exponential backoff. Bans self-expire. This only protects a
// node's own resources; real economic security against validators is on-chain
// slashing (see state slashing), not this list.
type Banlist struct {
	mu        sync.Mutex
	strikes   map[string]int
	until     map[string]time.Time
	threshold int
	base      time.Duration
	now       func() time.Time
}

// NewBanlist bans a key after `threshold` strikes for `base` (doubling per extra
// strike, capped). nowFn may be nil (uses time.Now).
func NewBanlist(threshold int, base time.Duration, nowFn func() time.Time) *Banlist {
	if threshold < 1 {
		threshold = 3
	}
	if base <= 0 {
		base = 10 * time.Minute
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Banlist{
		strikes: map[string]int{}, until: map[string]time.Time{},
		threshold: threshold, base: base, now: nowFn,
	}
}

// Strike records misbehavior by key (a node id or IP). Returns true if now banned.
func (b *Banlist) Strike(key string) bool {
	if key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.strikes[key]++
	n := b.strikes[key]
	if n < b.threshold {
		return false
	}
	exp := n - b.threshold
	if exp > 6 { // cap backoff at base*64
		exp = 6
	}
	b.until[key] = b.now().Add(b.base << uint(exp))
	return true
}

// Banned reports whether key is currently banned.
func (b *Banlist) Banned(key string) bool {
	if key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	u, ok := b.until[key]
	if !ok {
		return false
	}
	if b.now().Before(u) {
		return true
	}
	delete(b.until, key) // expired; reset strikes so it gets another chance
	delete(b.strikes, key)
	return false
}

// Snapshot returns currently-banned keys and when their ban lifts.
func (b *Banlist) Snapshot() map[string]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]string{}
	now := b.now()
	for k, u := range b.until {
		if now.Before(u) {
			out[k] = u.UTC().Format(time.RFC3339)
		}
	}
	return out
}

// Package mempool holds pending, validated transactions awaiting inclusion.
package mempool

import (
	"sort"
	"sync"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/types"
)

// Pool is a thread-safe transaction pool.
type Pool struct {
	mu    sync.Mutex
	st    *state.State
	byID  map[types.Hash]types.Transaction
	limit int
}

// New creates a pool that validates against st, capped at limit txs.
func New(st *state.State, limit int) *Pool {
	if limit < 1 {
		limit = 10000
	}
	return &Pool{st: st, byID: make(map[types.Hash]types.Transaction), limit: limit}
}

// Submit validates a tx against current state and enqueues it.
func (p *Pool) Submit(tx types.Transaction) error {
	if err := p.st.CheckTx(&tx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.byID) >= p.limit {
		return ErrFull
	}
	p.byID[tx.Hash()] = tx
	return nil
}

// Len returns the number of pooled txs.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byID)
}

// Reap selects up to max txs in (from,nonce) order suitable for a block.
// It returns a self-consistent batch: per sender, nonces are contiguous from
// the account's current nonce.
func (p *Pool) Reap(max int) []types.Transaction {
	p.mu.Lock()
	defer p.mu.Unlock()
	bySender := map[crypto.Address][]types.Transaction{}
	for _, tx := range p.byID {
		bySender[tx.From] = append(bySender[tx.From], tx)
	}
	var out []types.Transaction
	for from, txs := range bySender {
		sort.Slice(txs, func(i, j int) bool { return txs[i].Nonce < txs[j].Nonce })
		next := p.st.Get(from).Nonce
		for _, tx := range txs {
			if tx.Nonce != next {
				break // gap; stop including this sender
			}
			out = append(out, tx)
			next++
			if len(out) >= max {
				return out
			}
		}
	}
	return out
}

// Remove drops txs (e.g. after they were included in a block).
func (p *Pool) Remove(txs []types.Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range txs {
		delete(p.byID, txs[i].Hash())
	}
}

// ErrFull is returned when the pool is at capacity.
var ErrFull = poolErr("mempool: full")

type poolErr string

func (e poolErr) Error() string { return string(e) }

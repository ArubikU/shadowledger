// Package consensus defines how blocks are produced and headers authorized.
//
// Two engines ship:
//   - Authority: a single trusted producer (v0 default). Centralized but simple.
//   - PoStorage: deterministic leader election among a validator set, seeded by
//     the previous block hash — minting rotates, no coordination, one leader per
//     height (no forks from disagreement). The storage-eligibility gate (a node
//     must prove it stores its shards to lead) is advisory here; enforceable
//     on-chain proofs + slashing are the next step. See docs/CONSENSUS.md.
package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/pos"
	"github.com/ArubikU/shadowledger/internal/types"
)

// Engine decides block-production rights and authorizes headers.
type Engine interface {
	// LeaderFor returns the address allowed to produce the block at height,
	// given the previous block hash.
	LeaderFor(height uint64, prev types.Hash) crypto.Address
	// AuthorizeHeader checks that a header was produced by the right validator.
	AuthorizeHeader(h *types.Header) error
	// CanProduce reports whether THIS node may produce the next block.
	CanProduce(nextHeight uint64, prev types.Hash) bool
	// IsValidator reports whether this node is in the validator set at all.
	IsValidator() bool
}

// Consensus errors.
var (
	ErrUnauthorizedValidator = errors.New("consensus: header signed by unauthorized validator")
	ErrNotLeader             = errors.New("consensus: validator is not the elected leader for this height")
)

func validatorSet(validators []crypto.Address) (map[crypto.Address]bool, []crypto.Address) {
	set := make(map[crypto.Address]bool, len(validators))
	list := make([]crypto.Address, 0, len(validators))
	for _, v := range validators {
		if !set[v] {
			set[v] = true
			list = append(list, v)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	return set, list
}

// ---- Authority: single-producer (v0 default) ----

// Authority authorizes blocks from any key in a fixed validator set; in practice
// one configured producer.
type Authority struct {
	set  map[crypto.Address]bool
	Self crypto.Address
}

// NewAuthority builds an authority engine. self may be "" for non-producers.
func NewAuthority(validators []crypto.Address, self crypto.Address) *Authority {
	set, _ := validatorSet(validators)
	return &Authority{set: set, Self: self}
}

func (a *Authority) LeaderFor(uint64, types.Hash) crypto.Address { return a.Self }

func (a *Authority) AuthorizeHeader(h *types.Header) error {
	if err := h.VerifySig(); err != nil {
		return err
	}
	if !a.set[h.Validator] {
		return ErrUnauthorizedValidator
	}
	return nil
}

func (a *Authority) CanProduce(uint64, types.Hash) bool {
	return a.Self != "" && a.set[a.Self]
}

func (a *Authority) IsValidator() bool { return a.Self != "" && a.set[a.Self] }

// ---- PoStorage: deterministic leader election among validators ----

// PoStorage elects one leader per height by HRW over the validator set, seeded
// by the previous block hash. The validator set is read LIVE from a provider
// (the on-chain registry via state.ActiveValidators), so it is chain-derived and
// identical on every node — registrations/exits change the minting rotation
// without any config change. Optionally gates the local node on its storage
// score (advisory).
type PoStorage struct {
	validators func() []crypto.Address // live on-chain active validator set
	Self       crypto.Address
	scores     *pos.Scoreboard // optional advisory storage gate (nil = disabled)
	minRatio   float64
}

// StaticValidators wraps a fixed slice as a validator provider (tests/config).
func StaticValidators(vs []crypto.Address) func() []crypto.Address {
	return func() []crypto.Address { return vs }
}

// NewPoStorage builds a PoStorage engine over a live validator-set provider.
// scores may be nil (no storage gate).
func NewPoStorage(validators func() []crypto.Address, self crypto.Address, scores *pos.Scoreboard, minRatio float64) *PoStorage {
	if minRatio <= 0 {
		minRatio = 0.8
	}
	return &PoStorage{validators: validators, Self: self, scores: scores, minRatio: minRatio}
}

func (p *PoStorage) setList() (map[crypto.Address]bool, []crypto.Address) {
	return validatorSet(p.validators())
}

func leaderScore(prev types.Hash, v crypto.Address, height uint64) uint64 {
	h := sha256.New()
	h.Write(prev[:])
	h.Write([]byte(v))
	var hb [8]byte
	binary.BigEndian.PutUint64(hb[:], height)
	h.Write(hb[:])
	return binary.BigEndian.Uint64(h.Sum(nil)[:8])
}

// LeaderFor returns the elected validator for (height, prev).
func (p *PoStorage) LeaderFor(height uint64, prev types.Hash) crypto.Address {
	_, list := p.setList()
	var best crypto.Address
	var bestScore uint64
	for _, v := range list {
		s := leaderScore(prev, v, height)
		if best == "" || s > bestScore || (s == bestScore && v < best) {
			best, bestScore = v, s
		}
	}
	return best
}

func (p *PoStorage) AuthorizeHeader(h *types.Header) error {
	if err := h.VerifySig(); err != nil {
		return err
	}
	set, _ := p.setList()
	if !set[h.Validator] {
		return ErrUnauthorizedValidator
	}
	if h.Height == 0 {
		return nil // genesis: deterministic, stamped by validators[0], no election
	}
	if h.Validator != p.LeaderFor(h.Height, h.PrevHash) {
		return ErrNotLeader
	}
	return nil
}

func (p *PoStorage) CanProduce(nextHeight uint64, prev types.Hash) bool {
	set, _ := p.setList()
	if p.Self == "" || !set[p.Self] {
		return false
	}
	if p.LeaderFor(nextHeight, prev) != p.Self {
		return false
	}
	// Advisory storage gate: don't lead if we're failing storage proofs.
	if p.scores != nil && !p.scores.Eligible(string(p.Self), p.minRatio) {
		return false
	}
	return true
}

func (p *PoStorage) IsValidator() bool {
	set, _ := p.setList()
	return p.Self != "" && set[p.Self]
}

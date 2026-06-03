package consensus

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
)

func addrs(n int) []crypto.Address {
	out := make([]crypto.Address, n)
	for i := 0; i < n; i++ {
		kp, _ := crypto.Generate()
		out[i] = kp.Address()
	}
	return out
}

func TestLeaderDeterministicAndInSet(t *testing.T) {
	vs := addrs(5)
	e := NewPoStorage(StaticValidators(vs), vs[0], nil, 0.8)
	var prev types.Hash
	prev[0] = 0x42
	l1 := e.LeaderFor(7, prev, 0)
	l2 := e.LeaderFor(7, prev, 0)
	if l1 != l2 {
		t.Fatal("leader not deterministic")
	}
	inSet := false
	for _, v := range vs {
		if v == l1 {
			inSet = true
		}
	}
	if !inSet {
		t.Fatal("leader not in validator set")
	}
}

func TestStorageWeightedElection(t *testing.T) {
	vs := addrs(2)
	low, high := vs[0], vs[1]
	e := NewPoStorage(StaticValidators(vs), low, nil, 0.8)
	// `high` has proven storage (weight 65); `low` has the base weight 1.
	e.SetWeights(func() map[crypto.Address]uint64 {
		return map[crypto.Address]uint64{low: 1, high: 65}
	})
	var prev types.Hash
	wins := map[crypto.Address]int{}
	for h := uint64(0); h < 2000; h++ {
		prev[0] = byte(h)
		prev[1] = byte(h >> 8)
		wins[e.LeaderFor(h, prev, 0)]++
	}
	// With ~65:1 weighting, the high-storage validator should win the large
	// majority; assert at least 90% rather than an exact ratio.
	if wins[high]*10 < 9*(wins[low]+wins[high]) {
		t.Fatalf("storage weighting weak: high=%d low=%d", wins[high], wins[low])
	}
	if wins[low] == 0 {
		t.Fatal("base-weight validator never won (should still win occasionally)")
	}
}

func TestLeaderRotates(t *testing.T) {
	vs := addrs(5)
	e := NewPoStorage(StaticValidators(vs), vs[0], nil, 0.8)
	var prev types.Hash
	seen := map[crypto.Address]int{}
	for h := uint64(0); h < 200; h++ {
		seen[e.LeaderFor(h, prev, 0)]++
	}
	// Every validator should win at least once over 200 heights (rotation).
	if len(seen) < len(vs) {
		t.Fatalf("only %d/%d validators ever led — not rotating", len(seen), len(vs))
	}
}

func TestAuthorizeRequiresElectedLeader(t *testing.T) {
	// Build validators from known keypairs so we can sign.
	kps := make([]*crypto.KeyPair, 4)
	vs := make([]crypto.Address, 4)
	for i := range kps {
		kps[i], _ = crypto.Generate()
		vs[i] = kps[i].Address()
	}
	e := NewPoStorage(StaticValidators(vs), vs[0], nil, 0.8)
	var prev types.Hash
	prev[1] = 0x99
	height := uint64(10)
	leader := e.LeaderFor(height, prev, 0)

	// Header from the elected leader → authorized.
	var leaderKP *crypto.KeyPair
	for i, v := range vs {
		if v == leader {
			leaderKP = kps[i]
		}
	}
	h := types.Header{Height: height, PrevHash: prev}
	h.Sign(leaderKP)
	if err := e.AuthorizeHeader(&h); err != nil {
		t.Fatalf("elected leader rejected: %v", err)
	}

	// Header from a non-leader validator → ErrNotLeader.
	var other *crypto.KeyPair
	for i, v := range vs {
		if v != leader {
			other = kps[i]
			break
		}
	}
	h2 := types.Header{Height: height, PrevHash: prev}
	h2.Sign(other)
	if err := e.AuthorizeHeader(&h2); err != ErrNotLeader {
		t.Fatalf("want ErrNotLeader, got %v", err)
	}
}

func TestGenesisAuthorizedWithoutElection(t *testing.T) {
	kp, _ := crypto.Generate()
	e := NewPoStorage(StaticValidators([]crypto.Address{kp.Address()}), kp.Address(), nil, 0.8)
	h := types.Header{Height: 0}
	h.Sign(kp)
	if err := e.AuthorizeHeader(&h); err != nil {
		t.Fatalf("genesis rejected: %v", err)
	}
}

func TestRoundFallbackChangesLeader(t *testing.T) {
	vs := addrs(5)
	e := NewPoStorage(StaticValidators(vs), vs[0], nil, 0.8)
	var prev types.Hash
	prev[0] = 0x07
	// Across heights, advancing the round must (at least sometimes) elect a
	// different leader — that's how an offline round-0 leader gets bypassed.
	diff := 0
	for h := uint64(0); h < 20; h++ {
		if e.LeaderFor(h, prev, 0) != e.LeaderFor(h, prev, 1) {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("round advance never changed the leader — no liveness fallback")
	}
}

func TestSingleValidatorAlwaysLeads(t *testing.T) {
	kp, _ := crypto.Generate()
	e := NewPoStorage(StaticValidators([]crypto.Address{kp.Address()}), kp.Address(), nil, 0.8)
	var prev types.Hash
	if !e.CanProduce(1, prev, 0) {
		t.Fatal("single validator should always be able to produce")
	}
}

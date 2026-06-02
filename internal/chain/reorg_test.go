package chain

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/bloom"
	"github.com/ArubikU/shadowledger/internal/consensus"
	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/state"
	"github.com/ArubikU/shadowledger/internal/store"
)

// newMultiValChain builds a chain whose Authority engine accepts blocks from any
// of `vals` (so we can forge competing branches from two producers).
func newMultiValChain(t *testing.T, vals []crypto.Address, self crypto.Address) *Chain {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	eng := consensus.NewAuthority(vals, self)
	bf := bloom.New(1000, 0.01)
	members := func() []string { return []string{string(self)} }
	return New(st, state.New(), eng, bf, Config{SelfID: string(self), Members: members})
}

func TestReorgHeavierBranchWins(t *testing.T) {
	v1, _ := crypto.Generate()
	v2, _ := crypto.Generate()
	vals := []crypto.Address{v1.Address(), v2.Address()}
	fund := map[crypto.Address]uint64{v1.Address(): 1_000_000}

	// Branch A: a single block produced by v1.
	cA := newMultiValChain(t, vals, v1.Address())
	cA.Genesis(fund, v1.Address(), v1)
	blkA1, _, err := cA.ProduceBlock(nil, v1, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Branch B: TWO blocks (heavier) produced by v2 then v1.
	cB := newMultiValChain(t, vals, v2.Address())
	cB.Genesis(fund, v1.Address(), v1)
	blkB1, _, err := cB.ProduceBlock(nil, v2, 0)
	if err != nil {
		t.Fatal(err)
	}
	blkB2, _, err := cB.ProduceBlock(nil, v1, 0)
	if err != nil {
		t.Fatal(err)
	}

	// The node under test sees A's block first, then B's two blocks → must reorg
	// to the heavier branch B and end up with B's tip + B's state.
	cT := newMultiValChain(t, vals, v1.Address())
	cT.Genesis(fund, v1.Address(), v1)
	if err := cT.AcceptBlock(blkA1); err != nil {
		t.Fatalf("accept A1: %v", err)
	}
	if err := cT.AcceptBlock(blkB1); err != nil {
		t.Fatalf("accept B1: %v", err)
	}
	if err := cT.AcceptBlock(blkB2); err != nil {
		t.Fatalf("accept B2: %v", err)
	}

	head, _ := cT.Head()
	if head.ID() != blkB2.Header.ID() {
		t.Fatalf("head = %x, want B2 %x (did not reorg to heavier branch)", head.ID(), blkB2.Header.ID())
	}
	if head.Height != 2 {
		t.Fatalf("head height = %d, want 2", head.Height)
	}
	// State must match branch B: v2 earned the block-1 coinbase on branch B.
	if cT.State().Get(v2.Address()).Balance == 0 {
		t.Fatal("after reorg, v2 has no balance — branch-B state not replayed")
	}
	if cT.State().Get(v2.Address()).Balance != cB.State().Get(v2.Address()).Balance {
		t.Fatalf("reorged state mismatch vs branch B: v2 %d != %d",
			cT.State().Get(v2.Address()).Balance, cB.State().Get(v2.Address()).Balance)
	}
}

func TestFinalityAdvances(t *testing.T) {
	v1, _ := crypto.Generate()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	eng := consensus.NewAuthority([]crypto.Address{v1.Address()}, v1.Address())
	bf := bloom.New(1000, 0.01)
	members := func() []string { return []string{string(v1.Address())} }
	c := New(st, state.New(), eng, bf, Config{SelfID: string(v1.Address()), Members: members, FinalityDepth: 3})
	c.Genesis(map[crypto.Address]uint64{v1.Address(): 1_000_000_000}, v1.Address(), v1)

	for i := 0; i < 6; i++ {
		if _, _, err := c.ProduceBlock(nil, v1, 0); err != nil {
			t.Fatal(err)
		}
	}
	// head=6, F=3 -> blocks at height <=3 are irreversible.
	if f := c.Finalized(); f != 3 {
		t.Fatalf("finalized height = %d, want 3 (head 6 - F 3)", f)
	}
	// The reorg floor (replay base) moved up to the finalized height, so a reorg
	// can no longer rewind below it — deep history is locked in.
	if c.replayBaseHeight != 3 {
		t.Fatalf("replay base height = %d, want 3", c.replayBaseHeight)
	}
}

func TestAcceptBlockUnknownParent(t *testing.T) {
	v1, _ := crypto.Generate()
	c := newMultiValChain(t, []crypto.Address{v1.Address()}, v1.Address())
	c.Genesis(map[crypto.Address]uint64{v1.Address(): 1_000_000}, v1.Address(), v1)

	// A block whose parent we don't have (orphan) is refused.
	other := newMultiValChain(t, []crypto.Address{v1.Address()}, v1.Address())
	other.Genesis(map[crypto.Address]uint64{v1.Address(): 1_000_000}, v1.Address(), v1)
	_, _, _ = other.ProduceBlock(nil, v1, 0)   // b1 (c never sees it)
	b2, _, _ := other.ProduceBlock(nil, v1, 0) // parent = b1, which `c` never saw
	if err := c.AcceptBlock(b2); err != ErrUnknownParent {
		t.Fatalf("want ErrUnknownParent, got %v", err)
	}
}

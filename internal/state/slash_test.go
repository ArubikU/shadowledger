package state

import (
	"encoding/json"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/economy"
	"github.com/ArubikU/shadowledger/internal/types"
)

func TestEquivocationSlash(t *testing.T) {
	badVal, _ := crypto.Generate() // the equivocator (also a validator)
	reporter, _ := crypto.Generate()
	blkVal, _ := crypto.Generate() // block producer for our test blocks
	s := New()
	s.Credit(badVal.Address(), economy.MinBond)
	s.Credit(reporter.Address(), 1000)

	// badVal registers with a bond.
	reg := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Nonce: 0}
	reg.Sign(badVal)
	applyOne(t, s, 1, blkVal.Address(), reg)

	// badVal double-signs height 9: two different headers.
	hA := types.Header{Height: 9, TxCount: 1}
	hA.Sign(badVal)
	hB := types.Header{Height: 9, TxCount: 2} // differs -> different ID
	hB.Sign(badVal)
	if hA.ID() == hB.ID() {
		t.Fatal("test headers must differ")
	}
	ev, _ := json.Marshal(types.EquivocationEvidence{A: hA, B: hB})

	rBefore := s.Get(reporter.Address()).Balance
	slash := types.Transaction{Kind: types.KindSlash, Data: ev, Nonce: 0}
	slash.Sign(reporter)
	applyOne(t, s, 2, blkVal.Address(), slash)

	vi, ok := s.ValidatorInfo(badVal.Address())
	if !ok || vi.Active || !vi.Slashed || vi.Bond != 0 {
		t.Fatalf("validator not slashed: %+v ok=%v", vi, ok)
	}
	// Reporter got the 10% bounty.
	if got := s.Get(reporter.Address()).Balance - rBefore; got != economy.MinBond/10 {
		t.Fatalf("reporter bounty = %d, want %d", got, economy.MinBond/10)
	}
	// Slashed validator is barred from re-registering.
	s.Credit(badVal.Address(), economy.MinBond)
	reg2 := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Nonce: 1}
	reg2.Sign(badVal)
	blk := &types.Block{Header: types.Header{Height: 3, Validator: blkVal.Address()}, Txs: []types.Transaction{reg2}}
	if err := s.ApplyBlock(blk); err != ErrAlreadyValidator {
		t.Fatalf("slashed validator re-registered: %v", err)
	}
}

func TestSlashBadEvidenceRejected(t *testing.T) {
	v, _ := crypto.Generate()
	reporter, _ := crypto.Generate()
	blkVal, _ := crypto.Generate()
	s := New()
	s.Credit(v.Address(), economy.MinBond)
	reg := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Nonce: 0}
	reg.Sign(v)
	applyOne(t, s, 1, blkVal.Address(), reg)

	// Two IDENTICAL headers (not equivocation) -> rejected.
	h := types.Header{Height: 5}
	h.Sign(v)
	ev, _ := json.Marshal(types.EquivocationEvidence{A: h, B: h})
	slash := types.Transaction{Kind: types.KindSlash, Data: ev, Nonce: 0}
	slash.Sign(reporter)
	blk := &types.Block{Header: types.Header{Height: 2, Validator: blkVal.Address()}, Txs: []types.Transaction{slash}}
	if err := s.ApplyBlock(blk); err != ErrBadEvidence {
		t.Fatalf("want ErrBadEvidence, got %v", err)
	}
	if vi, _ := s.ValidatorInfo(v.Address()); !vi.Active {
		t.Fatal("validator wrongly slashed on bad evidence")
	}
}

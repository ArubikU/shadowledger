package state

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/economy"
	"github.com/ArubikU/shadowledger/internal/regpow"
	"github.com/ArubikU/shadowledger/internal/types"
)

func TestRegisterUnregisterValidator(t *testing.T) {
	node, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(node.Address(), economy.MinBond+1000)

	// Register with a sufficient bond.
	reg := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Fee: 10, Nonce: 0}
	reg.Sign(node)
	applyOne(t, s, 1, val.Address(), reg)

	if vi, ok := s.ValidatorInfo(node.Address()); !ok || !vi.Active || vi.Bond != economy.MinBond {
		t.Fatalf("validator not registered correctly: %+v ok=%v", vi, ok)
	}
	// Bond + fee left the balance (fee goes to block validator).
	if got := s.Get(node.Address()).Balance; got != 990 {
		t.Fatalf("balance after register = %d, want 990", got)
	}
	found := false
	for _, a := range s.ActiveValidators() {
		if a == node.Address() {
			found = true
		}
	}
	if !found {
		t.Fatal("registered node missing from ActiveValidators")
	}

	// Unregister returns the bond.
	un := types.Transaction{Kind: types.KindUnregister, Nonce: 1, Fee: 5}
	un.Sign(node)
	applyOne(t, s, 2, val.Address(), un)
	if _, ok := s.ValidatorInfo(node.Address()); ok {
		t.Fatal("validator still registered after unregister")
	}
	// 990 - 5 fee + MinBond returned
	if got := s.Get(node.Address()).Balance; got != 990-5+economy.MinBond {
		t.Fatalf("balance after unregister = %d", got)
	}
}

func TestRegisterRequiresPoW(t *testing.T) {
	node, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.SetRegPoWBits(12) // enable the Sybil gate
	s.Credit(node.Address(), economy.MinBond+1000)

	// No PoW nonce -> rejected.
	bad := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Nonce: 0}
	bad.Sign(node)
	blk := &types.Block{Header: types.Header{Height: 1, Validator: val.Address()}, Txs: []types.Transaction{bad}}
	if err := s.ApplyBlock(blk); err != ErrBadRegPoW {
		t.Fatalf("want ErrBadRegPoW, got %v", err)
	}
	if _, ok := s.ValidatorInfo(node.Address()); ok {
		t.Fatal("registered without PoW")
	}

	// Solved PoW nonce -> accepted.
	nonce := regpow.Solve(s.ChainID, node.Address(), 12)
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, nonce)
	good := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond, Data: data, Nonce: 0}
	good.Sign(node)
	applyOne(t, s, 2, val.Address(), good)
	if vi, ok := s.ValidatorInfo(node.Address()); !ok || !vi.Active {
		t.Fatal("valid PoW registration not accepted")
	}
}

func TestRegisterBelowMinBondRejected(t *testing.T) {
	node, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(node.Address(), economy.MinBond)

	reg := types.Transaction{Kind: types.KindRegister, Amount: economy.MinBond - 1, Nonce: 0}
	reg.Sign(node)
	blk := &types.Block{Header: types.Header{Height: 1, Validator: val.Address()}, Txs: []types.Transaction{reg}}
	if err := s.ApplyBlock(blk); err != ErrBondTooLow {
		t.Fatalf("want ErrBondTooLow, got %v", err)
	}
	// Block rejected atomically: nothing registered, balance intact.
	if _, ok := s.ValidatorInfo(node.Address()); ok {
		t.Fatal("validator registered despite low bond")
	}
	if s.Get(node.Address()).Balance != economy.MinBond {
		t.Fatal("balance mutated on rejected block")
	}
}

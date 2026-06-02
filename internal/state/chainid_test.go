package state

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
)

func TestChainIDReplayProtection(t *testing.T) {
	a, _ := crypto.Generate()
	b, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.SetChainID(1)
	s.Credit(a.Address(), 1000)

	// Wrong chain id -> rejected by CheckTx and ApplyBlock.
	wrong := types.Transaction{To: b.Address(), Amount: 1, ChainID: 2, Nonce: 0}
	wrong.Sign(a)
	if err := s.CheckTx(&wrong); err != ErrBadChainID {
		t.Fatalf("CheckTx wrong chain: got %v", err)
	}
	blk := &types.Block{Header: types.Header{Height: 1, Validator: val.Address()}, Txs: []types.Transaction{wrong}}
	if err := s.ApplyBlock(blk); err != ErrBadChainID {
		t.Fatalf("ApplyBlock wrong chain: got %v", err)
	}
	if s.Get(a.Address()).Balance != 1000 {
		t.Fatal("balance mutated on rejected cross-chain tx")
	}

	// Correct chain id -> accepted.
	ok := types.Transaction{To: b.Address(), Amount: 1, ChainID: 1, Nonce: 0}
	ok.Sign(a)
	if err := s.CheckTx(&ok); err != nil {
		t.Fatalf("CheckTx right chain: %v", err)
	}
}

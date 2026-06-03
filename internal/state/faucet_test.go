package state

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/faucet"
	"github.com/ArubikU/shadowledger/internal/types"
)

// faucetState builds a state with a funded treasury and a faucet configured
// against a single known anchor (block 4). Returns the state, treasury, anchor.
func faucetState(t *testing.T, bits int) (*State, *crypto.KeyPair, [32]byte) {
	t.Helper()
	treasury, _ := crypto.Generate()
	s := New()
	s.Credit(treasury.Address(), 1_000_000_000)
	var anchor [32]byte
	copy(anchor[:], []byte("body-hash-of-block-4............."))
	anchors := map[uint64][32]byte{4: anchor}
	s.SetFaucet(bits, 200_000_000, 50, 4, treasury.Address(), func(h uint64) ([32]byte, bool) {
		v, ok := anchors[h]
		return v, ok
	})
	return s, treasury, anchor
}

func faucetTx(t *testing.T, claimer *crypto.KeyPair, anchor [32]byte, anchorHeight uint64, bits int) types.Transaction {
	t.Helper()
	nonce := faucet.Solve(0, claimer.Address(), anchor, bits)
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:8], anchorHeight)
	binary.BigEndian.PutUint64(data[8:16], nonce)
	tx := types.Transaction{Kind: types.KindFaucet, Data: data, Nonce: 0}
	tx.Sign(claimer)
	return tx
}

func TestFaucetClaimPays(t *testing.T) {
	s, treasury, anchor := faucetState(t, 10)
	claimer, _ := crypto.Generate()
	val, _ := crypto.Generate()

	tx := faucetTx(t, claimer, anchor, 4, 10)
	applyOne(t, s, 10, val.Address(), tx) // anchor 4 is 6 deep (>= FaucetDepth 4)

	if got := s.Get(claimer.Address()).Balance; got != 200_000_000 {
		t.Fatalf("claimer balance = %d, want 200000000", got)
	}
	if got := s.Get(treasury.Address()).Balance; got != 1_000_000_000-200_000_000 {
		t.Fatalf("treasury balance = %d, want %d", got, 1_000_000_000-200_000_000)
	}
}

func TestFaucetRejectsBadPoW(t *testing.T) {
	s, _, anchor := faucetState(t, 12)
	claimer, _ := crypto.Generate()
	val, _ := crypto.Generate()

	tx := faucetTx(t, claimer, anchor, 4, 12)
	// Corrupt the nonce so the PoW no longer holds.
	binary.BigEndian.PutUint64(tx.Data[8:16], 0)
	tx.Sign(claimer)
	blk := &types.Block{Header: types.Header{Height: 10, Validator: val.Address()}, Txs: []types.Transaction{tx}}
	if err := s.ApplyBlock(blk); err != ErrBadFaucetPoW {
		t.Fatalf("got %v, want ErrBadFaucetPoW", err)
	}
}

func TestFaucetCooldown(t *testing.T) {
	s, _, anchor := faucetState(t, 10)
	claimer, _ := crypto.Generate()
	val, _ := crypto.Generate()

	applyOne(t, s, 10, val.Address(), faucetTx(t, claimer, anchor, 4, 10))
	// Second claim within the 50-block cooldown (height 30) must be refused.
	tx2 := faucetTx(t, claimer, anchor, 4, 10)
	tx2.Nonce = 1
	tx2.Sign(claimer)
	blk := &types.Block{Header: types.Header{Height: 30, Validator: val.Address()}, Txs: []types.Transaction{tx2}}
	if err := s.ApplyBlock(blk); err != ErrFaucetCooldown {
		t.Fatalf("got %v, want ErrFaucetCooldown", err)
	}
	// After the cooldown (height 70) it succeeds again.
	tx3 := faucetTx(t, claimer, anchor, 4, 10)
	tx3.Nonce = 1
	tx3.Sign(claimer)
	applyOne(t, s, 70, val.Address(), tx3)
	if got := s.Get(claimer.Address()).Balance; got != 400_000_000 {
		t.Fatalf("after second claim balance = %d, want 400000000", got)
	}
}

func TestFaucetStaleAnchorRejected(t *testing.T) {
	s, _, anchor := faucetState(t, 10)
	claimer, _ := crypto.Generate()
	val, _ := crypto.Generate()

	// Anchor 4 only 2 deep at height 6 (< FaucetDepth 4) -> rejected as unconfirmed.
	tx := faucetTx(t, claimer, anchor, 4, 10)
	blk := &types.Block{Header: types.Header{Height: 6, Validator: val.Address()}, Txs: []types.Transaction{tx}}
	if err := s.ApplyBlock(blk); err != ErrBadFaucet {
		t.Fatalf("got %v, want ErrBadFaucet", err)
	}
}

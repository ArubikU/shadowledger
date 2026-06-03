package state

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
)

// spState builds a state with one active validator and a stub storage-proof
// oracle: ShardProofOK accepts only the bytes "GOODSHARD".
func spState(t *testing.T) (*State, *crypto.KeyPair) {
	t.Helper()
	val, _ := crypto.Generate()
	s := New()
	s.RegisterGenesisValidator(val.Address())
	var bid [32]byte
	copy(bid[:], []byte("a-block-id-for-the-challenge...."))
	s.SetStorageProof(256, 4, 64,
		func(h uint64) ([32]byte, int, bool) { return bid, 4, true },
		func(h uint64, i int, shard []byte) bool { return string(shard) == "GOODSHARD" })
	return s, val
}

func spTx(t *testing.T, val *crypto.KeyPair, target uint64, shard string, nonce uint64) types.Transaction {
	t.Helper()
	data := make([]byte, 8+len(shard))
	binary.BigEndian.PutUint64(data[0:8], target)
	copy(data[8:], shard)
	tx := types.Transaction{Kind: types.KindStorageProof, Data: data, Nonce: nonce}
	tx.Sign(val)
	return tx
}

func TestStorageProofCreditsAndWeights(t *testing.T) {
	s, val := spState(t)
	prod, _ := crypto.Generate()

	// Weight starts at the base (1) with no proofs.
	if w := s.StorageWeights()[val.Address()]; w != 1 {
		t.Fatalf("initial weight = %d, want 1", w)
	}
	// Valid proof at height 10 targeting block 4 (6 deep, within window) is credited.
	applyOne(t, s, 10, prod.Address(), spTx(t, val, 4, "GOODSHARD", 0))
	if sc := s.Validators[val.Address()].StorageScore; sc != 1 {
		t.Fatalf("storage score = %d, want 1", sc)
	}
	if w := s.StorageWeights()[val.Address()]; w != 2 {
		t.Fatalf("weight after one proof = %d, want 2", w)
	}
}

func TestStorageProofRejectsBadShard(t *testing.T) {
	s, val := spState(t)
	prod, _ := crypto.Generate()
	tx := spTx(t, val, 4, "WRONG", 0)
	blk := &types.Block{Header: types.Header{Height: 10, Validator: prod.Address()}, Txs: []types.Transaction{tx}}
	if err := s.ApplyBlock(blk); err != ErrBadStorageProof {
		t.Fatalf("got %v, want ErrBadStorageProof", err)
	}
}

func TestStorageProofRejectsStaleAndShallow(t *testing.T) {
	s, val := spState(t)
	prod, _ := crypto.Generate()
	// Too shallow: target 9 at height 10 is only 1 deep (< minDepth 4).
	shallow := &types.Block{Header: types.Header{Height: 10, Validator: prod.Address()}, Txs: []types.Transaction{spTx(t, val, 9, "GOODSHARD", 0)}}
	if err := s.ApplyBlock(shallow); err != ErrBadStorageProof {
		t.Fatalf("shallow: got %v, want ErrBadStorageProof", err)
	}
	// Stale: target 1 at height 300 is older than the 256 window.
	stale := &types.Block{Header: types.Header{Height: 300, Validator: prod.Address()}, Txs: []types.Transaction{spTx(t, val, 1, "GOODSHARD", 0)}}
	if err := s.ApplyBlock(stale); err != ErrBadStorageProof {
		t.Fatalf("stale: got %v, want ErrBadStorageProof", err)
	}
}

func TestStorageWeightDecays(t *testing.T) {
	s, val := spState(t)
	prod, _ := crypto.Generate()
	applyOne(t, s, 10, prod.Address(), spTx(t, val, 4, "GOODSHARD", 0)) // LastProofAt=10
	if w := s.StorageWeights()[val.Address()]; w != 2 {
		t.Fatalf("fresh weight = %d, want 2", w)
	}
	// Advance head far past the window without new proofs: bonus decays to base.
	s.Height = 10 + 256 + 1
	if w := s.StorageWeights()[val.Address()]; w != 1 {
		t.Fatalf("decayed weight = %d, want 1 (bonus expired)", w)
	}
}

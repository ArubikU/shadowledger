package types

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
)

func TestTxSignVerifyAndTamper(t *testing.T) {
	kp, _ := crypto.Generate()
	to, _ := crypto.Generate()
	tx := Transaction{To: to.Address(), Amount: 42, Nonce: 0}
	tx.Sign(kp)
	if err := tx.VerifySig(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tx.From != kp.Address() {
		t.Fatal("From not set to signer address")
	}
	tx.Amount = 43 // tamper
	if err := tx.VerifySig(); err == nil {
		t.Fatal("tampered tx verified")
	}
}

func TestTxEncodeDecodeRoundTrip(t *testing.T) {
	kp, _ := crypto.Generate()
	to, _ := crypto.Generate()
	txs := []Transaction{}
	for i := uint64(0); i < 5; i++ {
		tx := Transaction{To: to.Address(), Amount: 100 + i, Nonce: i}
		tx.Sign(kp)
		txs = append(txs, tx)
	}
	body := EncodeTxs(txs)
	got, err := DecodeTxs(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(txs) {
		t.Fatalf("count %d != %d", len(got), len(txs))
	}
	for i := range txs {
		if got[i].Hash() != txs[i].Hash() {
			t.Fatalf("tx %d hash mismatch after round trip", i)
		}
	}
}

func TestHeaderSignVerify(t *testing.T) {
	kp, _ := crypto.Generate()
	h := Header{Height: 7, TxCount: 3, Spec: ShardSpec{K: 4, M: 2}}
	h.Sign(kp)
	if err := h.VerifySig(); err != nil {
		t.Fatalf("header verify: %v", err)
	}
	h.Height = 8
	if err := h.VerifySig(); err == nil {
		t.Fatal("tampered header verified")
	}
}

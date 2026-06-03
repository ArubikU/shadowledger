package faucet

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
)

func TestSolveVerify(t *testing.T) {
	kp, _ := crypto.Generate()
	var anchor [32]byte
	copy(anchor[:], []byte("a-recent-block-body-hash-32bytes"))
	const bits = 12
	nonce := Solve(1, kp.Address(), anchor, bits)
	if !Verify(1, kp.Address(), anchor, nonce, bits) {
		t.Fatal("solved nonce does not verify")
	}
}

func TestVerifyRejectsWrongAnchorOrAddr(t *testing.T) {
	a, _ := crypto.Generate()
	b, _ := crypto.Generate()
	var anchor, other [32]byte
	copy(anchor[:], []byte("anchor-one......................"))
	copy(other[:], []byte("anchor-two......................"))
	const bits = 12
	nonce := Solve(1, a.Address(), anchor, bits)

	if Verify(1, a.Address(), other, nonce, bits) {
		t.Fatal("accepted a nonce mined against a different anchor")
	}
	if Verify(1, b.Address(), anchor, nonce, bits) {
		t.Fatal("accepted another address's nonce (solution not address-bound)")
	}
	if Verify(2, a.Address(), anchor, nonce, bits) {
		t.Fatal("accepted a nonce from a different chain id")
	}
}

func TestBitsZeroOpen(t *testing.T) {
	kp, _ := crypto.Generate()
	var anchor [32]byte
	if !Verify(1, kp.Address(), anchor, 0, 0) {
		t.Fatal("bits=0 should accept any nonce")
	}
}

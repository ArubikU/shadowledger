package regpow

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
)

func TestSolveVerify(t *testing.T) {
	kp, _ := crypto.Generate()
	const bits = 12 // small for a fast test
	nonce := Solve(1, kp.Address(), bits)
	if !Verify(1, kp.Address(), nonce, bits) {
		t.Fatal("solved nonce failed verify")
	}
	// Wrong nonce almost certainly fails 12 leading zero bits.
	if Verify(1, kp.Address(), nonce+1, bits) && Verify(1, kp.Address(), nonce+2, bits) {
		t.Fatal("arbitrary nonces passed — difficulty not enforced")
	}
	// Bound to chain + address: same nonce on another chain/addr shouldn't pass.
	other, _ := crypto.Generate()
	if Verify(1, other.Address(), nonce, bits) {
		t.Fatal("nonce reusable across addresses")
	}
	if Verify(2, kp.Address(), nonce, bits) {
		t.Fatal("nonce reusable across chains")
	}
}

func TestBitsZeroDisables(t *testing.T) {
	kp, _ := crypto.Generate()
	if !Verify(1, kp.Address(), 0, 0) {
		t.Fatal("bits=0 should disable the puzzle")
	}
}

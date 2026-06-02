package state

import (
	"os"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/shl"
	"github.com/ArubikU/shadowledger/internal/types"
)

// compileFile compiles a .shl contract from the repo's contracts/ dir.
func compileFile(t *testing.T, rel string) []byte {
	t.Helper()
	src, err := os.ReadFile("../../contracts/" + rel)
	if err != nil {
		t.Skipf("contract source not found: %v", err)
	}
	code, err := shl.Compile(string(src))
	if err != nil {
		t.Fatalf("compile %s: %v", rel, err)
	}
	return code
}

func TestTokenContractEndToEnd(t *testing.T) {
	dev, _ := crypto.Generate()
	bob, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)

	code := compileFile(t, "token.shl")
	dep := types.Transaction{Kind: types.KindDeploy, Data: code, Nonce: 0}
	dep.Sign(dev)
	applyOne(t, s, 1, val.Address(), dep)
	tok := types.ContractAddress(dev.Address(), 0)

	devDig := types.AddrDigest(dev.Address())
	bobDig := types.AddrDigest(bob.Address())

	call := func(by *crypto.KeyPair, nonce uint64, w ...uint64) {
		tx := types.Transaction{To: tok, Kind: types.KindCall, Gas: 1_000_000, Nonce: nonce, Data: words(w...)}
		tx.Sign(by)
		applyOne(t, s, nonce+1, val.Address(), tx)
	}
	bal := func(dig uint64) uint64 {
		r, _ := s.QueryContract(tok, 0, words(2, dig), 0)
		return r
	}

	// init: dev gets the whole supply (dev nonce 1 — nonce 0 was the deploy).
	call(dev, 1, 0)
	if bal(devDig) != 1_000_000 {
		t.Fatalf("after init dev balance = %d", bal(devDig))
	}
	if total, _ := s.QueryContract(tok, 0, words(3), 0); total != 1_000_000 {
		t.Fatalf("totalSupply = %d", total)
	}

	// transfer 250k dev -> bob.
	call(dev, 2, 1, bobDig, 250_000)
	if bal(devDig) != 750_000 || bal(bobDig) != 250_000 {
		t.Fatalf("after transfer dev=%d bob=%d", bal(devDig), bal(bobDig))
	}

	// overdraw: bob tries to send 999k he doesn't have -> no-op.
	call(bob, 0, 1, devDig, 999_000) // bob's first tx, nonce 0
	if bal(bobDig) != 250_000 {
		t.Fatalf("overdraw changed bob balance: %d", bal(bobDig))
	}
}

func TestMemecoinBurn(t *testing.T) {
	dev, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)

	code := compileFile(t, "memecoin.shl")
	dep := types.Transaction{Kind: types.KindDeploy, Data: code, Nonce: 0}
	dep.Sign(dev)
	applyOne(t, s, 1, val.Address(), dep)
	tok := types.ContractAddress(dev.Address(), 0)
	devDig := types.AddrDigest(dev.Address())

	call := func(nonce uint64, w ...uint64) {
		tx := types.Transaction{To: tok, Kind: types.KindCall, Gas: 1_000_000, Nonce: nonce, Data: words(w...)}
		tx.Sign(dev)
		applyOne(t, s, nonce+1, val.Address(), tx)
	}
	total := func() uint64 { r, _ := s.QueryContract(tok, 0, words(4), 0); return r }
	bal := func() uint64 { r, _ := s.QueryContract(tok, 0, words(2, devDig), 0); return r }

	call(1, 0)             // init -> 21,000,000 (nonce 1; nonce 0 was the deploy)
	call(2, 3, 0, 1000000) // burn 1,000,000
	if bal() != 20_000_000 || total() != 20_000_000 {
		t.Fatalf("after burn balance=%d total=%d", bal(), total())
	}
}

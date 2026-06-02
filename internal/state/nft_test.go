package state

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/vm"
)

// --- tiny label-resolving assembler (test only) ---

type asm struct {
	code   []byte
	labels map[string]int
	fixups []fixup
}
type fixup struct {
	pos  int
	name string
}

func newAsm() *asm { return &asm{labels: map[string]int{}} }
func be8(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
func words(vs ...uint64) []byte {
	var o []byte
	for _, v := range vs {
		o = append(o, be8(v)...)
	}
	return o
}
func (a *asm) op(b byte) *asm { a.code = append(a.code, b); return a }
func (a *asm) push(v uint64) *asm {
	a.code = append(a.code, vm.PUSH)
	a.code = append(a.code, be8(v)...)
	return a
}
func (a *asm) pushLabel(name string) *asm {
	a.code = append(a.code, vm.PUSH)
	a.fixups = append(a.fixups, fixup{len(a.code), name})
	a.code = append(a.code, make([]byte, 8)...)
	return a
}
func (a *asm) label(name string) *asm { a.labels[name] = len(a.code); return a }
func (a *asm) build() []byte {
	for _, f := range a.fixups {
		binary.BigEndian.PutUint64(a.code[f.pos:f.pos+8], uint64(a.labels[f.name]))
	}
	return a.code
}

// nftCode: a minimal NFT. Input words: [selector, tokenId, newOwnerDigest].
//
//	selector 0 = mint     : storage[tokenId] = CALLER
//	selector 1 = transfer : require storage[tokenId]==CALLER; storage[tokenId]=newOwner (reverts if not owner)
//	selector 2 = ownerOf  : RETURN storage[tokenId]
func nftCode() []byte {
	a := newAsm()
	// if selector==0 goto mint
	a.push(0).op(vm.CDLOAD).op(vm.ISZERO).pushLabel("mint").op(vm.JUMPI)
	// if selector==1 goto transfer
	a.push(0).op(vm.CDLOAD).push(1).op(vm.EQ).pushLabel("transfer").op(vm.JUMPI)
	// else ownerOf: RETURN storage[tokenId]
	a.push(1).op(vm.CDLOAD).op(vm.SLOAD).op(vm.RETURN)

	a.label("mint")
	a.push(1).op(vm.CDLOAD). // key = tokenId
					op(vm.CALLER). // value = caller
					op(vm.SSTORE). // storage[tokenId] = caller
					op(vm.STOP)

	a.label("transfer")
	// require storage[tokenId] == CALLER
	a.push(1).op(vm.CDLOAD).op(vm.SLOAD). // [owner]
						op(vm.CALLER). // [owner, caller]
						op(vm.EQ).     // [owner==caller]
						op(vm.ISZERO). // [notOwner]
						pushLabel("deny").op(vm.JUMPI)
	// storage[tokenId] = newOwner
	a.push(1).op(vm.CDLOAD).push(2).op(vm.CDLOAD).op(vm.SSTORE)
	a.push(1).op(vm.RETURN) // success
	a.label("deny")
	a.push(1).push(0).op(vm.DIV) // 1/0 -> revert (unauthorized transfer fails hard)
	return a.build()
}

func TestNFTMintTransferOwnerOf(t *testing.T) {
	dev, _ := crypto.Generate()
	alice, _ := crypto.Generate()
	bob, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)
	s.Credit(alice.Address(), 1_000_000)
	s.Credit(bob.Address(), 1_000_000)

	// Deploy.
	dep := types.Transaction{Kind: types.KindDeploy, Data: nftCode(), Nonce: 0}
	dep.Sign(dev)
	applyOne(t, s, 1, val.Address(), dep)
	nft := types.ContractAddress(dev.Address(), 0)

	const tokenID = 7
	mint := func(by *crypto.KeyPair, nonce uint64) types.Transaction {
		tx := types.Transaction{To: nft, Kind: types.KindCall, Gas: 100000, Nonce: nonce,
			Data: words(0, tokenID, 0)}
		tx.Sign(by)
		return tx
	}

	// Alice mints token 7.
	applyOne(t, s, 2, val.Address(), mint(alice, 0))
	if got := s.Get(nft).Storage["7"]; got != types.AddrDigest(alice.Address()) {
		t.Fatalf("after mint, owner=%d want alice=%d", got, types.AddrDigest(alice.Address()))
	}

	// Alice transfers token 7 to Bob.
	tr := types.Transaction{To: nft, Kind: types.KindCall, Gas: 100000, Nonce: 1,
		Data: words(1, tokenID, types.AddrDigest(bob.Address()))}
	tr.Sign(alice)
	applyOne(t, s, 3, val.Address(), tr)
	if got := s.Get(nft).Storage["7"]; got != types.AddrDigest(bob.Address()) {
		t.Fatalf("after transfer, owner=%d want bob=%d", got, types.AddrDigest(bob.Address()))
	}

	// Read-only ownerOf query (no tx) returns Bob, mutates nothing.
	ret, ok := s.QueryContract(nft, 0, words(2, tokenID, 0), 0)
	if !ok || ret != types.AddrDigest(bob.Address()) {
		t.Fatalf("ownerOf query = %d ok=%v, want bob=%d", ret, ok, types.AddrDigest(bob.Address()))
	}

	// Alice (no longer owner) tries to transfer again -> must revert (storage unchanged).
	tr2 := types.Transaction{To: nft, Kind: types.KindCall, Gas: 100000, Nonce: 2, Fee: 50,
		Data: words(1, tokenID, types.AddrDigest(alice.Address()))}
	tr2.Sign(alice)
	applyOne(t, s, 4, val.Address(), tr2) // tx applies; the VM call reverts internally
	if got := s.Get(nft).Storage["7"]; got != types.AddrDigest(bob.Address()) {
		t.Fatalf("unauthorized transfer changed ownership: %d", got)
	}
}

// TestGasOutOfGasReverts shows gas metering: a storage write costs 100 gas, so a
// call with too little gas reverts and leaves storage untouched.
func TestGasOutOfGasReverts(t *testing.T) {
	dev, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)

	// code: storage[0] = 42  (PUSH0 PUSH42 SSTORE STOP) — SSTORE costs 100 gas.
	a := newAsm()
	a.push(0).push(42).op(vm.SSTORE).op(vm.STOP)
	dep := types.Transaction{Kind: types.KindDeploy, Data: a.build(), Nonce: 0}
	dep.Sign(dev)
	applyOne(t, s, 1, val.Address(), dep)
	c := types.ContractAddress(dev.Address(), 0)

	// Call with only 50 gas -> out of gas before SSTORE -> revert.
	low := types.Transaction{To: c, Kind: types.KindCall, Gas: 50, Nonce: 1}
	low.Sign(dev)
	applyOne(t, s, 2, val.Address(), low)
	if _, ok := s.Get(c).Storage["0"]; ok {
		t.Fatal("storage written despite out-of-gas (should revert)")
	}

	// Call with enough gas -> succeeds.
	ok := types.Transaction{To: c, Kind: types.KindCall, Gas: 100000, Nonce: 2}
	ok.Sign(dev)
	applyOne(t, s, 3, val.Address(), ok)
	if s.Get(c).Storage["0"] != 42 {
		t.Fatal("storage not written with sufficient gas")
	}
}

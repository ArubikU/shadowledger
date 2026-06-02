package state

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/vm"
)

// Full-address ERC-721 using the byte layer (no digests):
//
//	arg(0)=selector, arg(1)=tokenId.
//	0 = mint     : bowner[tokenId] = caller's FULL address (CALLERB)
//	1 = transfer : require bowner[tokenId]==caller (BEQ); then bowner[tokenId]=
//	               the new owner address taken from calldata bytes [16:16+len]
//	2 = ownerOf  : RETURNB bowner[tokenId]  (returns the full owner address)
//
// tokenId occupies bkey = arg(1). A temp bkey (2^63) holds the caller for compare
// and the transfer recipient is read from calldata after the 2 arg words.
func erc721FullCode() []byte {
	a := newAsm()
	const tmp = 1 << 63
	// dispatch on selector
	a.push(0).op(vm.CDLOAD).op(vm.ISZERO).pushLabel("mint").op(vm.JUMPI)
	a.push(0).op(vm.CDLOAD).push(1).op(vm.EQ).pushLabel("xfer").op(vm.JUMPI)
	// ownerOf: RETURNB bowner[tokenId]
	a.push(1).op(vm.CDLOAD).op(vm.RETURNB)

	a.label("mint")                        // bowner[tokenId] = CALLER full addr
	a.push(1).op(vm.CDLOAD).op(vm.CALLERB) // pop bkey=tokenId -> store caller addr
	a.op(vm.STOP)

	a.label("xfer")
	// load caller into tmp, compare to owner
	a.push(tmp).op(vm.CALLERB)
	a.push(1).op(vm.CDLOAD). // bkeyA = tokenId (owner)
					push(tmp). // bkeyB = tmp (caller)
					op(vm.BEQ).op(vm.ISZERO).pushLabel("deny").op(vm.JUMPI)
	// new owner = calldata bytes at offset 16 (after 2 words), length 42 (sl+40hex).
	// BSTORE pops bkey, off, len (top first), so push len, off, bkey in that order.
	a.push(42). // len (address string length)
			push(16).              // offset
			push(1).op(vm.CDLOAD). // bkey = tokenId (on top)
			op(vm.BSTORE)
	a.op(vm.STOP)
	a.label("deny")
	a.push(1).push(0).op(vm.DIV) // revert
	return a.build()
}

func TestERC721FullAddress(t *testing.T) {
	dev, _ := crypto.Generate()
	alice, _ := crypto.Generate()
	bob, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)
	s.Credit(alice.Address(), 1_000_000)

	dep := types.Transaction{Kind: types.KindDeploy, Data: erc721FullCode(), Nonce: 0}
	dep.Sign(dev)
	applyOne(t, s, 1, val.Address(), dep)
	nft := types.ContractAddress(dev.Address(), 0)

	const tok = 7
	// alice mints token 7 -> bowner[7] = alice's full address
	mint := types.Transaction{To: nft, Kind: types.KindCall, Gas: 1_000_000, Nonce: 0, Data: words(0, tok)}
	mint.Sign(alice)
	applyOne(t, s, 2, val.Address(), mint)

	// ownerOf(7) returns alice's FULL address (not a digest).
	_, owner, ok := s.QueryContractRaw(nft, 0, words(2, tok), 0)
	if !ok || string(owner) != string(alice.Address()) {
		t.Fatalf("ownerOf = %q, want %q", string(owner), string(alice.Address()))
	}

	// alice transfers 7 to bob: calldata = [sel=1][tok][bob-address-bytes].
	data := append(words(1, tok), []byte(bob.Address())...)
	xfer := types.Transaction{To: nft, Kind: types.KindCall, Gas: 1_000_000, Nonce: 1, Data: data}
	xfer.Sign(alice)
	applyOne(t, s, 3, val.Address(), xfer)

	_, owner2, _ := s.QueryContractRaw(nft, 0, words(2, tok), 0)
	if string(owner2) != string(bob.Address()) {
		t.Fatalf("after transfer ownerOf = %q, want bob %q", string(owner2), string(bob.Address()))
	}
}

package state

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
	"github.com/ArubikU/shadowledger/internal/vm"
)

func u64b(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
func push(v uint64) []byte { return append([]byte{vm.PUSH}, u64b(v)...) }

// counterCode: storage[0] = storage[0] + 1
func counterCode() []byte {
	var c []byte
	c = append(c, push(0)...) // SSTORE key (under value)
	c = append(c, push(0)...) // SLOAD key
	c = append(c, vm.SLOAD)
	c = append(c, push(1)...)
	c = append(c, vm.ADD)
	c = append(c, vm.SSTORE)
	c = append(c, vm.STOP)
	return c
}

func applyOne(t *testing.T, s *State, height uint64, val crypto.Address, tx types.Transaction) {
	t.Helper()
	blk := &types.Block{
		Header: types.Header{Height: height, Validator: val},
		Txs:    []types.Transaction{tx},
	}
	if err := s.ApplyBlock(blk); err != nil {
		t.Fatalf("apply height %d: %v", height, err)
	}
}

func TestDeployAndCallCounter(t *testing.T) {
	dev, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)

	// Deploy at nonce 0.
	deploy := types.Transaction{Kind: types.KindDeploy, Data: counterCode(), Nonce: 0}
	deploy.Sign(dev)
	applyOne(t, s, 1, val.Address(), deploy)

	caddr := types.ContractAddress(dev.Address(), 0)
	if !s.IsContract(caddr) {
		t.Fatal("contract not created")
	}

	// Call it three times; storage[0] should reach 3.
	for i := uint64(1); i <= 3; i++ {
		call := types.Transaction{To: caddr, Kind: types.KindCall, Nonce: i, Gas: 10000}
		call.Sign(dev)
		applyOne(t, s, i+1, val.Address(), call)
		if got := s.Get(caddr).Storage["0"]; got != i {
			t.Fatalf("after %d calls storage[0]=%d, want %d", i, got, i)
		}
	}
}

func TestCallValueAndRevertRefund(t *testing.T) {
	dev, _ := crypto.Generate()
	val, _ := crypto.Generate()
	s := New()
	s.Credit(dev.Address(), 1_000_000)

	// Deploy a contract that always reverts (DIV by zero).
	var code []byte
	code = append(code, push(0)...)
	code = append(code, push(1)...)
	code = append(code, push(0)...)
	code = append(code, vm.DIV) // revert
	code = append(code, vm.SSTORE)
	deploy := types.Transaction{Kind: types.KindDeploy, Data: code, Nonce: 0}
	deploy.Sign(dev)
	applyOne(t, s, 1, val.Address(), deploy)
	caddr := types.ContractAddress(dev.Address(), 0)

	before := s.Get(dev.Address()).Balance
	// Call sending value 500 with fee 10; contract reverts -> value refunded, fee kept.
	call := types.Transaction{To: caddr, Kind: types.KindCall, Amount: 500, Fee: 10, Nonce: 1, Gas: 10000}
	call.Sign(dev)
	applyOne(t, s, 2, val.Address(), call)

	after := s.Get(dev.Address()).Balance
	if before-after != 10 {
		t.Fatalf("revert should cost only the fee; lost %d", before-after)
	}
	if s.Get(caddr).Balance != 0 {
		t.Fatalf("reverted contract kept value: %d", s.Get(caddr).Balance)
	}
	if got := val.Address(); s.Get(got).Balance != 10 {
		t.Fatalf("validator fee = %d, want 10", s.Get(got).Balance)
	}
}

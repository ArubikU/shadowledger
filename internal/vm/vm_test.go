package vm

import (
	"encoding/binary"
	"testing"
)

func u64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// asm builds bytecode; PUSH is followed by an 8-byte immediate via push().
func push(v uint64) []byte { return append([]byte{PUSH}, u64(v)...) }

func TestArithmeticReturn(t *testing.T) {
	// (2 + 3) * 4 = 20
	var code []byte
	code = append(code, push(2)...)
	code = append(code, push(3)...)
	code = append(code, ADD)
	code = append(code, push(4)...)
	code = append(code, MUL)
	code = append(code, RETURN)

	res, err := Execute(code, nil, map[uint64]uint64{}, 1000, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Return != 20 {
		t.Fatalf("return = %d, want 20", res.Return)
	}
}

func TestStorageCounter(t *testing.T) {
	// storage[0] = storage[0] + 1
	var code []byte
	code = append(code, push(0)...) // key for SSTORE (under value)
	code = append(code, push(0)...) // key for SLOAD
	code = append(code, SLOAD)
	code = append(code, push(1)...)
	code = append(code, ADD)
	code = append(code, SSTORE)
	code = append(code, STOP)

	store := map[uint64]uint64{}
	for i := 1; i <= 3; i++ {
		if _, err := Execute(code, nil, store, 1000, Context{}); err != nil {
			t.Fatal(err)
		}
		if store[0] != uint64(i) {
			t.Fatalf("after %d calls storage[0]=%d", i, store[0])
		}
	}
}

func TestOutOfGasReverts(t *testing.T) {
	var code []byte
	code = append(code, push(0)...)
	code = append(code, push(7)...)
	code = append(code, SSTORE) // costs 100 gas
	store := map[uint64]uint64{}
	_, err := Execute(code, nil, store, 5, Context{}) // not enough gas
	if err != ErrOutOfGas {
		t.Fatalf("err = %v, want ErrOutOfGas", err)
	}
	if len(store) != 0 {
		t.Fatal("storage mutated despite out-of-gas (revert failed)")
	}
}

func TestDivByZeroReverts(t *testing.T) {
	var code []byte
	code = append(code, push(0)...) // key
	code = append(code, push(5)...)
	code = append(code, push(0)...)
	code = append(code, DIV) // 5 / 0 -> revert
	code = append(code, SSTORE)
	store := map[uint64]uint64{}
	if _, err := Execute(code, nil, store, 1000, Context{}); err != ErrDivZero {
		t.Fatalf("err = %v, want ErrDivZero", err)
	}
	if len(store) != 0 {
		t.Fatal("storage mutated despite revert")
	}
}

func TestConditionalJump(t *testing.T) {
	// if input[0] != 0 return 111 else return 222
	// layout: CDLOAD(0); PUSH dest; JUMPI; PUSH 222; RETURN; [dest:] PUSH 111; RETURN
	var code []byte
	code = append(code, push(0)...) // word index 0
	code = append(code, CDLOAD)     // push input[0]
	// compute dest after we know length; we'll patch.
	jmpiPos := len(code)
	code = append(code, push(0)...) // placeholder dest
	code = append(code, JUMPI)
	code = append(code, push(222)...)
	code = append(code, RETURN)
	dest := len(code)
	code = append(code, push(111)...)
	code = append(code, RETURN)
	binary.BigEndian.PutUint64(code[jmpiPos+1:jmpiPos+9], uint64(dest))

	res, err := Execute(code, u64(1), map[uint64]uint64{}, 1000, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Return != 111 {
		t.Fatalf("truthy branch return = %d, want 111", res.Return)
	}
	res, _ = Execute(code, u64(0), map[uint64]uint64{}, 1000, Context{})
	if res.Return != 222 {
		t.Fatalf("falsy branch return = %d, want 222", res.Return)
	}
}

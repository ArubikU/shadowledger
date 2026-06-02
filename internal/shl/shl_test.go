package shl

import (
	"encoding/binary"
	"testing"

	"github.com/ArubikU/shadowledger/internal/vm"
)

func run(t *testing.T, src string, input []byte) (*vm.Result, map[uint64]uint64) {
	t.Helper()
	code, err := Compile(src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	storage := map[uint64]uint64{}
	res, err := vm.Execute(code, input, storage, 10_000_000, vm.Context{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return res, storage
}

func words(vs ...uint64) []byte {
	b := make([]byte, 8*len(vs))
	for i, v := range vs {
		binary.BigEndian.PutUint64(b[i*8:], v)
	}
	return b
}

func TestCompileCounter(t *testing.T) {
	res, st := run(t, `
		store[0] = store[0] + 1;
		return store[0];
	`, nil)
	if st[0] != 1 || res.Return != 1 {
		t.Fatalf("counter: storage=%d return=%d", st[0], res.Return)
	}
}

func TestCompileIfElse(t *testing.T) {
	src := `
		if (arg(0) == 0) {
			store[1] = 100;
		} else {
			store[1] = 200;
		}
	`
	if _, st := run(t, src, words(0)); st[1] != 100 {
		t.Fatalf("if-branch: %d", st[1])
	}
	if _, st := run(t, src, words(5)); st[1] != 200 {
		t.Fatalf("else-branch: %d", st[1])
	}
}

func TestCompileWhileLoop(t *testing.T) {
	_, st := run(t, `
		let i = 0;
		while (i < 5) {
			i = i + 1;
		}
		store[0] = i;
	`, nil)
	if st[0] != 5 {
		t.Fatalf("while: got %d want 5", st[0])
	}
}

func TestCompileBoolAndCompare(t *testing.T) {
	// store[2] = 1 if (a >= 10 && a <= 20) else 0
	src := `
		if (arg(0) >= 10 && arg(0) <= 20) { store[2] = 1; } else { store[2] = 0; }
	`
	if _, st := run(t, src, words(15)); st[2] != 1 {
		t.Fatalf("range in: %d", st[2])
	}
	if _, st := run(t, src, words(25)); st[2] != 0 {
		t.Fatalf("range out: %d", st[2])
	}
}

func TestEstimateNonZero(t *testing.T) {
	code, gas, err := CompileAndEstimate(`store[0] = 1; emit(7, 8);`)
	if err != nil || len(code) == 0 || gas == 0 {
		t.Fatalf("estimate: code=%d gas=%d err=%v", len(code), gas, err)
	}
}

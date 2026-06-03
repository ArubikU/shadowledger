package shl

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArubikU/shadowledger/internal/vm"
)

func TestRealContractsCompile(t *testing.T) {
	dir := filepath.Join("..", "..", "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read contracts dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".shl") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		code, err := Compile(string(src))
		if err != nil {
			t.Fatalf("compile %s: %v", e.Name(), err)
		}
		if len(code) == 0 {
			t.Fatalf("%s compiled to empty bytecode", e.Name())
		}
		n++
	}
	if n == 0 {
		t.Fatal("no .shl contracts found")
	}
}

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

// --- Solidity-like dialect: selector dispatch, mappings, require, checked math ---

const solToken = `
state balances;
state total;

fn init() {
    balances[caller()] = 1000;
    total = 1000;
}
fn transfer(to, amount) {
    require(balances[caller()] >= amount);
    balances[caller()] = balances[caller()] - amount;
    balances[to] = balances[to] + amount;
}
fn balanceOf(who) { return balances[who]; }
fn supply() { return total; }
`

func TestSolidityDispatchAndMapping(t *testing.T) {
	code, err := Compile(solToken)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	storage := map[uint64]uint64{}
	call := func(name string, args ...uint64) *vm.Result {
		t.Helper()
		cd := append([]uint64{Selector(name)}, args...)
		res, err := vm.Execute(code, words(cd...), storage, 10_000_000, vm.Context{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res
	}
	call("init")
	if s := call("supply").Return; s != 1000 {
		t.Fatalf("supply = %d, want 1000", s)
	}
	call("transfer", 5, 100) // caller (id 0) -> 5
	if b := call("balanceOf", 5).Return; b != 100 {
		t.Fatalf("balanceOf(5) = %d, want 100", b)
	}
	if b := call("balanceOf", 0).Return; b != 900 {
		t.Fatalf("balanceOf(0) = %d, want 900", b)
	}
}

func TestRequireReverts(t *testing.T) {
	code, err := Compile(solToken)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	storage := map[uint64]uint64{}
	// init, then a transfer beyond balance must REVERT and leave storage untouched.
	if _, err := vm.Execute(code, words(Selector("init")), storage, 1e7, vm.Context{}); err != nil {
		t.Fatalf("init: %v", err)
	}
	before := storage[0] // not meaningful directly, but snapshot the map size effect
	_, err = vm.Execute(code, words(Selector("transfer"), 5, 999999), storage, 1e7, vm.Context{})
	if err != vm.ErrRevert {
		t.Fatalf("over-transfer: got %v, want ErrRevert", err)
	}
	// Balance of caller (id 0) is still the full 1000 — the revert undid nothing.
	res, err := vm.Execute(code, words(Selector("balanceOf"), 0), storage, 1e7, vm.Context{})
	if err != nil || res.Return != 1000 {
		t.Fatalf("post-revert balance = %d (err %v), want 1000", res.Return, err)
	}
	_ = before
}

func TestUnknownSelectorReverts(t *testing.T) {
	code, err := Compile(solToken)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = vm.Execute(code, words(0xDEADBEEF), map[uint64]uint64{}, 1e7, vm.Context{})
	if err != vm.ErrRevert {
		t.Fatalf("unknown selector: got %v, want ErrRevert", err)
	}
}

func TestCheckedArithmeticReverts(t *testing.T) {
	over, err := Compile(`return 18446744073709551615 + 1;`)
	if err != nil {
		t.Fatalf("compile over: %v", err)
	}
	if _, err := vm.Execute(over, nil, map[uint64]uint64{}, 1e7, vm.Context{}); err != vm.ErrOverflow {
		t.Fatalf("overflow: got %v, want ErrOverflow", err)
	}
	under, err := Compile(`return 0 - 1;`)
	if err != nil {
		t.Fatalf("compile under: %v", err)
	}
	if _, err := vm.Execute(under, nil, map[uint64]uint64{}, 1e7, vm.Context{}); err != vm.ErrOverflow {
		t.Fatalf("underflow: got %v, want ErrOverflow", err)
	}
}

func TestSelectorStable(t *testing.T) {
	// Selectors are deterministic and distinct per name.
	if Selector("transfer") == Selector("balanceOf") {
		t.Fatal("distinct names collided")
	}
	if Selector("transfer") != Selector("transfer") {
		t.Fatal("selector not stable")
	}
}

func TestEstimateNonZero(t *testing.T) {
	code, gas, err := CompileAndEstimate(`store[0] = 1; emit(7, 8);`)
	if err != nil || len(code) == 0 || gas == 0 {
		t.Fatalf("estimate: code=%d gas=%d err=%v", len(code), gas, err)
	}
}

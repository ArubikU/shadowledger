package shl

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/ArubikU/shadowledger/internal/vm"
)

// localBase is where compiler-allocated locals (params + `let`) live: very high
// storage slots so they don't collide with user data. v1 locals are storage-backed.
const localBase uint64 = 0xFFFFFFFF00000000

// stateBase is where named `state` variables live. Each declared name gets one
// fixed slot here (Solidity-style slot 0,1,2…), well clear of flat store[…] keys
// and of address digests. A scalar reads/writes its slot directly; a mapping
// keys entries via MIX(slot, key) (the VM's keccak-equivalent), so two named
// mappings never collide and neither pollutes the other's space.
const stateBase uint64 = 0xFFFFFFF000000000

// Selector is the 8-byte function selector for a name: be8(sha256(name)). A call
// puts this in calldata word 0; the dispatcher matches it to a function. This is
// the ShadowLedger analogue of Solidity's 4-byte keccak selector.
func Selector(name string) uint64 {
	sum := sha256.Sum256([]byte(name))
	return binary.BigEndian.Uint64(sum[:8])
}

type gen struct {
	code      []byte
	vars      map[string]uint64 // locals: params + `let` -> localBase slot
	next      uint64
	state     map[string]uint64 // named state vars -> stateBase slot
	nextState uint64
	labels    []int        // label id -> byte position (-1 until marked)
	fixups    []fixupEntry // PUSH placeholders to patch with a label position
}
type fixupEntry struct {
	pos   int // offset of the 8-byte operand
	label int
}

// Compile parses .shl source and emits ShadowLedger VM bytecode.
//
// Two shapes are supported, both from the same grammar:
//   - Flat script (no `fn`): statements run top-to-bottom (the original SHL).
//   - Solidity-like: `state` vars + `fn name(params){…}`. The compiler emits a
//     selector dispatcher (calldata word 0 = selector) that routes to the
//     matching function, binds its params from calldata, and reverts on no match.
func Compile(src string) ([]byte, error) {
	prog, err := Parse(src)
	if err != nil {
		return nil, err
	}
	g := &gen{
		vars:      map[string]uint64{},
		next:      localBase,
		state:     map[string]uint64{},
		nextState: stateBase,
	}

	// Pass 1: register all named state vars so forward references resolve, and
	// split functions from any flat top-level statements.
	var fns []*FnDecl
	var flat []Node
	for _, n := range prog {
		switch d := n.(type) {
		case *StateDecl:
			g.declState(d.Name)
		case *FnDecl:
			fns = append(fns, d)
		default:
			flat = append(flat, n)
		}
	}

	// Flat top-level statements (if any) run first, as a prelude.
	for _, s := range flat {
		if err := g.stmt(s); err != nil {
			return nil, err
		}
	}

	if len(fns) > 0 {
		if err := g.dispatch(fns); err != nil {
			return nil, err
		}
	}

	g.emit(vm.STOP)
	g.resolve()
	return g.code, nil
}

// dispatch emits the selector router + each function body.
func (g *gen) dispatch(fns []*FnDecl) error {
	// Load calldata word 0 (the selector) once.
	g.pushU64(0)
	g.emit(vm.CDLOAD)

	// For each fn: if selector == fn-selector, jump to its label.
	fnLabels := make([]int, len(fns))
	for i, f := range fns {
		fnLabels[i] = g.newLabel()
		g.emit(vm.DUP)              // keep selector for the next comparison
		g.pushU64(Selector(f.Name)) // expected selector
		g.emit(vm.EQ)
		g.pushLabel(fnLabels[i])
		g.emit(vm.JUMPI) // pop dest, pop (sel==fnsel) -> jump if match
	}
	// No match: drop the selector and revert (unknown function).
	g.emit(vm.POP)
	g.emit(vm.REVERT)

	// Emit each function body.
	for i, f := range fns {
		g.mark(fnLabels[i])
		g.emit(vm.POP) // drop the leftover selector copy
		// Fresh local scope per function; bind params from calldata word(i+1).
		g.vars = map[string]uint64{}
		g.next = localBase
		for pi, p := range f.Params {
			slot := g.slot(p)
			g.pushU64(slot)           // key
			g.pushU64(uint64(pi + 1)) // calldata word index (word 0 = selector)
			g.emit(vm.CDLOAD)         // value
			g.emit(vm.SSTORE)
		}
		if err := g.stmts(f.Body); err != nil {
			return err
		}
		g.emit(vm.STOP) // implicit return-nothing at end of function
	}
	return nil
}

func (g *gen) emit(b byte) { g.code = append(g.code, b) }
func (g *gen) pushU64(v uint64) {
	g.emit(vm.PUSH)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	g.code = append(g.code, b[:]...)
}
func (g *gen) newLabel() int { g.labels = append(g.labels, -1); return len(g.labels) - 1 }
func (g *gen) mark(id int)   { g.labels[id] = len(g.code) }
func (g *gen) pushLabel(id int) {
	g.emit(vm.PUSH)
	g.fixups = append(g.fixups, fixupEntry{pos: len(g.code), label: id})
	g.code = append(g.code, make([]byte, 8)...)
}
func (g *gen) resolve() {
	for _, f := range g.fixups {
		binary.BigEndian.PutUint64(g.code[f.pos:f.pos+8], uint64(g.labels[f.label]))
	}
}

// slot returns the local storage slot for a name, allocating one on first use.
func (g *gen) slot(name string) uint64 {
	if s, ok := g.vars[name]; ok {
		return s
	}
	s := g.next
	g.next++
	g.vars[name] = s
	return s
}

// declState assigns a fixed storage slot to a named state variable/mapping.
func (g *gen) declState(name string) uint64 {
	if s, ok := g.state[name]; ok {
		return s
	}
	s := g.nextState
	g.nextState++
	g.state[name] = s
	return s
}

func (g *gen) stmt(n Node) error {
	switch s := n.(type) {
	case *LetStmt:
		// Assignment. If the name is a declared state scalar, write its slot;
		// otherwise it's a local (`let` or a local reassignment).
		if slot, ok := g.state[s.Name]; ok {
			g.pushU64(slot)
			if err := g.expr(s.Val); err != nil {
				return err
			}
			g.emit(vm.SSTORE)
			return nil
		}
		slot := g.slot(s.Name)
		g.pushU64(slot) // key (under value)
		if err := g.expr(s.Val); err != nil {
			return err
		}
		g.emit(vm.SSTORE)
	case *IndexStore: // name[key] = val  (mapping write)
		slot, ok := g.state[s.Name]
		if !ok {
			return fmt.Errorf("shl: %q[...] is not a declared state mapping", s.Name)
		}
		g.pushU64(slot)
		if err := g.expr(s.Key); err != nil {
			return err
		}
		g.emit(vm.MIX) // key = MIX(slot, index)
		if err := g.expr(s.Val); err != nil {
			return err
		}
		g.emit(vm.SSTORE)
	case *StoreStmt:
		if err := g.expr(s.Key); err != nil {
			return err
		}
		if err := g.expr(s.Val); err != nil {
			return err
		}
		g.emit(vm.SSTORE)
	case *RequireStmt:
		cont := g.newLabel()
		if err := g.expr(s.Cond); err != nil {
			return err
		}
		g.pushLabel(cont)
		g.emit(vm.JUMPI) // if cond != 0 skip the revert
		g.emit(vm.REVERT)
		g.mark(cont)
	case *RevertStmt:
		g.emit(vm.REVERT)
	case *IfStmt:
		elseL, endL := g.newLabel(), g.newLabel()
		if err := g.expr(s.Cond); err != nil {
			return err
		}
		g.emit(vm.ISZERO)
		g.pushLabel(elseL)
		g.emit(vm.JUMPI) // if !cond goto else
		if err := g.stmts(s.Then); err != nil {
			return err
		}
		g.pushLabel(endL)
		g.emit(vm.JUMP)
		g.mark(elseL)
		if err := g.stmts(s.Else); err != nil {
			return err
		}
		g.mark(endL)
	case *WhileStmt:
		startL, endL := g.newLabel(), g.newLabel()
		g.mark(startL)
		if err := g.expr(s.Cond); err != nil {
			return err
		}
		g.emit(vm.ISZERO)
		g.pushLabel(endL)
		g.emit(vm.JUMPI)
		if err := g.stmts(s.Body); err != nil {
			return err
		}
		g.pushLabel(startL)
		g.emit(vm.JUMP)
		g.mark(endL)
	case *EmitStmt:
		for _, a := range s.Args {
			if err := g.expr(a); err != nil {
				return err
			}
		}
		g.pushU64(uint64(len(s.Args)))
		g.emit(vm.LOG)
	case *ReturnStmt:
		if err := g.expr(s.Val); err != nil {
			return err
		}
		g.emit(vm.RETURN)
	case *StopStmt:
		g.emit(vm.STOP)
	default:
		return fmt.Errorf("shl: cannot compile statement %T", n)
	}
	return nil
}

func (g *gen) stmts(ns []Node) error {
	for _, n := range ns {
		if err := g.stmt(n); err != nil {
			return err
		}
	}
	return nil
}

func (g *gen) expr(n Node) error {
	switch e := n.(type) {
	case *Num:
		g.pushU64(e.V)
	case *Var:
		// local/param first, then named state scalar.
		if slot, ok := g.vars[e.Name]; ok {
			g.pushU64(slot)
			g.emit(vm.SLOAD)
			return nil
		}
		if slot, ok := g.state[e.Name]; ok {
			g.pushU64(slot)
			g.emit(vm.SLOAD)
			return nil
		}
		return fmt.Errorf("shl: undefined variable %q", e.Name)
	case *Index: // name[key] mapping read
		slot, ok := g.state[e.Name]
		if !ok {
			return fmt.Errorf("shl: %q[...] is not a declared state mapping", e.Name)
		}
		g.pushU64(slot)
		if err := g.expr(e.Key); err != nil {
			return err
		}
		g.emit(vm.MIX)
		g.emit(vm.SLOAD)
	case *Load:
		if err := g.expr(e.Key); err != nil {
			return err
		}
		g.emit(vm.SLOAD)
	case *Builtin:
		switch e.Name {
		case "caller":
			g.emit(vm.CALLER)
		case "value":
			g.emit(vm.VALUE)
		case "balance":
			g.emit(vm.BAL)
		case "self":
			g.emit(vm.SELF)
		case "arg":
			if err := g.expr(e.Arg); err != nil {
				return err
			}
			g.emit(vm.CDLOAD)
		default:
			return fmt.Errorf("shl: unknown builtin %q", e.Name)
		}
	case *Unary: // only "!"
		if err := g.expr(e.X); err != nil {
			return err
		}
		g.emit(vm.ISZERO)
	case *Binary:
		if err := g.expr(e.L); err != nil {
			return err
		}
		if err := g.expr(e.R); err != nil {
			return err
		}
		return g.binop(e.Op)
	default:
		return fmt.Errorf("shl: cannot compile expression %T", n)
	}
	return nil
}

func (g *gen) binop(op string) error {
	switch op {
	// Arithmetic is overflow-CHECKED by default (Solidity 0.8 semantics): an
	// overflow/underflow reverts rather than wrapping. DIV/MOD already revert on
	// a zero divisor in the VM.
	case "+":
		g.emit(vm.ADDC)
	case "-":
		g.emit(vm.SUBC)
	case "*":
		g.emit(vm.MULC)
	case "/":
		g.emit(vm.DIV)
	case "%":
		g.emit(vm.MOD)
	case "==":
		g.emit(vm.EQ)
	case "<":
		g.emit(vm.LT)
	case ">":
		g.emit(vm.GT)
	case "&&":
		g.emit(vm.AND)
	case "||":
		g.emit(vm.OR)
	case "!=":
		g.emit(vm.EQ)
		g.emit(vm.ISZERO)
	case "<=": // !(a > b)
		g.emit(vm.GT)
		g.emit(vm.ISZERO)
	case ">=": // !(a < b)
		g.emit(vm.LT)
		g.emit(vm.ISZERO)
	default:
		return fmt.Errorf("shl: unknown operator %q", op)
	}
	return nil
}

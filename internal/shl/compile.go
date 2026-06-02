package shl

import (
	"encoding/binary"
	"fmt"

	"github.com/ArubikU/shadowledger/internal/vm"
)

// localBase is where compiler-allocated variables live: high storage slots, so
// they don't collide with user store[...] keys. v1 locals are storage-backed.
const localBase uint64 = 0xFFFFFFFF00000000

type gen struct {
	code   []byte
	vars   map[string]uint64
	next   uint64
	labels []int        // label id -> byte position (-1 until marked)
	fixups []fixupEntry // PUSH placeholders to patch with a label position
}
type fixupEntry struct {
	pos   int // offset of the 8-byte operand
	label int
}

// Compile parses .shl source and emits ShadowLedger VM bytecode.
func Compile(src string) ([]byte, error) {
	prog, err := Parse(src)
	if err != nil {
		return nil, err
	}
	g := &gen{vars: map[string]uint64{}, next: localBase}
	for _, s := range prog {
		if err := g.stmt(s); err != nil {
			return nil, err
		}
	}
	g.emit(vm.STOP)
	g.resolve()
	return g.code, nil
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

func (g *gen) slot(name string) uint64 {
	if s, ok := g.vars[name]; ok {
		return s
	}
	s := g.next
	g.next++
	g.vars[name] = s
	return s
}

func (g *gen) stmt(n Node) error {
	switch s := n.(type) {
	case *LetStmt:
		slot := g.slot(s.Name)
		g.pushU64(slot) // key (under value)
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
		s, ok := g.vars[e.Name]
		if !ok {
			return fmt.Errorf("shl: undefined variable %q", e.Name)
		}
		g.pushU64(s)
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
	case "+":
		g.emit(vm.ADD)
	case "-":
		g.emit(vm.SUB)
	case "*":
		g.emit(vm.MUL)
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

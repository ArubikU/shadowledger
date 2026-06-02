// Package vm is ShadowLedger's minimal deterministic smart-contract VM.
//
// It is a stack machine over uint64 words with per-contract uint64 key/value
// storage and gas metering. Determinism is total: no time, no randomness, no
// floating point — so every node executing the same contract on the same input
// and storage reaches the same result, which is mandatory for consensus.
//
// This is intentionally small (v0.4): enough for counters, balances, access
// control and simple logic, with a clear path to a richer VM (or WASM) later.
package vm

import "errors"

// Opcodes.
const (
	STOP   = 0x00
	PUSH   = 0x01 // PUSH <8 bytes BE> : push immediate
	POP    = 0x02
	ADD    = 0x10
	SUB    = 0x11
	MUL    = 0x12
	DIV    = 0x13 // div by zero -> revert
	MOD    = 0x14
	EQ     = 0x20
	LT     = 0x21
	GT     = 0x22
	ISZERO = 0x23
	AND    = 0x24
	OR     = 0x25
	NOT    = 0x26
	DUP    = 0x30 // duplicate top
	SWAP   = 0x31 // swap top two
	JUMP   = 0x40 // pop dest -> pc=dest
	JUMPI  = 0x41 // pop dest, pop cond -> if cond!=0 pc=dest
	SLOAD  = 0x50 // pop key -> push storage[key]
	SSTORE = 0x51 // pop value, pop key -> storage[key]=value
	CALLER = 0x60 // push caller-id (uint64 derived from address)
	VALUE  = 0x61 // push value sent with the call
	BAL    = 0x62 // push contract balance
	CDLOAD = 0x63 // pop word-index -> push input[index] (uint64 word, 0 if oob)
	RETURN = 0x70 // pop -> set return value, halt
)

// Errors. ErrRevert and out-of-gas are "soft": the caller reverts effects but
// still charges the fee. Structural errors are also treated as reverts.
var (
	ErrOutOfGas   = errors.New("vm: out of gas")
	ErrStackUnder = errors.New("vm: stack underflow")
	ErrStackOver  = errors.New("vm: stack overflow")
	ErrBadJump    = errors.New("vm: invalid jump destination")
	ErrDivZero    = errors.New("vm: division by zero")
	ErrBadCode    = errors.New("vm: malformed bytecode")
	ErrStepLimit  = errors.New("vm: step limit exceeded")
)

// Context is the execution environment exposed to a contract.
type Context struct {
	Caller  uint64 // caller identity (uint64 digest of address)
	Value   uint64 // value sent with this call
	Balance uint64 // contract balance
}

const (
	maxStack = 1024
	maxSteps = 1_000_000
)

// gas cost per opcode (storage writes are the expensive ones).
func gasCost(op byte) uint64 {
	switch op {
	case SSTORE:
		return 100
	case SLOAD:
		return 20
	case PUSH:
		return 3
	default:
		return 1
	}
}

// Result reports the outcome of execution.
type Result struct {
	GasUsed uint64
	Return  uint64
	Stack   []uint64
}

// words decodes input bytes into uint64 words (8 bytes BE each).
func word(input []byte, idx uint64) uint64 {
	off := idx * 8
	if off+8 > uint64(len(input)) {
		return 0
	}
	var v uint64
	for i := uint64(0); i < 8; i++ {
		v = v<<8 | uint64(input[off+i])
	}
	return v
}

// Execute runs code against storage (mutated in place ONLY on success) with the
// given input, gas limit and context. On any error the storage map is left
// untouched (the caller copies before calling, so reverts are clean).
func Execute(code, input []byte, storage map[uint64]uint64, gas uint64, ctx Context) (*Result, error) {
	// Run against a working copy so a mid-execution error reverts storage.
	work := make(map[uint64]uint64, len(storage))
	for k, v := range storage {
		work[k] = v
	}

	var st []uint64
	push := func(v uint64) error {
		if len(st) >= maxStack {
			return ErrStackOver
		}
		st = append(st, v)
		return nil
	}
	pop := func() (uint64, error) {
		if len(st) == 0 {
			return 0, ErrStackUnder
		}
		v := st[len(st)-1]
		st = st[:len(st)-1]
		return v, nil
	}

	var used uint64
	var ret uint64
	steps := 0
	pc := 0
	for pc < len(code) {
		if steps++; steps > maxSteps {
			return nil, ErrStepLimit
		}
		op := code[pc]
		c := gasCost(op)
		if used+c > gas {
			return nil, ErrOutOfGas
		}
		used += c
		pc++

		switch op {
		case STOP:
			return commit(storage, work, used, ret, st), nil
		case PUSH:
			if pc+8 > len(code) {
				return nil, ErrBadCode
			}
			var v uint64
			for i := 0; i < 8; i++ {
				v = v<<8 | uint64(code[pc+i])
			}
			pc += 8
			if err := push(v); err != nil {
				return nil, err
			}
		case POP:
			if _, err := pop(); err != nil {
				return nil, err
			}
		case ADD, SUB, MUL, DIV, MOD, EQ, LT, GT, AND, OR:
			b, err := pop()
			if err != nil {
				return nil, err
			}
			a, err := pop()
			if err != nil {
				return nil, err
			}
			r, err := binop(op, a, b)
			if err != nil {
				return nil, err
			}
			if err := push(r); err != nil {
				return nil, err
			}
		case ISZERO, NOT:
			a, err := pop()
			if err != nil {
				return nil, err
			}
			if op == ISZERO {
				st = append(st, b2u(a == 0))
			} else {
				st = append(st, ^a)
			}
		case DUP:
			a, err := pop()
			if err != nil {
				return nil, err
			}
			push(a)
			if err := push(a); err != nil {
				return nil, err
			}
		case SWAP:
			b, err := pop()
			if err != nil {
				return nil, err
			}
			a, err := pop()
			if err != nil {
				return nil, err
			}
			push(b)
			push(a)
		case JUMP:
			dest, err := pop()
			if err != nil {
				return nil, err
			}
			if int(dest) >= len(code) {
				return nil, ErrBadJump
			}
			pc = int(dest)
		case JUMPI:
			dest, err := pop()
			if err != nil {
				return nil, err
			}
			cond, err := pop()
			if err != nil {
				return nil, err
			}
			if cond != 0 {
				if int(dest) >= len(code) {
					return nil, ErrBadJump
				}
				pc = int(dest)
			}
		case SLOAD:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			if err := push(work[k]); err != nil {
				return nil, err
			}
		case SSTORE:
			v, err := pop()
			if err != nil {
				return nil, err
			}
			k, err := pop()
			if err != nil {
				return nil, err
			}
			work[k] = v
		case CALLER:
			if err := push(ctx.Caller); err != nil {
				return nil, err
			}
		case VALUE:
			if err := push(ctx.Value); err != nil {
				return nil, err
			}
		case BAL:
			if err := push(ctx.Balance); err != nil {
				return nil, err
			}
		case CDLOAD:
			idx, err := pop()
			if err != nil {
				return nil, err
			}
			if err := push(word(input, idx)); err != nil {
				return nil, err
			}
		case RETURN:
			r, err := pop()
			if err != nil {
				return nil, err
			}
			ret = r
			return commit(storage, work, used, ret, st), nil
		default:
			return nil, ErrBadCode
		}
	}
	return commit(storage, work, used, ret, st), nil
}

func commit(dst, work map[uint64]uint64, used, ret uint64, st []uint64) *Result {
	for k := range dst {
		delete(dst, k)
	}
	for k, v := range work {
		dst[k] = v
	}
	return &Result{GasUsed: used, Return: ret, Stack: st}
}

func binop(op byte, a, b uint64) (uint64, error) {
	switch op {
	case ADD:
		return a + b, nil
	case SUB:
		return a - b, nil
	case MUL:
		return a * b, nil
	case DIV:
		if b == 0 {
			return 0, ErrDivZero
		}
		return a / b, nil
	case MOD:
		if b == 0 {
			return 0, ErrDivZero
		}
		return a % b, nil
	case EQ:
		return b2u(a == b), nil
	case LT:
		return b2u(a < b), nil
	case GT:
		return b2u(a > b), nil
	case AND:
		return a & b, nil
	case OR:
		return a | b, nil
	}
	return 0, ErrBadCode
}

func b2u(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

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

import (
	"crypto/sha256"
	"errors"
)

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
	ADDC   = 0x15 // checked add  : revert on overflow  (Solidity 0.8 default)
	SUBC   = 0x16 // checked sub  : revert on underflow
	MULC   = 0x17 // checked mul  : revert on overflow
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
	MIX    = 0x57 // pop key, pop base -> push be8(sha256(base||key)); mapping slot (keccak-equiv)
	CALLER = 0x60 // push caller-id (uint64 derived from address)
	VALUE  = 0x61 // push value sent with the call
	BAL    = 0x62 // push contract balance
	CDLOAD = 0x63 // pop word-index -> push input[index] (uint64 word, 0 if oob)
	SELF   = 0x64 // push this contract's id (uint64 digest of its address)
	CALL   = 0x71 // pop gasLimit, pop arg, pop target -> call target contract; push its return (0 on fail)
	RETURN = 0x70 // pop -> set return value, halt
	REVERT = 0x74 // abort: discard all storage writes, charge gas (require/revert)
	LOG    = 0x72 // pop n, pop n topic words -> emit an event (history); n<=maxLogTopics

	// Byte-value layer: full addresses & strings live in byte-storage (bkey -> []byte),
	// sourced from calldata. The uint64 stack holds byte-storage keys / lengths.
	CDLEN   = 0x66 // push calldata length in bytes
	BSTORE  = 0x53 // pop bkey, pop off, pop len -> bstore[bkey] = calldata[off:off+len]
	BEQ     = 0x54 // pop bkeyA, pop bkeyB -> push 1 if the byte blobs are equal else 0
	BLEN    = 0x55 // pop bkey -> push len(bstore[bkey])
	BHASH   = 0x56 // pop bkey -> push uint64 digest of bstore[bkey] (for keying)
	CALLERB = 0x67 // pop bkey -> bstore[bkey] = caller's full address bytes
	SELFB   = 0x68 // pop bkey -> bstore[bkey] = this contract's full address bytes
	RETURNB = 0x73 // pop bkey -> return bstore[bkey] as the call's byte return, halt
)

const maxLogTopics = 8

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
	ErrRevert     = errors.New("vm: reverted")
	ErrOverflow   = errors.New("vm: arithmetic overflow")
)

// CallHost lets a contract invoke another contract. The host resolves the
// target id to a contract, executes it, and reports its return value and gas
// used. ok is false if the target is missing or it reverted.
type CallHost interface {
	Call(self, target, arg, gasLimit uint64, depth int) (ret uint64, gasUsed uint64, ok bool)
}

// Context is the execution environment exposed to a contract.
type Context struct {
	Caller     uint64   // caller identity (uint64 digest of address)
	Self       uint64   // this contract's identity
	CallerAddr []byte   // caller's FULL address bytes (for the byte layer)
	SelfAddr   []byte   // this contract's FULL address bytes
	Value      uint64   // value sent with this call
	Balance    uint64   // contract balance
	Host       CallHost // cross-contract call host (nil disables CALL)
	Depth      int      // current call depth (reentrancy / recursion bound)
}

const (
	maxStack = 1024
	maxSteps = 1_000_000
	maxDepth = 8 // contract-to-contract call depth limit
)

// gas cost per opcode (storage writes are the expensive ones).
func gasCost(op byte) uint64 {
	switch op {
	case SSTORE, BSTORE, CALLERB, SELFB:
		return 100
	case LOG:
		return 50 // + 8 per topic, charged inline
	case SLOAD, BEQ, BHASH, MIX:
		return 20
	case PUSH:
		return 3
	default:
		return 1
	}
}

// Result reports the outcome of execution.
type Result struct {
	GasUsed     uint64
	Return      uint64
	ReturnBytes []byte // set by RETURNB (e.g. an address or a token URI)
	Stack       []uint64
	Logs        [][]uint64 // events emitted via LOG (each is a list of topic words)
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
	return ExecuteB(code, input, storage, nil, gas, ctx)
}

// ExecuteB is Execute with a byte-storage map (bstore: key -> []byte) for the
// address/string layer. bstore may be nil (byte ops then have nothing to read,
// stores are dropped). It is mutated in place ONLY on success.
func ExecuteB(code, input []byte, storage map[uint64]uint64, bstore map[uint64][]byte, gas uint64, ctx Context) (*Result, error) {
	// Run against a working copy so a mid-execution error reverts storage.
	work := make(map[uint64]uint64, len(storage))
	for k, v := range storage {
		work[k] = v
	}
	bwork := make(map[uint64][]byte, len(bstore))
	for k, v := range bstore {
		bwork[k] = append([]byte(nil), v...)
	}
	var retBytes []byte

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
	var logs [][]uint64
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
			return commitB(storage, work, bstore, bwork, used, ret, retBytes, st, logs), nil
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
		case ADDC, SUBC, MULC:
			b, err := pop()
			if err != nil {
				return nil, err
			}
			a, err := pop()
			if err != nil {
				return nil, err
			}
			var r uint64
			switch op {
			case ADDC:
				r = a + b
				if r < a {
					return nil, ErrOverflow
				}
			case SUBC:
				if b > a {
					return nil, ErrOverflow
				}
				r = a - b
			case MULC:
				r = a * b
				if a != 0 && r/a != b {
					return nil, ErrOverflow
				}
			}
			if err := push(r); err != nil {
				return nil, err
			}
		case MIX:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			base, err := pop()
			if err != nil {
				return nil, err
			}
			var buf [16]byte
			for i := 0; i < 8; i++ {
				buf[i] = byte(base >> (56 - 8*i))
				buf[8+i] = byte(k >> (56 - 8*i))
			}
			sum := sha256.Sum256(buf[:])
			if err := push(beU64(sum[:8])); err != nil {
				return nil, err
			}
		case REVERT:
			return nil, ErrRevert
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
		case SELF:
			if err := push(ctx.Self); err != nil {
				return nil, err
			}
		case CALL:
			gl, err := pop()
			if err != nil {
				return nil, err
			}
			arg, err := pop()
			if err != nil {
				return nil, err
			}
			target, err := pop()
			if err != nil {
				return nil, err
			}
			// Forward at most the remaining gas; a failed/absent call pushes 0.
			if ctx.Host == nil || ctx.Depth >= maxDepth {
				if err := push(0); err != nil {
					return nil, err
				}
				break
			}
			forward := gl
			if rem := gas - used; forward > rem {
				forward = rem
			}
			ret2, gu, ok := ctx.Host.Call(ctx.Self, target, arg, forward, ctx.Depth+1)
			used += gu
			if used > gas {
				return nil, ErrOutOfGas
			}
			if ok {
				if err := push(ret2); err != nil {
					return nil, err
				}
			} else if err := push(0); err != nil {
				return nil, err
			}
		case LOG:
			n, err := pop()
			if err != nil {
				return nil, err
			}
			if n > maxLogTopics {
				return nil, ErrBadCode
			}
			if used+8*n > gas {
				return nil, ErrOutOfGas
			}
			used += 8 * n
			topics := make([]uint64, n)
			for i := int(n) - 1; i >= 0; i-- { // popped in reverse → restore order
				v, err := pop()
				if err != nil {
					return nil, err
				}
				topics[i] = v
			}
			logs = append(logs, topics)
		case CDLEN:
			if err := push(uint64(len(input))); err != nil {
				return nil, err
			}
		case BSTORE:
			bkey, e1 := pop()
			off, e2 := pop()
			ln, e3 := pop()
			if e1 != nil || e2 != nil || e3 != nil {
				return nil, ErrStackUnder
			}
			if off+ln > uint64(len(input)) || off+ln < off {
				return nil, ErrBadCode // out-of-range calldata slice
			}
			bwork[bkey] = append([]byte(nil), input[off:off+ln]...)
		case BEQ:
			a, e1 := pop()
			b, e2 := pop()
			if e1 != nil || e2 != nil {
				return nil, ErrStackUnder
			}
			if err := push(b2u(bytesEqual(bwork[a], bwork[b]))); err != nil {
				return nil, err
			}
		case BLEN:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			if err := push(uint64(len(bwork[k]))); err != nil {
				return nil, err
			}
		case BHASH:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(bwork[k])
			if err := push(beU64(sum[:8])); err != nil {
				return nil, err
			}
		case CALLERB:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			bwork[k] = append([]byte(nil), ctx.CallerAddr...)
		case SELFB:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			bwork[k] = append([]byte(nil), ctx.SelfAddr...)
		case RETURNB:
			k, err := pop()
			if err != nil {
				return nil, err
			}
			retBytes = append([]byte(nil), bwork[k]...)
			return commitB(storage, work, bstore, bwork, used, ret, retBytes, st, logs), nil
		case RETURN:
			r, err := pop()
			if err != nil {
				return nil, err
			}
			ret = r
			return commitB(storage, work, bstore, bwork, used, ret, retBytes, st, logs), nil
		default:
			return nil, ErrBadCode
		}
	}
	return commitB(storage, work, bstore, bwork, used, ret, retBytes, st, logs), nil
}

// commitB flushes the uint64 working storage AND the byte working storage back
// to the live maps (only called on successful halt), and builds the Result.
func commitB(dst, work map[uint64]uint64, bdst, bwork map[uint64][]byte, used, ret uint64, retBytes []byte, st []uint64, logs [][]uint64) *Result {
	for k := range dst {
		delete(dst, k)
	}
	for k, v := range work {
		dst[k] = v
	}
	if bdst != nil {
		for k := range bdst {
			delete(bdst, k)
		}
		for k, v := range bwork {
			bdst[k] = v
		}
	}
	return &Result{GasUsed: used, Return: ret, ReturnBytes: retBytes, Stack: st, Logs: logs}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func beU64(b []byte) uint64 {
	var v uint64
	for i := 0; i < len(b) && i < 8; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v
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

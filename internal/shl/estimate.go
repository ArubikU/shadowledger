package shl

import (
	"encoding/binary"

	"github.com/ArubikU/shadowledger/internal/vm"
)

// Estimate approximates the gas a compiled program consumes in ONE straight-line
// pass: it sums each opcode's cost once. It is a lower-bound guide, NOT exact —
// loops cost per-iteration (counted once here) and CALL's callee gas is not
// included. Mirrors the VM's gas schedule.
func Estimate(code []byte) uint64 {
	var total, lastConst uint64
	hasConst := false
	for i := 0; i < len(code); {
		op := code[i]
		switch op {
		case vm.PUSH:
			total += 3
			if i+9 <= len(code) {
				lastConst = binary.BigEndian.Uint64(code[i+1 : i+9])
				hasConst = true
			}
			i += 9
			continue
		case vm.SSTORE:
			total += 100
		case vm.SLOAD:
			total += 20
		case vm.LOG:
			total += 50
			if hasConst { // topic count was the last pushed constant
				total += 8 * lastConst
			}
		default:
			total++
		}
		if op != vm.PUSH {
			hasConst = false
		}
		i++
	}
	return total
}

// CompileAndEstimate compiles source and returns the bytecode + approx gas.
func CompileAndEstimate(src string) ([]byte, uint64, error) {
	code, err := Compile(src)
	if err != nil {
		return nil, 0, err
	}
	return code, Estimate(code), nil
}

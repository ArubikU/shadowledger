// Package regpow is ShadowLedger's registration Proof-of-Work: a one-time
// compute puzzle a node must solve to register as a validator (on TOP of its
// bond). It is NOT used to mint blocks — that would waste energy. Its jobs are
// Sybil resistance (mass fake identities cost real compute) and a randomness
// seed for shard assignment, exactly like Zilliqa's DS-epoch PoW. Difficulty is
// a few hundred-thousand hashes (sub-second) on a testnet.
package regpow

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/ArubikU/shadowledger/internal/crypto"
)

const prefix = "sl-regpow:"

// Hash binds the puzzle to the network and the registering address so a solution
// can't be reused on another chain or by another identity.
func Hash(chainID uint64, addr crypto.Address, nonce uint64) [32]byte {
	h := sha256.New()
	h.Write([]byte(prefix))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], chainID)
	h.Write(b[:])
	h.Write([]byte(addr))
	binary.BigEndian.PutUint64(b[:], nonce)
	h.Write(b[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// leadingZeroBits counts the leading zero bits of a hash.
func leadingZeroBits(h [32]byte) int {
	n := 0
	for _, b := range h {
		if b == 0 {
			n += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if b&(1<<uint(i)) == 0 {
				n++
			} else {
				return n
			}
		}
	}
	return n
}

// Verify reports whether nonce solves the puzzle at the given difficulty (bits).
// bits == 0 disables the requirement.
func Verify(chainID uint64, addr crypto.Address, nonce uint64, bits int) bool {
	if bits <= 0 {
		return true
	}
	return leadingZeroBits(Hash(chainID, addr, nonce)) >= bits
}

// Solve finds a nonce satisfying the puzzle (the "work"). Deterministic search
// from 0; returns the first solution.
func Solve(chainID uint64, addr crypto.Address, bits int) uint64 {
	if bits <= 0 {
		return 0
	}
	for nonce := uint64(0); ; nonce++ {
		if leadingZeroBits(Hash(chainID, addr, nonce)) >= bits {
			return nonce
		}
	}
}

// Seed returns the solved puzzle hash — usable as unbiased randomness for shard
// assignment (a validator can't pick its own shards).
func Seed(chainID uint64, addr crypto.Address, nonce uint64) [32]byte {
	return Hash(chainID, addr, nonce)
}

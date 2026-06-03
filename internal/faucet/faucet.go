// Package faucet is ShadowLedger's storage-flavored Proof-of-Work faucet: a way
// for FOLLOWERS (non-validators) to earn a small amount of $SHARD by doing real
// compute, without a bond or a centralized tap. Validators already mint via the
// block coinbase; this is the on-ramp for everyone else.
//
// The puzzle is Bitcoin-style (find a nonce so the hash has N leading zero bits)
// but ANCHORED to a recent block's BodyHash — the commitment to the body that
// gets erasure-coded into that block's shards. You are, in effect, mining against
// the latest shard set's commitment. The anchor is:
//   - committed + signed in the block header, so EVERY node can verify a claim
//     without holding the (fragmented) body — this is what keeps it consensus-safe
//     in a network where no node stores full history;
//   - unknown until the block is produced, so solutions can't be precomputed
//     (ongoing work, like Bitcoin), and it rotates as the chain advances.
//
// Sybil resistance is the hashing cost per address; a per-address cooldown caps
// the rate, and payouts are drawn from the treasury (bounded), not freshly minted
// (so the 21M cap and halving schedule are untouched).
package faucet

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/ArubikU/shadowledger/internal/crypto"
)

const prefix = "sl-faucet:"

// Hash binds the puzzle to the network, the claiming address and a recent block's
// BodyHash (the anchor), so a solution can't be reused on another chain, by
// another identity, or against a different/older block.
func Hash(chainID uint64, addr crypto.Address, anchor [32]byte, nonce uint64) [32]byte {
	h := sha256.New()
	h.Write([]byte(prefix))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], chainID)
	h.Write(b[:])
	h.Write([]byte(addr))
	h.Write(anchor[:])
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
// bits == 0 disables the requirement (faucet effectively open).
func Verify(chainID uint64, addr crypto.Address, anchor [32]byte, nonce uint64, bits int) bool {
	if bits <= 0 {
		return true
	}
	return leadingZeroBits(Hash(chainID, addr, anchor, nonce)) >= bits
}

// Solve finds a nonce satisfying the puzzle (the "work"). Deterministic search
// from 0; returns the first solution. This is the miner a follower runs.
func Solve(chainID uint64, addr crypto.Address, anchor [32]byte, bits int) uint64 {
	if bits <= 0 {
		return 0
	}
	for nonce := uint64(0); ; nonce++ {
		if leadingZeroBits(Hash(chainID, addr, anchor, nonce)) >= bits {
			return nonce
		}
	}
}

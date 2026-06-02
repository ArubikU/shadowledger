// Package merkle implements a binary Merkle tree with inclusion proofs.
//
// Leaf  = sha256(0x00 || data)
// Inner = sha256(0x01 || left || right)
// Odd nodes at a level are duplicated. Empty input -> zero root.
package merkle

import "crypto/sha256"

// Hash is a 32-byte digest.
type Hash [32]byte

// Zero is the all-zero hash (empty-tree root, genesis prev-hash).
var Zero Hash

func leafHash(data []byte) Hash {
	h := sha256.New()
	h.Write([]byte{0x00})
	h.Write(data)
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

func innerHash(l, r Hash) Hash {
	h := sha256.New()
	h.Write([]byte{0x01})
	h.Write(l[:])
	h.Write(r[:])
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// Root computes the Merkle root over the given leaves (already-hashed leaf inputs).
func Root(items [][]byte) Hash {
	if len(items) == 0 {
		return Zero
	}
	level := make([]Hash, len(items))
	for i, it := range items {
		level[i] = leafHash(it)
	}
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		next := make([]Hash, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			next = append(next, innerHash(level[i], level[i+1]))
		}
		level = next
	}
	return level[0]
}

// ProofStep is one sibling on the path to the root.
type ProofStep struct {
	Sibling Hash
	IsLeft  bool // sibling is on the left of the running hash
}

// Proof returns the inclusion proof for leaf index in items.
func Proof(items [][]byte, index int) []ProofStep {
	if index < 0 || index >= len(items) {
		return nil
	}
	level := make([]Hash, len(items))
	for i, it := range items {
		level[i] = leafHash(it)
	}
	var steps []ProofStep
	idx := index
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		if idx%2 == 0 {
			steps = append(steps, ProofStep{Sibling: level[idx+1], IsLeft: false})
		} else {
			steps = append(steps, ProofStep{Sibling: level[idx-1], IsLeft: true})
		}
		next := make([]Hash, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			next = append(next, innerHash(level[i], level[i+1]))
		}
		level = next
		idx /= 2
	}
	return steps
}

// Verify checks that data at the proof path yields root.
func Verify(data []byte, steps []ProofStep, root Hash) bool {
	cur := leafHash(data)
	for _, s := range steps {
		if s.IsLeft {
			cur = innerHash(s.Sibling, cur)
		} else {
			cur = innerHash(cur, s.Sibling)
		}
	}
	return cur == root
}

// Package forkchoice implements ShadowLedger's deterministic chain-selection
// rule: among competing branches, pick the heaviest (most cumulative weight),
// breaking ties by lowest block id. Because every node computes it from the same
// block tree, all nodes converge on the SAME canonical tip without coordination
// — replacing the v0.x "first-valid-block-per-height wins" (which could diverge).
//
// Per-block weight is supplied by the caller; the intent is storage/availability
// weight (a block counts more when its fragments are provably available), so the
// "heaviest" chain is the most-available one — ShadowLedger's thesis applied to
// fork choice. v1 callers may pass weight=1 (longest-chain).
//
// This package is the SELECTION rule. Acting on it (rewinding state to the common
// ancestor and replaying the heavier branch) is the chain-level reorg, built on
// top of this.
package forkchoice

import "github.com/ArubikU/shadowledger/internal/types"

type entry struct {
	parent types.Hash
	height uint64
	cum    uint64 // cumulative weight from genesis to this block
}

// Tree is a block tree (all known valid headers, including side branches).
type Tree struct {
	nodes map[types.Hash]entry
}

// New returns an empty tree.
func New() *Tree { return &Tree{nodes: map[types.Hash]entry{}} }

// Add inserts a block by id with its parent, height and per-block weight. Adding
// the same id twice is a no-op. The genesis block has parent == zero hash.
func (t *Tree) Add(id, parent types.Hash, height, weight uint64) {
	if _, ok := t.nodes[id]; ok {
		return
	}
	cum := weight
	if p, ok := t.nodes[parent]; ok {
		cum += p.cum
	}
	t.nodes[id] = entry{parent: parent, height: height, cum: cum}
}

// Best returns the canonical tip: the block with the greatest cumulative weight,
// ties broken by the lexicographically smallest id (deterministic on every node).
func (t *Tree) Best() (types.Hash, uint64) {
	var best types.Hash
	var bestCum uint64
	first := true
	for id, e := range t.nodes {
		if first || e.cum > bestCum || (e.cum == bestCum && less(id, best)) {
			best, bestCum, first = id, e.cum, false
		}
	}
	return best, bestCum
}

// Height returns the height of a known block (ok=false if unknown).
func (t *Tree) Height(id types.Hash) (uint64, bool) {
	e, ok := t.nodes[id]
	return e.height, ok
}

// CommonAncestor walks both branches back to their first shared block — the
// point a reorg must rewind to before replaying the heavier branch.
func (t *Tree) CommonAncestor(a, b types.Hash) (types.Hash, bool) {
	seen := map[types.Hash]bool{}
	for {
		e, ok := t.nodes[a]
		if !ok {
			break
		}
		seen[a] = true
		if a == e.parent { // reached a root with self-parent (shouldn't happen)
			break
		}
		a = e.parent
		if a == (types.Hash{}) {
			break
		}
	}
	for {
		if seen[b] {
			return b, true
		}
		e, ok := t.nodes[b]
		if !ok || b == (types.Hash{}) {
			break
		}
		b = e.parent
	}
	// fall back: genesis (zero-parent block) if both descend from it
	return types.Hash{}, false
}

func less(a, b types.Hash) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

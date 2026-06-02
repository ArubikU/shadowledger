package chain

import (
	"errors"

	"github.com/ArubikU/shadowledger/internal/types"
)

// Reorg errors.
var (
	ErrUnknownParent = errors.New("chain: block's parent is unknown (can't attach)")
	ErrReorgReplay   = errors.New("chain: heavier branch failed to replay (rejected)")
)

// AcceptBlock is the fork-choice-aware ingestion path (used by postorage gossip):
// unlike ApplyExternalBlock (strictly head+1, no reorg) it accepts blocks on ANY
// known branch, adds them to the block tree, and makes the canonical chain follow
// the heaviest tip — fast-forwarding when that tip extends the head, or REORGING
// (rewind to the replay base + replay the winning branch) when it's a different
// branch. This is how the network converges after a partition / equivocation.
func (c *Chain) AcceptBlock(blk *types.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasGen {
		return ErrNoGenesis
	}
	hdr := &blk.Header
	id := hdr.ID()
	if _, known := c.tree.Height(id); known {
		return nil // already have it
	}
	if err := c.engine.AuthorizeHeader(hdr); err != nil {
		return err
	}
	if types.MerkleRootOf(blk.Txs) != hdr.MerkleRoot {
		return ErrBadMerkle
	}
	if types.BodyHash(blk.Body()) != hdr.BodyHash {
		return ErrBadBodyHash
	}
	parent := c.block(hdr.PrevHash)
	if parent == nil {
		return ErrUnknownParent
	}
	if hdr.Height != parent.Header.Height+1 {
		return ErrBadHeight
	}
	if hdr.Timestamp < parent.Header.Timestamp+int64(hdr.Round)*c.blockTime {
		return ErrBadRound
	}

	c.recordBlock(blk)
	return c.reorgToBest()
}

// walkDown follows parents from tip until it reaches `stop` (exclusive),
// returning the blocks in ascending (stop→tip) order. ok=false if `stop` is not
// an ancestor of tip or a body is missing.
func (c *Chain) walkDown(tip, stop types.Hash) ([]*types.Block, bool) {
	var rev []*types.Block
	for id := tip; id != stop; {
		b := c.block(id)
		if b == nil {
			return nil, false
		}
		rev = append(rev, b)
		id = b.Header.PrevHash
		if id == (types.Hash{}) {
			return nil, false // hit genesis without reaching stop
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, true
}

// reorgToBest makes the head follow the heaviest tip. Two cases:
//   - FAST-FORWARD: the best tip extends the current head → just apply the new
//     blocks on the live state (needs no replay base, so a node WITHOUT one —
//     e.g. the bootstrap node after restart — still follows other validators).
//   - REORG: the best tip is on a divergent branch → rewind to the replay base
//     and replay the winning branch on a fresh clone (committed only if it fully
//     applies). Requires a replay base; without one, the head is kept.
//
// Caller holds c.mu.
func (c *Chain) reorgToBest() error {
	best, _ := c.tree.Best()
	if best == c.head.ID() {
		return nil
	}

	// Fast-forward: best descends from the current head.
	if suffix, ok := c.walkDown(best, c.head.ID()); ok {
		for _, b := range suffix {
			if err := c.state.ApplyBlock(b); err != nil {
				return ErrReorgReplay
			}
			c.commitCanonical(b)
		}
		if n := len(suffix); n > 0 {
			c.head = suffix[n-1].Header
			c.advanceFinality()
		}
		return nil
	}

	// Divergent branch → needs the replay base to rewind to.
	if c.replayBase == nil {
		return nil
	}
	branch, ok := c.walkDown(best, c.replayBaseID)
	if !ok {
		return nil // deeper than our rewind floor; keep current head
	}
	ns := c.replayBase.Clone()
	for _, b := range branch {
		if err := ns.ApplyBlock(b); err != nil {
			return ErrReorgReplay
		}
	}
	c.state.ReplaceWith(ns)
	for _, b := range branch {
		c.commitCanonical(b)
	}
	if n := len(branch); n > 0 {
		c.head = branch[n-1].Header
		c.advanceFinality()
	}
	return nil
}

// commitCanonical persists a now-canonical block's shards + logs.
func (c *Chain) commitCanonical(b *types.Block) {
	body := b.Body()
	_, _ = c.persistBlockSet(&b.Header, body)
	c.persistLogs(&b.Header)
}

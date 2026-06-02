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

// pathToBase walks parents from tip down to (and excluding) the replay base,
// returning the branch in genesis→tip order. ok=false if the tip does not
// descend from the replay base (a reorg deeper than our rewind floor).
func (c *Chain) pathToBase(tip types.Hash) ([]*types.Block, bool) {
	var rev []*types.Block
	for id := tip; id != c.replayBaseID; {
		b := c.block(id)
		if b == nil {
			return nil, false
		}
		rev = append(rev, b)
		id = b.Header.PrevHash
		if id == (types.Hash{}) {
			return nil, false // reached genesis without hitting the base
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, true
}

// reorgToBest makes the head follow the heaviest tip, fast-forwarding if it
// extends the current head, else rewinding to the replay base and replaying the
// winning branch onto a fresh copy (committed only if it fully applies, so a bad
// branch can never corrupt live state). Caller holds c.mu.
func (c *Chain) reorgToBest() error {
	if c.replayBase == nil {
		return nil // reorg base unavailable
	}
	best, _ := c.tree.Best()
	if best == c.head.ID() {
		return nil
	}

	branch, ok := c.pathToBase(best)
	if !ok {
		return nil // can't reorg below the replay floor; keep current head
	}

	// Fast-forward: if the current head is on the winning branch, just apply the
	// blocks above it to the live state (no full rewind).
	if idx := indexOf(branch, c.head.ID()); idx >= 0 {
		for _, b := range branch[idx+1:] {
			if err := c.state.ApplyBlock(b); err != nil {
				return ErrReorgReplay
			}
			c.commitCanonical(b)
		}
		if n := len(branch); n > 0 {
			c.head = branch[n-1].Header
		}
		return nil
	}

	// Divergent branch: rewind to base, replay the whole branch on a clone.
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
	}
	return nil
}

// commitCanonical persists a now-canonical block's shards + logs.
func (c *Chain) commitCanonical(b *types.Block) {
	body := b.Body()
	_, _ = c.persistBlockSet(&b.Header, body)
	c.persistLogs(&b.Header)
}

func indexOf(branch []*types.Block, id types.Hash) int {
	for i, b := range branch {
		if b.Header.ID() == id {
			return i
		}
	}
	return -1
}

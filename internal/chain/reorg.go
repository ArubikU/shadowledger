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

// AcceptBlock is the fork-choice-aware ingestion path: unlike ApplyExternalBlock
// (strictly head+1, no reorg), it accepts blocks on ANY known branch, adds them
// to the block tree, and — if a competing branch becomes heavier — performs a
// REORG: rewinds state to genesis and replays the winning branch. This is what
// lets the network converge after a partition / equivocating producer, the last
// consensus-safety piece. (v1: replays from genesis using in-session bodies.)
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
	// Forgery checks (same crypto/commitment rules as the linear path).
	if err := c.engine.AuthorizeHeader(hdr); err != nil {
		return err
	}
	if types.MerkleRootOf(blk.Txs) != hdr.MerkleRoot {
		return ErrBadMerkle
	}
	if types.BodyHash(blk.Body()) != hdr.BodyHash {
		return ErrBadBodyHash
	}
	// Parent must be known (genesis or a block we hold), and round-timing is
	// validated against the PARENT (not the current head, since this may be a
	// side branch).
	parent, ok := c.blockByID[hdr.PrevHash]
	if !ok {
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

// reorgToBest makes the canonical chain follow the heaviest tip in the tree,
// rewinding+replaying state if that tip is on a different branch than the head.
// On a replay failure the current head is kept (the heavier branch is invalid).
// Caller holds c.mu.
func (c *Chain) reorgToBest() error {
	if c.genesisState == nil {
		return nil // reorg unavailable (e.g. fast-synced node without genesis state)
	}
	best, _ := c.tree.Best()
	if best == c.head.ID() {
		return nil // already canonical
	}

	// Collect the branch genesis→best (walk parents, then reverse).
	var branch []*types.Block
	for id := best; id != c.genesisID; {
		b, ok := c.blockByID[id]
		if !ok {
			return ErrReorgReplay // missing a body on the winning branch
		}
		branch = append(branch, b)
		id = b.Header.PrevHash
	}
	for i, j := 0, len(branch)-1; i < j; i, j = i+1, j-1 {
		branch[i], branch[j] = branch[j], branch[i]
	}

	// Replay onto a fresh copy of genesis state; commit only if the whole branch
	// applies cleanly (so a bad branch can never corrupt the live state).
	ns := c.genesisState.Clone()
	for _, b := range branch {
		if err := ns.ApplyBlock(b); err != nil {
			return ErrReorgReplay
		}
	}

	c.state.ReplaceWith(ns)
	for _, b := range branch {
		body := b.Body()
		if _, err := c.persistBlockSet(&b.Header, body); err != nil {
			return err
		}
		c.persistLogs(&b.Header)
	}
	if len(branch) > 0 {
		c.head = branch[len(branch)-1].Header
	} else {
		c.head, _, _ = c.store.GetHeader(0)
	}
	return nil
}

// Package netparams makes ShadowLedger self-tuning: the erasure parameters
// (K data / M parity shards) and the shard replication factor are derived from
// the LIVE node count, not pinned in any config file. The network decides.
//
// The block producer stamps the chosen ShardSpec into each block header, so the
// rest of the network simply reads it — no agreement protocol needed for the
// erasure shape. Replication is a placement policy each node computes locally
// from its current view of membership (eventually consistent under churn).
package netparams

import "github.com/ArubikU/shadowledger/internal/types"

// Bounds on the total shard count so a tiny net still has parity and a huge net
// does not explode shard counts per block.
const (
	minTotalShards = 3
	maxTotalShards = 24
)

// Spec chooses (K, M) for the current network size n (number of known nodes).
//
// Policy: total shards T scales with n (clamped); ~1/3 are parity so the block
// tolerates losing ~1/3 of the shards (and thus that fraction of holders).
//
//	n=1  -> K=2 M=1   (single node holds all 3; self-reconstruct)
//	n=6  -> K=4 M=2
//	n=24+-> K=16 M=8
func Spec(n int) types.ShardSpec {
	t := n
	if t < minTotalShards {
		t = minTotalShards
	}
	if t > maxTotalShards {
		t = maxTotalShards
	}
	m := t / 3
	if m < 1 {
		m = 1
	}
	k := t - m
	if k < 1 {
		k = 1
	}
	return types.ShardSpec{K: uint8(k), M: uint8(m)}
}

// GenesisSpec is a FIXED erasure shape for block 0. Genesis must be identical
// on every node (so block-1's prev-hash links agree), so it cannot depend on a
// node's live membership view — it uses a constant shape regardless of n.
func GenesisSpec() types.ShardSpec { return types.ShardSpec{K: 2, M: 1} }

// Replication chooses how many holders each shard gets, scaling with the net so
// shards gain redundancy as more nodes join.
//
//	n<3  -> 1   (too few nodes to replicate meaningfully)
//	n<8  -> 2
//	else -> 3
func Replication(n int) int {
	switch {
	case n < 3:
		return 1
	case n < 8:
		return 2
	default:
		return 3
	}
}

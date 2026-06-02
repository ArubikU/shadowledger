// Package rendezvous implements Highest-Random-Weight (HRW) hashing to assign
// each block shard to a deterministic, stable set of holder nodes without any
// global coordination.
package rendezvous

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/ArubikU/shadowledger/internal/types"
)

// score computes the HRW weight of a node for a given (blockID, shardIndex).
func score(nodeID string, blockID types.Hash, shardIndex int) uint64 {
	h := sha256.New()
	h.Write([]byte(nodeID))
	h.Write(blockID[:])
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], uint32(shardIndex))
	h.Write(idx[:])
	sum := h.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

type ranked struct {
	id    string
	score uint64
}

// Holders returns up to r node IDs responsible for (blockID, shardIndex),
// ranked by descending HRW score. Stable for a fixed node set.
func Holders(nodes []string, blockID types.Hash, shardIndex, r int) []string {
	if r <= 0 || len(nodes) == 0 {
		return nil
	}
	rs := make([]ranked, len(nodes))
	for i, n := range nodes {
		rs[i] = ranked{id: n, score: score(n, blockID, shardIndex)}
	}
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].score != rs[j].score {
			return rs[i].score > rs[j].score
		}
		return rs[i].id < rs[j].id // tie-break deterministically
	})
	if r > len(rs) {
		r = len(rs)
	}
	out := make([]string, r)
	for i := 0; i < r; i++ {
		out[i] = rs[i].id
	}
	return out
}

// IsHolder reports whether nodeID is among the top-r holders for the shard.
func IsHolder(nodeID string, nodes []string, blockID types.Hash, shardIndex, r int) bool {
	for _, h := range Holders(nodes, blockID, shardIndex, r) {
		if h == nodeID {
			return true
		}
	}
	return false
}

// AssignedShards returns the shard indices (of total) that nodeID must hold for
// a block, given the full node set and replication factor r.
func AssignedShards(nodeID string, nodes []string, blockID types.Hash, total, r int) []int {
	var out []int
	for i := 0; i < total; i++ {
		if IsHolder(nodeID, nodes, blockID, i, r) {
			out = append(out, i)
		}
	}
	return out
}

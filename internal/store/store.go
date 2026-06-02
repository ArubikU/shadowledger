// Package store persists block headers (with shard sets) and the erasure shards
// this node is responsible for, under a data directory.
package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ArubikU/shadowledger/internal/types"
)

// Store is the on-disk persistence layer.
type Store struct {
	dir string
	mu  sync.RWMutex
}

// headerRecord is what we persist per block: the header plus its shard set.
type headerRecord struct {
	Header   types.Header   `json:"header"`
	ShardSet types.ShardSet `json:"shard_set"`
}

// Open prepares the directory layout under dir.
func Open(dir string) (*Store, error) {
	for _, sub := range []string{"blocks", "shards", "keys", "logs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir}, nil
}

// Dir returns the data directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) headerPath(height uint64) string {
	return filepath.Join(s.dir, "blocks", fmt.Sprintf("%020d.hdr", height))
}

func (s *Store) shardDir(blockID types.Hash) string {
	return filepath.Join(s.dir, "shards", hex.EncodeToString(blockID[:]))
}

func (s *Store) logsSetPath(height uint64) string {
	return filepath.Join(s.dir, "logs", fmt.Sprintf("%020d.set", height))
}

// PutLogsSet persists the erasure shard-set COMMITMENT for a block's logs (just
// the hashes/spec — the log DATA lives in rendezvous-distributed shards, not
// here). Nothing is written when the block emitted no logs.
func (s *Store) PutLogsSet(height uint64, set types.ShardSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(set)
	if err != nil {
		return err
	}
	return os.WriteFile(s.logsSetPath(height), b, 0o644)
}

// GetLogsSet loads a block's log shard-set commitment (ok=false if none).
func (s *Store) GetLogsSet(height uint64) (types.ShardSet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.logsSetPath(height))
	if err != nil {
		return types.ShardSet{}, false
	}
	var set types.ShardSet
	if json.Unmarshal(b, &set) != nil {
		return types.ShardSet{}, false
	}
	return set, true
}

func (s *Store) blockPath(id types.Hash) string {
	return filepath.Join(s.dir, "bodies", hex.EncodeToString(id[:])+".json")
}

// PutBlock persists a full block body by id (for reorg replay across restarts +
// side branches). Bounded pruning is a later concern.
func (s *Store) PutBlock(id types.Hash, b any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.dir, "bodies"), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(s.blockPath(id), raw, 0o644)
}

// GetBlockRaw returns the raw JSON of a stored block body (nil if absent).
func (s *Store) GetBlockRaw(id types.Hash) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, err := os.ReadFile(s.blockPath(id))
	if err != nil {
		return nil
	}
	return raw
}

// PutState persists a named state snapshot (e.g. the reorg replay base).
func (s *Store) PutState(name string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, name), raw, 0o644)
}

// GetStateRaw returns a named state snapshot's raw JSON (nil if absent).
func (s *Store) GetStateRaw(name string) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return nil
	}
	return raw
}

// PutHeader persists a header + shard set at its height.
func (s *Store) PutHeader(h types.Header, set types.ShardSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := headerRecord{Header: h, ShardSet: set}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.headerPath(h.Height), b, 0o644)
}

// GetHeader loads the header + shard set at height.
func (s *Store) GetHeader(height uint64) (types.Header, types.ShardSet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.headerPath(height))
	if err != nil {
		return types.Header{}, types.ShardSet{}, err
	}
	var rec headerRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return types.Header{}, types.ShardSet{}, err
	}
	return rec.Header, rec.ShardSet, nil
}

// HeaderByID scans for a header matching the block id.
func (s *Store) HeaderByID(id types.Hash) (types.Header, types.ShardSet, error) {
	h := s.Height()
	for i := int64(h); i >= 0; i-- {
		hdr, set, err := s.GetHeader(uint64(i))
		if err != nil {
			continue
		}
		if hdr.ID() == id {
			return hdr, set, nil
		}
	}
	return types.Header{}, types.ShardSet{}, os.ErrNotExist
}

// Height returns the greatest stored block height (0 if only genesis or empty).
func (s *Store) Height() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "blocks"))
	if err != nil {
		return 0
	}
	var max uint64
	var found bool
	for _, e := range entries {
		var h uint64
		if _, err := fmt.Sscanf(e.Name(), "%020d.hdr", &h); err == nil {
			if !found || h > max {
				max, found = h, true
			}
		}
	}
	return max
}

// HasHeaders reports whether any block header is stored.
func (s *Store) HasHeaders() bool {
	entries, err := os.ReadDir(filepath.Join(s.dir, "blocks"))
	return err == nil && len(entries) > 0
}

// PutShard writes one shard's bytes for a block.
func (s *Store) PutShard(blockID types.Hash, index int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.shardDir(blockID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.shard", index)), data, 0o644)
}

// GetShard reads a stored shard, or os.ErrNotExist if absent.
func (s *Store) GetShard(blockID types.Hash, index int) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return os.ReadFile(filepath.Join(s.shardDir(blockID), fmt.Sprintf("%d.shard", index)))
}

// HeldShards lists the shard indices stored locally for a block.
func (s *Store) HeldShards(blockID types.Hash) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.shardDir(blockID))
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		var i int
		if _, err := fmt.Sscanf(e.Name(), "%d.shard", &i); err == nil {
			out = append(out, i)
		}
	}
	return out
}

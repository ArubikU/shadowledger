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

func (s *Store) logsPath(height uint64) string {
	return filepath.Join(s.dir, "logs", fmt.Sprintf("%020d.json", height))
}

// PutLogs persists a block's event logs (any JSON-serializable value). Skips
// writing when there are no logs.
func (s *Store) PutLogs(height uint64, logs any) error {
	b, err := json.Marshal(logs)
	if err != nil {
		return err
	}
	if string(b) == "null" || string(b) == "[]" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.dir, "logs"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.logsPath(height), b, 0o644)
}

// GetLogs returns the raw JSON logs at a height (or "[]" if none).
func (s *Store) GetLogs(height uint64) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.logsPath(height))
	if err != nil {
		return []byte("[]")
	}
	return b
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

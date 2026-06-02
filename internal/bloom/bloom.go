// Package bloom is a small Bloom filter for "do I hold this shard?" lookups.
package bloom

import (
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"math"
	"os"
	"sync"

	"github.com/ArubikU/shadowledger/internal/types"
)

// Filter is a thread-safe Bloom filter.
type Filter struct {
	mu   sync.RWMutex
	Bits []uint64 `json:"bits"`
	M    uint64   `json:"m"` // number of bits
	K    uint64   `json:"k"` // number of hash functions
}

// New sizes a filter for n expected items at false-positive rate p.
func New(n int, p float64) *Filter {
	if n < 1 {
		n = 1
	}
	if p <= 0 || p >= 1 {
		p = 0.01
	}
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	if m < 64 {
		m = 64
	}
	k := uint64(math.Round(float64(m) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	return &Filter{Bits: make([]uint64, (m+63)/64), M: m, K: k}
}

func (f *Filter) hashes(h types.Hash) (uint64, uint64) {
	h1 := fnv.New64a()
	h1.Write(h[:])
	a := h1.Sum64()
	b := binary.BigEndian.Uint64(h[24:32]) | 1 // ensure odd, nonzero
	return a, b
}

// Add inserts a shard hash.
func (f *Filter) Add(h types.Hash) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, b := f.hashes(h)
	for i := uint64(0); i < f.K; i++ {
		bit := (a + i*b) % f.M
		f.Bits[bit/64] |= 1 << (bit % 64)
	}
}

// Test reports whether h is possibly present (no false negatives).
func (f *Filter) Test(h types.Hash) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	a, b := f.hashes(h)
	for i := uint64(0); i < f.K; i++ {
		bit := (a + i*b) % f.M
		if f.Bits[bit/64]&(1<<(bit%64)) == 0 {
			return false
		}
	}
	return true
}

// Save persists the filter to disk as JSON.
func (f *Filter) Save(path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Load reads a filter from disk.
func Load(path string) (*Filter, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Filter
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Package erasure wraps Reed-Solomon coding to fragment block bodies into
// shards and rebuild them from any K of K+M shards.
package erasure

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/klauspost/reedsolomon"

	"github.com/ArubikU/shadowledger/internal/types"
)

// Errors.
var (
	ErrBadSpec    = errors.New("erasure: K and M must be >= 1")
	ErrShortInput = errors.New("erasure: not enough shards to reconstruct")
)

// Encode splits body into K data + M parity shards.
//
// Layout: the first 8 bytes prepended to the body store the original length,
// so the data is self-describing after reconstruction. The padded payload is
// chunked into K equal data shards; M parity shards are computed over them.
func Encode(body []byte, spec types.ShardSpec) (shards [][]byte, paddedLen, origLen int, err error) {
	k, m := int(spec.K), int(spec.M)
	if k < 1 || m < 1 {
		return nil, 0, 0, ErrBadSpec
	}
	// Prepend a 4-byte length header to the body.
	framed := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(framed[:4], uint32(len(body)))
	copy(framed[4:], body)

	// Per-shard size, rounded up so K shards cover the framed payload.
	shardLen := (len(framed) + k - 1) / k
	if shardLen == 0 {
		shardLen = 1
	}
	padded := make([]byte, shardLen*k)
	copy(padded, framed)

	enc, err := reedsolomon.New(k, m)
	if err != nil {
		return nil, 0, 0, err
	}
	all, err := enc.Split(padded)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := enc.Encode(all); err != nil {
		return nil, 0, 0, err
	}
	return all, shardLen, len(body), nil
}

// Reconstruct rebuilds the original body from a sparse shard set. Missing
// shards must be nil entries in shards (len == K+M). It verifies and returns
// the original body bytes.
func Reconstruct(shards [][]byte, spec types.ShardSpec, origLen int) ([]byte, error) {
	k, m := int(spec.K), int(spec.M)
	if k < 1 || m < 1 {
		return nil, ErrBadSpec
	}
	if len(shards) != k+m {
		return nil, ErrShortInput
	}
	present := 0
	for _, s := range shards {
		if s != nil {
			present++
		}
	}
	if present < k {
		return nil, ErrShortInput
	}
	enc, err := reedsolomon.New(k, m)
	if err != nil {
		return nil, err
	}
	if err := enc.Reconstruct(shards); err != nil {
		return nil, err
	}
	ok, err := enc.Verify(shards)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("erasure: reconstructed shards fail parity check")
	}
	// Join the K data shards back into the padded payload.
	var joined []byte
	for i := 0; i < k; i++ {
		joined = append(joined, shards[i]...)
	}
	if len(joined) < 4 {
		return nil, ErrShortInput
	}
	n := binary.BigEndian.Uint32(joined[:4])
	if int(n) != origLen || int(n)+4 > len(joined) {
		return nil, errors.New("erasure: length mismatch after reconstruct")
	}
	return joined[4 : 4+n], nil
}

// ShardHash binds a shard to its block and index, so corruption is detectable.
func ShardHash(blockID types.Hash, index int, data []byte) types.Hash {
	h := sha256.New()
	h.Write(blockID[:])
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], uint32(index))
	h.Write(idx[:])
	h.Write(data)
	var out types.Hash
	copy(out[:], h.Sum(nil))
	return out
}

// BuildShardSet encodes a body and returns the shards plus their commitment set.
func BuildShardSet(blockID types.Hash, body []byte, spec types.ShardSpec) ([][]byte, types.ShardSet, error) {
	shards, paddedLen, origLen, err := Encode(body, spec)
	if err != nil {
		return nil, types.ShardSet{}, err
	}
	set := types.ShardSet{
		BlockID:   blockID,
		Spec:      spec,
		PaddedLen: paddedLen,
		OrigLen:   origLen,
		ShardHash: make([]types.Hash, len(shards)),
	}
	for i, s := range shards {
		set.ShardHash[i] = ShardHash(blockID, i, s)
	}
	return shards, set, nil
}

// VerifyShard checks raw shard bytes against the committed hash in a set.
func VerifyShard(set types.ShardSet, index int, data []byte) bool {
	if index < 0 || index >= len(set.ShardHash) {
		return false
	}
	return ShardHash(set.BlockID, index, data) == set.ShardHash[index]
}

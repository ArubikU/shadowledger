package erasure

import (
	"bytes"
	"testing"

	"github.com/ArubikU/shadowledger/internal/types"
)

func TestEncodeReconstructAllPresent(t *testing.T) {
	body := []byte("ShadowLedger fragmented history payload — any K of K+M rebuild it.")
	spec := types.ShardSpec{K: 4, M: 2}
	shards, _, origLen, err := Encode(body, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 6 {
		t.Fatalf("want 6 shards, got %d", len(shards))
	}
	got, err := Reconstruct(shards, spec, origLen)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("round-trip mismatch")
	}
}

func TestReconstructWithLostShards(t *testing.T) {
	body := bytes.Repeat([]byte("shard-loss-test "), 100)
	spec := types.ShardSpec{K: 4, M: 2}
	shards, _, origLen, err := Encode(body, spec)
	if err != nil {
		t.Fatal(err)
	}
	// Drop M=2 shards (max tolerable): indices 1 and 4.
	shards[1] = nil
	shards[4] = nil
	got, err := Reconstruct(shards, spec, origLen)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("reconstruct after 2 losses mismatch")
	}
}

func TestReconstructTooFewFails(t *testing.T) {
	body := []byte("not enough shards")
	spec := types.ShardSpec{K: 4, M: 2}
	shards, _, origLen, _ := Encode(body, spec)
	// Drop 3 shards (> M); must fail.
	shards[0] = nil
	shards[1] = nil
	shards[2] = nil
	if _, err := Reconstruct(shards, spec, origLen); err == nil {
		t.Fatal("expected reconstruction failure with too few shards")
	}
}

func TestShardHashDetectsCorruption(t *testing.T) {
	body := []byte("integrity")
	spec := types.ShardSpec{K: 2, M: 1}
	var blockID types.Hash
	blockID[0] = 0xAB
	shards, set, err := BuildShardSet(blockID, body, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyShard(set, 0, shards[0]) {
		t.Fatal("valid shard rejected")
	}
	bad := append([]byte{}, shards[0]...)
	bad[0] ^= 0xFF
	if VerifyShard(set, 0, bad) {
		t.Fatal("corrupted shard accepted")
	}
}

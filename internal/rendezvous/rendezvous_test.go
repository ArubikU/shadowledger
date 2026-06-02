package rendezvous

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/types"
)

func TestDeterministic(t *testing.T) {
	nodes := []string{"node-a", "node-b", "node-c", "node-d"}
	var b types.Hash
	b[0] = 0x11
	h1 := Holders(nodes, b, 0, 2)
	h2 := Holders(nodes, b, 0, 2)
	if len(h1) != 2 {
		t.Fatalf("want 2 holders, got %d", len(h1))
	}
	for i := range h1 {
		if h1[i] != h2[i] {
			t.Fatal("non-deterministic holder selection")
		}
	}
}

func TestStableUnderChurn(t *testing.T) {
	var b types.Hash
	b[3] = 0x99
	base := []string{"a", "b", "c", "d", "e"}
	before := Holders(base, b, 1, 2)

	// Add a new node; existing assignment should be largely stable (HRW property).
	churned := append(append([]string{}, base...), "f")
	after := Holders(churned, b, 1, 2)

	// Top holder only changes if the new node outranks it for this shard.
	if before[0] != after[0] && after[0] != "f" {
		t.Fatalf("top holder changed unexpectedly: %s -> %s", before[0], after[0])
	}
}

func TestAssignmentCoversShards(t *testing.T) {
	nodes := []string{"n1", "n2", "n3"}
	var b types.Hash
	b[0] = 0x42
	total, repl := 6, 2
	counts := map[int]int{}
	for _, n := range nodes {
		for _, s := range AssignedShards(n, nodes, b, total, repl) {
			counts[s]++
		}
	}
	// Each shard must be assigned to exactly repl holders.
	for i := 0; i < total; i++ {
		if counts[i] != repl {
			t.Fatalf("shard %d assigned to %d holders, want %d", i, counts[i], repl)
		}
	}
}

package netparams

import "testing"

func TestSpecScalesAndStaysValid(t *testing.T) {
	for _, n := range []int{1, 2, 3, 6, 12, 24, 100} {
		s := Spec(n)
		if s.K < 1 || s.M < 1 {
			t.Fatalf("n=%d: invalid spec K=%d M=%d", n, s.K, s.M)
		}
		if s.Total() < minTotalShards || s.Total() > maxTotalShards {
			t.Fatalf("n=%d: total %d out of bounds", n, s.Total())
		}
	}
	// More nodes -> at least as many data shards (monotonic up to the cap).
	if Spec(24).K < Spec(3).K {
		t.Fatal("data shards did not scale up with node count")
	}
}

func TestReplicationGrows(t *testing.T) {
	if Replication(1) != 1 || Replication(5) != 2 || Replication(20) != 3 {
		t.Fatalf("replication policy off: %d %d %d", Replication(1), Replication(5), Replication(20))
	}
}

func TestGenesisSpecFixed(t *testing.T) {
	// Must be constant regardless of call site (determinism of block 0).
	a, b := GenesisSpec(), GenesisSpec()
	if a != b || a.K != 2 || a.M != 1 {
		t.Fatalf("genesis spec not fixed: %+v", a)
	}
}

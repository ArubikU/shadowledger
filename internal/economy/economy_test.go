package economy

import "testing"

func TestHalvingSchedule(t *testing.T) {
	cases := []struct {
		height uint64
		want   uint64
	}{
		{0, 50 * Coin},
		{HalvingInterval - 1, 50 * Coin},
		{HalvingInterval, 25 * Coin},
		{2 * HalvingInterval, 12_50000000},
		{3 * HalvingInterval, 6_25000000},
	}
	for _, c := range cases {
		if got := BlockReward(c.height, 0); got != c.want {
			t.Fatalf("height %d: reward %d, want %d", c.height, got, c.want)
		}
	}
}

func TestSupplyCapClamps(t *testing.T) {
	// Already at the cap: no further emission.
	if got := BlockReward(0, MaxSupply); got != 0 {
		t.Fatalf("reward past cap = %d, want 0", got)
	}
	// Near the cap: reward is clamped to the remaining headroom.
	minted := MaxSupply - 10
	if got := BlockReward(0, minted); got != 10 {
		t.Fatalf("clamped reward = %d, want 10", got)
	}
}

func TestSubsidyEventuallyZero(t *testing.T) {
	if got := BlockReward(64*HalvingInterval, 0); got != 0 {
		t.Fatalf("subsidy after 64 halvings = %d, want 0", got)
	}
}

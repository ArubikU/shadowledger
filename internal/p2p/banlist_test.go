package p2p

import (
	"testing"
	"time"
)

func TestBanlistThresholdAndExpiry(t *testing.T) {
	var clk time.Time = time.Unix(1000, 0)
	now := func() time.Time { return clk }
	b := NewBanlist(3, time.Minute, now)

	if b.Strike("x") || b.Strike("x") {
		t.Fatal("banned before threshold")
	}
	if !b.Strike("x") { // 3rd strike
		t.Fatal("not banned at threshold")
	}
	if !b.Banned("x") {
		t.Fatal("should be banned")
	}
	// Expire: advance past base (1m) — first ban is base<<0 = 1m.
	clk = clk.Add(61 * time.Second)
	if b.Banned("x") {
		t.Fatal("ban should have expired")
	}
	// Unknown keys never banned.
	if b.Banned("y") || b.Banned("") {
		t.Fatal("unknown/empty banned")
	}
}

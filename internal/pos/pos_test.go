package pos

import "testing"

func TestChallengeDeterministicAndBindsData(t *testing.T) {
	var nonce [32]byte
	nonce[0] = 0x7e
	a := Challenge(nonce, []byte("shard-bytes"))
	b := Challenge(nonce, []byte("shard-bytes"))
	if a != b {
		t.Fatal("challenge not deterministic")
	}
	if Challenge(nonce, []byte("shard-bytez")) == a {
		t.Fatal("challenge did not bind shard bytes")
	}
	var other [32]byte
	other[0] = 0x01
	if Challenge(other, []byte("shard-bytes")) == a {
		t.Fatal("challenge did not bind nonce")
	}
}

func TestScoreboard(t *testing.T) {
	var clk int64 = 100
	b := NewScoreboard(func() int64 { return clk })
	b.Pass("n1")
	clk = 200
	b.Pass("n1")
	b.Miss("n1")
	s := b.Snapshot()["n1"]
	if s.Pass != 2 || s.Miss != 1 {
		t.Fatalf("got pass=%d miss=%d", s.Pass, s.Miss)
	}
	if s.LastSeen != 200 {
		t.Fatalf("last seen = %d", s.LastSeen)
	}
	if got := s.Ratio(); got < 0.66 || got > 0.67 {
		t.Fatalf("ratio = %f", got)
	}
	if b.Eligible("n1", 0.8) {
		t.Fatal("should be ineligible at 0.8 threshold")
	}
	if !b.Eligible("n1", 0.5) {
		t.Fatal("should be eligible at 0.5 threshold")
	}
	if !b.Eligible("unknown", 0.99) {
		t.Fatal("never-challenged node should get benefit of the doubt")
	}
}

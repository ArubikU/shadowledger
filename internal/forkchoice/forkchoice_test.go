package forkchoice

import (
	"testing"

	"github.com/ArubikU/shadowledger/internal/types"
)

func h(b byte) types.Hash { var x types.Hash; x[0] = b; return x }

func TestHeaviestWins(t *testing.T) {
	tr := New()
	g := h(0) // genesis (parent = zero)
	tr.Add(g, types.Hash{}, 0, 1)
	// Branch A: g <- a1 <- a2   (weight 1 each -> cum 3 at a2)
	a1, a2 := h(10), h(11)
	tr.Add(a1, g, 1, 1)
	tr.Add(a2, a1, 2, 1)
	// Branch B: g <- b1          (cum 2)
	b1 := h(20)
	tr.Add(b1, g, 1, 1)

	best, cum := tr.Best()
	if best != a2 || cum != 3 {
		t.Fatalf("best=%x cum=%d, want a2 cum=3", best[:2], cum)
	}

	// Now branch B gets heavier (a high-weight block, e.g. very available).
	b2 := h(21)
	tr.Add(b2, b1, 2, 100)
	best, cum = tr.Best()
	if best != b2 || cum != 102 {
		t.Fatalf("after heavy block best=%x cum=%d, want b2 cum=102", best[:2], cum)
	}
}

func TestDeterministicTiebreak(t *testing.T) {
	tr := New()
	g := h(0)
	tr.Add(g, types.Hash{}, 0, 1)
	hi, lo := h(0xAA), h(0x11)
	tr.Add(hi, g, 1, 5)
	tr.Add(lo, g, 1, 5) // same cumulative weight
	best, _ := tr.Best()
	if best != lo {
		t.Fatalf("tie not broken by lowest id: got %x", best[:2])
	}
}

func TestCommonAncestor(t *testing.T) {
	tr := New()
	g := h(1) // genesis id (non-zero; only its PrevHash is zero)
	tr.Add(g, types.Hash{}, 0, 1)
	a1, a2 := h(10), h(11)
	b1 := h(20)
	tr.Add(a1, g, 1, 1)
	tr.Add(a2, a1, 2, 1)
	tr.Add(b1, g, 1, 1)
	anc, ok := tr.CommonAncestor(a2, b1)
	if !ok || anc != g {
		t.Fatalf("common ancestor = %x ok=%v, want genesis", anc[:2], ok)
	}
}

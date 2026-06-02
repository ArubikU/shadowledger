package version

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.11.0", "v0.10.0", true},
		{"v0.10.0", "v0.10.0", false},
		{"v0.9.0", "v0.10.0", false},
		{"v1.0.0", "v0.10.0", true},
		{"v0.10.1", "v0.10.0", true},
		{"v0.11.0", "dev", false}, // local build never "out of date"
		{"", "v0.10.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q,%q)=%v want %v", c.latest, c.current, got, c.want)
		}
	}
}

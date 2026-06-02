package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LatestRelease fetches the newest published release tag from GitHub. Best-effort
// (informational only — ShadowLedger never auto-applies updates).
func LatestRelease(timeout time.Duration) (string, error) {
	c := &http.Client{Timeout: timeout}
	resp, err := c.Get("https://api.github.com/repos/" + Repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version: github returned %d", resp.StatusCode)
	}
	var out struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Tag, nil
}

// IsNewer reports whether release tag `latest` is newer than `current`
// (dotted numeric compare, leading "v" ignored). A "dev" current is never
// considered out of date (it's a local build).
func IsNewer(latest, current string) bool {
	if current == "dev" || latest == "" {
		return false
	}
	return cmp(parse(latest), parse(current)) > 0
}

func parse(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

func cmp(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

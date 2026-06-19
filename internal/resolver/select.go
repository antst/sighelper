package resolver

import (
	"os"
	"sort"

	"github.com/antst/sighelper/internal/signkey"
)

// Resolve runs the full engine — discover, trust, probe, select — and returns the chosen
// socket (or nil) along with every candidate considered (for verbose reporting).
func Resolve(cfg Config) SelectionOutcome {
	uid := os.Getuid()
	cands := Discover(cfg)
	for _, c := range cands {
		checkTrust(c, uid)
	}
	probeAll(cands, cfg)

	var eligible []*Candidate
	for _, c := range cands {
		if c.Reject != RejectNone || !c.Live {
			continue
		}
		if cfg.RequiredKey != nil && !holdsKey(c, cfg.RequiredKey) {
			c.Reject = RejectWrongKey
			continue
		}
		eligible = append(eligible, c)
	}

	// Deterministic selection: newest mtime, then lexicographically first path (FR-006, R6).
	sort.Slice(eligible, func(i, j int) bool {
		if !eligible[i].ModTime.Equal(eligible[j].ModTime) {
			return eligible[i].ModTime.After(eligible[j].ModTime)
		}
		return eligible[i].Path < eligible[j].Path
	})

	out := SelectionOutcome{Considered: cands}
	if len(eligible) > 0 {
		out.Chosen = eligible[0]
	}
	return out
}

func holdsKey(c *Candidate, want []byte) bool {
	blobs := make([][]byte, len(c.Identities))
	for i, id := range c.Identities {
		blobs[i] = id.Blob
	}
	return signkey.Match(want, blobs)
}

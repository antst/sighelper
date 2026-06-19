package resolver

import (
	"os"
	"path/filepath"
)

// Discover returns candidate sockets from the configured glob plus the current
// SSH_AUTH_SOCK, deduplicated by resolved path (FR-001). Symlinks are followed here so
// that dedup and downstream trust checks operate on the real path (research R3).
func Discover(cfg Config) []*Candidate {
	seen := make(map[string]bool)
	var cands []*Candidate

	add := func(raw string) {
		if raw == "" {
			return
		}
		resolved, err := filepath.EvalSymlinks(raw)
		if err != nil {
			// Path missing or dangling: keep the raw path so it surfaces as a
			// rejected candidate (not-a-socket / dead) rather than vanishing.
			resolved = raw
		}
		if seen[resolved] {
			return
		}
		seen[resolved] = true
		cands = append(cands, &Candidate{RawPath: raw, Path: resolved})
	}

	matches, _ := filepath.Glob(cfg.Pattern)
	for _, m := range matches {
		add(m)
	}
	add(os.Getenv("SSH_AUTH_SOCK"))

	return cands
}

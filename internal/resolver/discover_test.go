package resolver

import (
	"os"
	"path/filepath"
	"testing"
)

// Discover deduplicates the glob match and $SSH_AUTH_SOCK when they resolve to one path.
func TestDiscoverDedup(t *testing.T) {
	dir := mkdir0700(t, shortDir(t), "a")
	sock := startAgent(t, dir, nil)
	t.Setenv("SSH_AUTH_SOCK", sock)
	cands := Discover(Config{Pattern: filepath.Join(dir, "agent.sock")})
	if len(cands) != 1 {
		t.Fatalf("expected 1 deduped candidate, got %d", len(cands))
	}
}

// A glob match that is a dangling symlink falls back to its raw path (EvalSymlinks error).
func TestDiscoverRawFallback(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	dir := mkdir0700(t, shortDir(t), "a")
	link := filepath.Join(dir, "agent.sock")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), link); err != nil {
		t.Fatal(err)
	}
	cands := Discover(Config{Pattern: filepath.Join(dir, "agent.sock")})
	if len(cands) != 1 || cands[0].Path != link {
		t.Fatalf("expected raw-path fallback to %q, got %+v", link, cands)
	}
}

// An untrusted (non-socket) glob match is skipped by the probe loop; the good socket wins.
func TestResolveSkipsUntrusted(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	parent := shortDir(t)
	good := mkdir0700(t, parent, "good")
	goodSock := startAgent(t, good, nil)
	bad := mkdir0700(t, parent, "bad")
	if err := os.WriteFile(filepath.Join(bad, "agent.sock"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Resolve(Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: probeTimeout})
	if out.Chosen == nil || out.Chosen.Path != resolved(t, goodSock) {
		t.Fatalf("expected the good socket, got %+v", out.Chosen)
	}
}

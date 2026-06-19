package resolver

import (
	"path/filepath"
	"testing"
	"time"
)

// probe's dial-failure path: a path with no listener is rejected as dead (distinct from the
// hung-agent List() timeout path covered in resolve_test.go).
func TestProbeDialFailure(t *testing.T) {
	c := &Candidate{Path: filepath.Join(shortDir(t), "no-listener.sock")}
	probe(c, Config{Timeout: 100 * time.Millisecond})
	if c.Live {
		t.Fatal("a path with no listener must not be live")
	}
	if c.Reject != RejectDead {
		t.Fatalf("got %q, want RejectDead", c.Reject)
	}
}

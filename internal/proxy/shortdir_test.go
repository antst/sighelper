package proxy

import (
	"os"
	"testing"
)

// shortDir returns a short-pathed temp directory under /tmp. macOS caps unix-socket paths
// (sun_path) at 104 bytes, and t.TempDir() under $TMPDIR can exceed that; /tmp stays short
// on Linux and macOS (where it resolves to /private/tmp).
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "shk")
	if err != nil {
		t.Fatalf("shortDir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

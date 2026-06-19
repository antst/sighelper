package main

import (
	"os"
	"testing"
)

// TestMain makes git config hermetic for this package: the resolver's signing-key
// auto-detection (signkey.DetermineKey) must not read the developer's real global key, or
// tests using empty in-memory agents would be (correctly) rejected for not holding it.
// /dev/null is an empty config; the repo's local config carries no signing settings.
func TestMain(m *testing.M) {
	_ = os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	_ = os.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	os.Exit(m.Run())
}

// shortDir: see internal/resolver/shortdir_test.go — keeps unix-socket paths under macOS's
// 104-byte sun_path limit.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "shk")
	if err != nil {
		t.Fatalf("shortDir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

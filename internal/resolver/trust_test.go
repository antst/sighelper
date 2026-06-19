package resolver

import (
	"os"
	"path/filepath"
	"testing"
)

// checkTrust takes uid as a parameter, so foreign ownership is exercised by passing a uid
// other than the socket's real owner — no privileged setup needed.
func TestTrustForeignOwner(t *testing.T) {
	dir := mkdir0700(t, shortDir(t), "a")
	sock := startAgent(t, dir, nil)
	c := &Candidate{Path: resolved(t, sock)}
	checkTrust(c, os.Getuid()+1) // pretend a different user owns us
	if c.Reject != RejectForeign {
		t.Fatalf("got %q, want RejectForeign", c.Reject)
	}
	if c.Trusted {
		t.Fatal("foreign socket must not be trusted")
	}
}

func TestTrustUntrustedDir(t *testing.T) {
	parent := shortDir(t)
	dir := filepath.Join(parent, "ww")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil { // force group/world-writable past the umask
		t.Fatal(err)
	}
	sock := startAgent(t, dir, nil)
	c := &Candidate{Path: resolved(t, sock)}
	checkTrust(c, os.Getuid())
	if c.Reject != RejectUntrusted {
		t.Fatalf("got %q, want RejectUntrusted", c.Reject)
	}
}

func TestTrustNonSocket(t *testing.T) {
	dir := mkdir0700(t, shortDir(t), "a")
	f := filepath.Join(dir, "agent.sock")
	if err := os.WriteFile(f, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Candidate{Path: f}
	checkTrust(c, os.Getuid())
	if c.Reject != RejectNotSocket {
		t.Fatalf("got %q, want RejectNotSocket", c.Reject)
	}
}

func TestTrustMissing(t *testing.T) {
	c := &Candidate{Path: filepath.Join(shortDir(t), "does-not-exist")}
	checkTrust(c, os.Getuid())
	if c.Reject != RejectNotSocket {
		t.Fatalf("got %q, want RejectNotSocket", c.Reject)
	}
}

func TestTrustGood(t *testing.T) {
	dir := mkdir0700(t, shortDir(t), "a")
	sock := startAgent(t, dir, nil)
	c := &Candidate{Path: resolved(t, sock)}
	checkTrust(c, os.Getuid())
	if !c.Trusted || c.Reject != RejectNone {
		t.Fatalf("expected trusted, got trusted=%v reject=%q", c.Trusted, c.Reject)
	}
}

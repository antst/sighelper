package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// serveAgent serves an empty in-memory keyring on dir/agent.sock (dir must be 0700).
func serveAgent(t *testing.T, dir string) string {
	t.Helper()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	kr := agent.NewKeyring()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(kr, conn) }()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return sock
}

// T014 / FR-007 / FR-015: a live socket is printed to stdout, exit 0, stderr clean.
func TestRunResolverPrintsSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	dir := filepath.Join(shortDir(t), "a")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := serveAgent(t, dir)
	resolvedSock, _ := filepath.EvalSymlinks(sock)

	var out, errb bytes.Buffer
	code := run([]string{"--pattern", filepath.Join(dir, "agent.sock"), "--timeout", "500ms"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != resolvedSock {
		t.Fatalf("stdout=%q, want %q", out.String(), resolvedSock)
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", errb.String())
	}
}

// T014 / N1 / SC-007: no usable socket → exit 1, empty stdout, clear stderr message.
func TestRunResolverNoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	var out, errb bytes.Buffer
	code := run([]string{"--pattern", filepath.Join(shortDir(t), "none", "*"), "--timeout", "200ms"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout must be empty on failure, got %q", out.String())
	}
	if strings.TrimSpace(errb.String()) == "" {
		t.Fatal("expected a clear stderr message on failure")
	}
}

// FR-008: usage/config errors exit 2.
func TestRunResolverUsageError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--nonsuch"}, &out, &errb); code != 2 {
		t.Fatalf("unknown flag: exit %d, want 2", code)
	}
	if code := run([]string{"--timeout", "0s"}, &out, &errb); code != 2 {
		t.Fatalf("non-positive timeout: exit %d, want 2", code)
	}
}

// FR-002 / SC-003: a foreign-owned socket pointed at by SSH_AUTH_SOCK is never printed.
func TestRunResolverForeignNeverChosen(t *testing.T) {
	// A live socket owned by us but in a world-writable dir is untrusted and must not win.
	parent := shortDir(t)
	bad := filepath.Join(parent, "ww")
	if err := os.Mkdir(bad, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bad, 0o777); err != nil { // force group/world-writable past the umask
		t.Fatal(err)
	}
	sock := serveAgent(t, bad)
	t.Setenv("SSH_AUTH_SOCK", sock)

	var out, errb bytes.Buffer
	code := run([]string{"--pattern", filepath.Join(parent, "none", "*"), "--timeout", "300ms"}, &out, &errb)
	if code != 1 || out.Len() != 0 {
		t.Fatalf("untrusted socket must not be chosen: exit=%d stdout=%q", code, out.String())
	}
}

func TestHasYFlag(t *testing.T) {
	if !hasYFlag([]string{"-Y", "sign"}) {
		t.Fatal("expected proxy dispatch on -Y")
	}
	if hasYFlag([]string{"--verbose"}) {
		t.Fatal("resolver args must not trigger proxy")
	}
}

// FR-010: an unparseable --key override is a usage error (exit 2).
func TestRunResolverBadKey(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"--key", "ssh-ed25519 not-valid-base64!!", "--pattern", "/tmp/none-*/x"}, &out, &errb)
	if code != 2 {
		t.Fatalf("bad --key: exit %d, want 2", code)
	}
	if errb.Len() == 0 {
		t.Fatal("expected an error message on stderr")
	}
}

// US3: --verbose emits a CHOSEN line on stderr while stdout stays the bare path.
func TestRunResolverVerbose(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	dir := filepath.Join(shortDir(t), "a")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	serveAgent(t, dir)
	var out, errb bytes.Buffer
	code := run([]string{"--pattern", filepath.Join(dir, "agent.sock"), "--timeout", "500ms", "--verbose"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errb.String(), "CHOSEN") {
		t.Fatalf("verbose stderr missing CHOSEN: %q", errb.String())
	}
}

// A valid --key override parses (covers determineKey's success path); with no agent it exits 1.
func TestRunResolverValidKeyOverride(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	literal := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp)))
	var out, errb bytes.Buffer
	code := run([]string{"--key", literal, "--pattern", filepath.Join(shortDir(t), "none", "*"), "--timeout", "200ms"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

// run() dispatches -Y argv to the proxy; a bogus ssh-keygen makes exec fail fast → exit 2.
func TestRunProxyDispatch(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", filepath.Join(shortDir(t), "nonexistent-ssh-keygen"))
	if code := run([]string{"-Y", "verify", "-f", "/k.pub"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("proxy dispatch: exit %d, want 2", code)
	}
}

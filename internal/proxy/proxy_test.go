package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestParseY(t *testing.T) {
	op, key := parseY([]string{"-Y", "sign", "-n", "git", "-f", "/k.pub", "/buf"})
	if op != "sign" || key != "/k.pub" {
		t.Fatalf("got op=%q key=%q", op, key)
	}
	op, key = parseY([]string{"-Y", "verify", "-f", "/k.pub", "-n", "git"})
	if op != "verify" || key != "/k.pub" {
		t.Fatalf("got op=%q key=%q", op, key)
	}
}

func TestRealSSHKeygenOverride(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", "/custom/ssh-keygen")
	got, err := realSSHKeygen()
	if err != nil || got != "/custom/ssh-keygen" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestRealSSHKeygenFromPath(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", "")
	got, err := realSSHKeygen()
	if err != nil {
		t.Skipf("ssh-keygen not on PATH: %v", err)
	}
	if filepath.Base(got) != "ssh-keygen" {
		t.Fatalf("expected an ssh-keygen path, got %q", got)
	}
}

// FR-010: with no live agent, the proxy refuses to sign (exit 1) before any exec.
func TestPrepareSignNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("SIGHELPER_PATTERN", filepath.Join(shortDir(t), "none", "*"))
	if code := prepareSign(""); code != 1 {
		t.Fatalf("expected exit 1 with no agent, got %d", code)
	}
}

// Run: realSSHKeygen failure (no override, empty PATH) → exit 2 before any resolution.
func TestRunKeygenNotFound(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", "")
	t.Setenv("PATH", "")
	if code := Run([]string{"-Y", "sign", "-f", "/k.pub", "/buf"}); code != 2 {
		t.Fatalf("expected exit 2 when ssh-keygen missing, got %d", code)
	}
}

// Run: a sign op with no live agent fails (exit 1) before exec.
func TestRunSignNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("SIGHELPER_SSH_KEYGEN", "/usr/bin/ssh-keygen")
	t.Setenv("SIGHELPER_PATTERN", filepath.Join(shortDir(t), "none", "*"))
	if code := Run([]string{"-Y", "sign", "-f", "/k.pub", "/buf"}); code != 1 {
		t.Fatalf("expected exit 1 with no agent, got %d", code)
	}
}

// prepareSign with verbose enabled exercises the diagnostic report path.
func TestPrepareSignVerbose(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("SIGHELPER_VERBOSE", "1")
	t.Setenv("SIGHELPER_PATTERN", filepath.Join(shortDir(t), "none", "*"))
	if code := prepareSign(""); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

// Run + execKeygen: a non-sign op execs ssh-keygen; pointing at a bogus binary makes exec
// fail fast (ENOENT, no process replacement) → exit 2, exercising execKeygen.
func TestRunExecFailure(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", filepath.Join(shortDir(t), "nonexistent-ssh-keygen"))
	if code := Run([]string{"-Y", "verify", "-f", "/k.pub"}); code != 2 {
		t.Fatalf("expected exit 2 when exec fails, got %d", code)
	}
}

// realSSHKeygen returns an error when nothing is found on an empty PATH and no override.
func TestRealSSHKeygenNotFound(t *testing.T) {
	t.Setenv("SIGHELPER_SSH_KEYGEN", "")
	t.Setenv("PATH", "")
	if _, err := realSSHKeygen(); err == nil {
		t.Fatal("expected error when ssh-keygen is unavailable")
	}
}

// realSSHKeygen skips empty PATH elements, non-executable entries, and a ssh-keygen that is
// our own binary, then returns the genuine one.
func TestRealSSHKeygenSkipsSelfAndJunk(t *testing.T) {
	realPath, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	selfReal, err := filepath.EvalSymlinks(self)
	if err != nil {
		t.Fatal(err)
	}
	junk := t.TempDir() // a non-executable ssh-keygen
	if err := os.WriteFile(filepath.Join(junk, "ssh-keygen"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	selfDir := t.TempDir() // an ssh-keygen that is really us
	if err := os.Symlink(selfReal, filepath.Join(selfDir, "ssh-keygen")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIGHELPER_SSH_KEYGEN", "")
	t.Setenv("PATH", ":"+junk+":"+selfDir+":"+filepath.Dir(realPath)) // leading "" + junk + self + real
	got, err := realSSHKeygen()
	if err != nil {
		t.Fatalf("realSSHKeygen: %v", err)
	}
	if got != filepath.Join(filepath.Dir(realPath), "ssh-keygen") {
		t.Fatalf("got %q, want the real ssh-keygen", got)
	}
}

// SC-008 (core): the proxy resolves a live agent that holds the signing key (even with a
// dead SSH_AUTH_SOCK), and a real ssh-keygen sign through it produces a valid signature.
func TestSigningEndToEnd(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(shortDir(t), "agentdir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := serveKeyring(t, dir, priv)

	pubFile := filepath.Join(shortDir(t), "id.pub")
	if err := os.WriteFile(pubFile, ssh.MarshalAuthorizedKey(sp), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SSH_AUTH_SOCK", "/tmp/this-socket-is-dead/agent.0") // simulate a reconnect
	t.Setenv("SIGHELPER_PATTERN", filepath.Join(dir, "agent.sock"))

	if code := prepareSign(pubFile); code != 0 {
		t.Fatalf("prepareSign returned %d, want 0", code)
	}
	// The resolver sets SSH_AUTH_SOCK to the resolved (canonical) path; on macOS /tmp is a
	// symlink to /private/tmp, so compare against the symlink-resolved socket path.
	wantSock, err := filepath.EvalSymlinks(sock)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SSH_AUTH_SOCK"); got != wantSock {
		t.Fatalf("SSH_AUTH_SOCK = %q, want %q", got, wantSock)
	}

	data := filepath.Join(shortDir(t), "buf")
	if err := os.WriteFile(data, []byte("commit payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(keygen, "-Y", "sign", "-n", "git", "-f", pubFile, data)
	cmd.Env = os.Environ() // carries the SSH_AUTH_SOCK set by prepareSign
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen sign failed: %v\n%s", err, out)
	}
	if fi, err := os.Stat(data + ".sig"); err != nil || fi.Size() == 0 {
		t.Fatalf("expected a non-empty signature file: %v", err)
	}
}

// serveKeyring serves an in-memory agent holding priv on dir/agent.sock.
func serveKeyring(t *testing.T, dir string, priv ed25519.PrivateKey) string {
	t.Helper()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	kr := agent.NewKeyring()
	if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
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

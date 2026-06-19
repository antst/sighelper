package signkey

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRepo creates a throwaway git repo and runs git inside it for the duration of the test
// by switching the working directory (DetermineKey shells out to `git config`).
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	// Avoid inheriting the developer's real global signing config during the test.
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "nonexistent-global"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(dir, "nonexistent-system"))
	chdir(t, dir)
	return dir
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func gitSet(t *testing.T, dir, key, val string) {
	t.Helper()
	cmd := exec.Command("git", "config", key, val)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config %s: %v\n%s", key, err, out)
	}
}

func TestDetermineKeyNotSSH(t *testing.T) {
	gitRepo(t) // no gpg.format set → not ssh
	blob, err := DetermineKey()
	if err != nil || blob != nil {
		t.Fatalf("expected nil key for non-ssh config, got blob=%v err=%v", blob, err)
	}
}

func TestDetermineKeyNoSigningKey(t *testing.T) {
	dir := gitRepo(t)
	gitSet(t, dir, "gpg.format", "ssh")
	blob, err := DetermineKey()
	if err != nil || blob != nil {
		t.Fatalf("expected nil when signingkey unset, got blob=%v err=%v", blob, err)
	}
}

func TestDetermineKeyLiteral(t *testing.T) {
	dir := gitRepo(t)
	line, want := pubLine(t)
	gitSet(t, dir, "gpg.format", "ssh")
	gitSet(t, dir, "user.signingkey", line)
	blob, err := DetermineKey()
	if err != nil {
		t.Fatalf("DetermineKey: %v", err)
	}
	if !bytes.Equal(blob, want) {
		t.Fatal("blob mismatch from git-config literal key")
	}
}

func TestDetermineKeyFile(t *testing.T) {
	dir := gitRepo(t)
	line, want := pubLine(t)
	keyFile := filepath.Join(dir, "sign.pub")
	if err := os.WriteFile(keyFile, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitSet(t, dir, "gpg.format", "ssh")
	gitSet(t, dir, "user.signingkey", keyFile)
	blob, err := DetermineKey()
	if err != nil {
		t.Fatalf("DetermineKey: %v", err)
	}
	if !bytes.Equal(blob, want) {
		t.Fatal("blob mismatch from git-config key file")
	}
}

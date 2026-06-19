package signkey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// pubLine returns an authorized_keys line and its wire blob for a fresh ed25519 key.
func pubLine(t *testing.T) (line string, blob []byte) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))), sp.Marshal()
}

func TestParseKeyLiteral(t *testing.T) {
	line, blob := pubLine(t)
	got, err := ParseKey(line)
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if !bytes.Equal(got, blob) {
		t.Fatal("literal blob mismatch")
	}
}

func TestParseKeyLiteralWithComment(t *testing.T) {
	line, blob := pubLine(t)
	got, err := ParseKey(line + " user@host")
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if !bytes.Equal(got, blob) {
		t.Fatal("blob mismatch with trailing comment")
	}
}

func TestParseKeyFile(t *testing.T) {
	line, blob := pubLine(t)
	f := filepath.Join(t.TempDir(), "id_ed25519.pub")
	if err := os.WriteFile(f, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseKey(f)
	if err != nil {
		t.Fatalf("ParseKey(file): %v", err)
	}
	if !bytes.Equal(got, blob) {
		t.Fatal("file blob mismatch")
	}
}

func TestParseKeyErrors(t *testing.T) {
	if _, err := ParseKey(""); err == nil {
		t.Fatal("empty ref should error")
	}
	if _, err := ParseKey("/no/such/key/file"); err == nil {
		t.Fatal("missing file should error")
	}
	if _, err := ParseKey("ssh-ed25519 not-base64!!"); err == nil {
		t.Fatal("bad base64 should error")
	}
}

// git's literal signing-key form carries a "key::" prefix, which must be stripped.
func TestParseKeyGitLiteralPrefix(t *testing.T) {
	line, want := pubLine(t)
	got, err := ParseKey("key::" + line + " ant@example")
	if err != nil {
		t.Fatalf("ParseKey(key::...): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("key:: prefix not handled")
	}
}

func TestParseKeyHomePath(t *testing.T) {
	line, want := pubLine(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "k.pub"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseKey("~/k.pub")
	if err != nil {
		t.Fatalf("ParseKey(~/): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("home-path blob mismatch")
	}
}

func TestMatch(t *testing.T) {
	_, blob := pubLine(t)
	_, other := pubLine(t)
	if !Match(blob, [][]byte{other, blob}) {
		t.Fatal("expected match")
	}
	if Match(blob, [][]byte{other}) {
		t.Fatal("unexpected match")
	}
	if Match(blob, nil) {
		t.Fatal("empty set must not match")
	}
}

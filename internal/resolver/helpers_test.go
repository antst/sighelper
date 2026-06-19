package resolver

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// genKey returns a fresh ed25519 private key and its SSH public-key wire blob.
func genKey(t *testing.T) (ed25519.PrivateKey, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return priv, sp.Marshal()
}

// mkdir0700 makes a 0700 subdirectory (a trusted agent-socket directory).
func mkdir0700(t *testing.T, parent, name string) string {
	t.Helper()
	d := filepath.Join(parent, name)
	if err := os.Mkdir(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

// startAgent serves an in-memory keyring (optionally holding priv) on dir/agent.sock and
// returns the socket path. The listener is closed on cleanup.
func startAgent(t *testing.T, dir string, priv ed25519.PrivateKey) string {
	t.Helper()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	kr := agent.NewKeyring()
	if priv != nil {
		if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
			t.Fatal(err)
		}
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

// startHung listens on dir/agent.sock but never serves the connection, so a probe connects
// but List() blocks until the deadline — simulating a dead/hung agent.
func startHung(t *testing.T, dir string) string {
	t.Helper()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return sock
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return r
}

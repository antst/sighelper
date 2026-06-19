package resolver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const probeTimeout = 500 * time.Millisecond

// US1 happy path: a live owned agent is resolved and its resolved path returned.
func TestResolvePicksLiveSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	parent := shortDir(t)
	dir := mkdir0700(t, parent, "a")
	sock := startAgent(t, dir, nil)

	out := Resolve(Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: probeTimeout})
	if out.Chosen == nil {
		t.Fatal("expected a chosen socket")
	}
	if out.Chosen.Path != resolved(t, sock) {
		t.Fatalf("chosen %q, want %q", out.Chosen.Path, resolved(t, sock))
	}
}

// SC-007: nothing usable → no chosen socket.
func TestResolveNoUsableSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	out := Resolve(Config{Pattern: filepath.Join(shortDir(t), "none", "*"), Timeout: probeTimeout})
	if out.Chosen != nil {
		t.Fatalf("expected no socket, got %q", out.Chosen.Path)
	}
}

// SC-004: a socket that connects but whose agent never responds is rejected as dead.
func TestResolveRejectsHungAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	parent := shortDir(t)
	dir := mkdir0700(t, parent, "a")
	startHung(t, dir)

	out := Resolve(Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: 120 * time.Millisecond})
	if out.Chosen != nil {
		t.Fatal("hung agent must not be chosen")
	}
	if len(out.Considered) != 1 || out.Considered[0].Reject != RejectDead {
		t.Fatalf("expected RejectDead, got %+v", out.Considered)
	}
}

// SC-005: with several live agents holding the key, selection is newest-mtime then path,
// and is identical across runs.
func TestResolveDeterministicSelection(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	priv, blob := genKey(t)
	parent := shortDir(t)
	dirA := mkdir0700(t, parent, "a")
	dirB := mkdir0700(t, parent, "b")
	sockA := startAgent(t, dirA, priv)
	sockB := startAgent(t, dirB, priv)

	older := time.Now().Add(-1 * time.Hour)
	newer := time.Now()
	if err := os.Chtimes(sockA, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(sockB, newer, newer); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: probeTimeout, RequiredKey: blob}
	first := Resolve(cfg)
	second := Resolve(cfg)
	if first.Chosen == nil || first.Chosen.Path != resolved(t, sockB) {
		t.Fatalf("expected newest (sockB) chosen, got %+v", first.Chosen)
	}
	if second.Chosen == nil || second.Chosen.Path != first.Chosen.Path {
		t.Fatal("selection not deterministic across runs")
	}
}

// FR-010: only an agent holding the required key is eligible; others are RejectWrongKey.
func TestResolveKeyFilter(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	privA, _ := genKey(t)
	privB, blobB := genKey(t)
	parent := shortDir(t)
	dirA := mkdir0700(t, parent, "a")
	dirB := mkdir0700(t, parent, "b")
	startAgent(t, dirA, privA)
	sockB := startAgent(t, dirB, privB)

	out := Resolve(Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: probeTimeout, RequiredKey: blobB})
	if out.Chosen == nil || out.Chosen.Path != resolved(t, sockB) {
		t.Fatalf("expected key-B socket, got %+v", out.Chosen)
	}
	var sawWrongKey bool
	for _, c := range out.Considered {
		if c.Reject == RejectWrongKey {
			sawWrongKey = true
		}
	}
	if !sawWrongKey {
		t.Fatal("expected the non-holder to be RejectWrongKey")
	}
}

// FR-001: the current SSH_AUTH_SOCK is included as a candidate even outside the glob.
func TestResolveIncludesAuthSock(t *testing.T) {
	parent := shortDir(t)
	dir := mkdir0700(t, parent, "a")
	sock := startAgent(t, dir, nil)
	t.Setenv("SSH_AUTH_SOCK", sock)

	out := Resolve(Config{Pattern: filepath.Join(shortDir(t), "none", "*"), Timeout: probeTimeout})
	if out.Chosen == nil || out.Chosen.Path != resolved(t, sock) {
		t.Fatalf("expected SSH_AUTH_SOCK candidate chosen, got %+v", out.Chosen)
	}
}

// G1 / FR-005: probing must not mutate or create sockets — inode, mtime, and the
// directory listing are unchanged after resolution.
func TestResolveReadOnlyNoMutation(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	parent := shortDir(t)
	dir := mkdir0700(t, parent, "a")
	sock := startAgent(t, dir, nil)

	statBefore, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	entriesBefore, _ := os.ReadDir(dir)

	Resolve(Config{Pattern: filepath.Join(parent, "*", "agent.sock"), Timeout: probeTimeout})

	statAfter, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("socket vanished after probe: %v", err)
	}
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Fatal("probe modified the socket mtime")
	}
	if entriesAfter, _ := os.ReadDir(dir); len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("probe changed the directory listing: %d → %d", len(entriesBefore), len(entriesAfter))
	}
}

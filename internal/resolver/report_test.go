package resolver

import (
	"bytes"
	"strings"
	"testing"
)

// US3 / FR-012 + SC-006: verbose output explains each candidate and leaks no secrets.
func TestReportContentAndSecretSafety(t *testing.T) {
	secretBlob := []byte("THIS-IS-A-PUBLIC-KEY-BLOB-BYTES")
	chosen := &Candidate{
		Path: "/tmp/ssh-CCCC/agent.sock",
		Live: true,
		Identities: []Identity{{
			Blob:        secretBlob,
			Fingerprint: "SHA256:abc123",
			Comment:     "me@host",
		}},
	}
	rejected := &Candidate{RawPath: "/tmp/ssh-AAAA/agent.111", Reject: RejectForeign}
	out := SelectionOutcome{Chosen: chosen, Considered: []*Candidate{rejected, chosen}}

	var buf bytes.Buffer
	Report(&buf, out)
	got := buf.String()

	if !strings.Contains(got, "CHOSEN") {
		t.Error("missing CHOSEN line")
	}
	if !strings.Contains(got, "rejected: "+string(RejectForeign)) {
		t.Error("missing reject reason")
	}
	if !strings.Contains(got, "SHA256:abc123") {
		t.Error("fingerprint should be shown")
	}
	if strings.Contains(got, string(secretBlob)) {
		t.Error("key blob bytes leaked into verbose output")
	}
	for _, marker := range []string{"PRIVATE KEY", "BEGIN OPENSSH"} {
		if strings.Contains(got, marker) {
			t.Errorf("secret marker %q leaked", marker)
		}
	}
}

// A live, non-chosen candidate is reported as "eligible".
func TestReportEligibleBranch(t *testing.T) {
	chosen := &Candidate{Path: "/tmp/ssh-A/agent.1", Live: true}
	other := &Candidate{Path: "/tmp/ssh-B/agent.2", Live: true}
	out := SelectionOutcome{Chosen: chosen, Considered: []*Candidate{chosen, other}}

	var buf bytes.Buffer
	Report(&buf, out)
	got := buf.String()
	if !strings.Contains(got, "/tmp/ssh-B/agent.2  eligible") {
		t.Fatalf("expected an eligible line, got:\n%s", got)
	}
}

// Package resolver is the shared engine: it discovers candidate ssh-agent sockets,
// keeps only trusted ones owned by the current user, confirms liveness with a read-only
// agent handshake, and deterministically selects one. Both the resolver CLI and the
// signing proxy build on it.
package resolver

import "time"

// RejectReason explains why a candidate was dropped (empty means not rejected).
type RejectReason string

const (
	RejectNone      RejectReason = ""
	RejectNotSocket RejectReason = "not a socket"
	RejectForeign   RejectReason = "foreign owner"
	RejectUntrusted RejectReason = "untrusted location"
	RejectDead      RejectReason = "no response"
	RejectWrongKey  RejectReason = "agent does not hold the signing key"
)

// Config controls one resolution. RequiredKey is the public-key wire blob the chosen
// agent must hold; nil means key-agnostic selection.
type Config struct {
	Pattern     string
	Timeout     time.Duration
	Verbose     bool
	RequiredKey []byte
}

// Identity is a public identity reported by a live agent. Public information only —
// no private key material is ever held.
type Identity struct {
	Blob        []byte
	Fingerprint string
	Comment     string
}

// Candidate is one discovered potential agent endpoint, enriched as it passes the
// trust and liveness checks.
type Candidate struct {
	RawPath    string
	Path       string // resolved real path (symlinks followed)
	OwnerUID   uint32
	ModTime    time.Time
	Trusted    bool
	Live       bool
	Identities []Identity
	Reject     RejectReason
}

// SelectionOutcome is the result of one resolution. Contains no secret material.
type SelectionOutcome struct {
	Chosen     *Candidate
	Considered []*Candidate
}

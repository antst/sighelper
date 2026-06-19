# Phase 1 Data Model: SSH Agent Socket Resolver & Signing Helper

**Feature**: 001-ssh-agent-resolver | **Date**: 2026-06-19

The tool is stateless — these are in-memory value types for one invocation, not persisted
entities. Field types are indicative (Go), validation rules derive from the spec's FRs.

## Config

Resolved once at startup from flags (resolver) or environment (proxy). The only configurable
state the tool holds.

| Field | Type | Default | Source | Notes |
|-------|------|---------|--------|-------|
| `Pattern` | `string` | `/tmp/ssh-*/agent.*` | `--pattern` / `SIGHELPER_PATTERN` | glob for discovery (FR-001) |
| `Timeout` | `time.Duration` | `250ms` | `--timeout` / `SIGHELPER_TIMEOUT` | per-probe deadline (FR-013, R2) |
| `KeyOverride` | `string` | `""` | `--key` / proxy `-f` | path or literal public key (R4) |
| `Verbose` | `bool` | `false` | `-v/--verbose` / `SIGHELPER_VERBOSE` | per-candidate reasons (FR-012); the env var is the proxy-mode path (git owns argv) |
| `SSHKeygenPath` | `string` | PATH lookup | `--ssh-keygen` / `SIGHELPER_SSH_KEYGEN` | proxy only (R5) |

**Validation**: `Timeout > 0` else exit `2`. Unknown flags ⇒ exit `2`. `Pattern` is used
verbatim by the globber (an empty match set is not an error here — it surfaces later as exit `1`).

## Candidate

One discovered potential agent endpoint. Built during discovery, enriched during probing.

| Field | Type | Meaning |
|-------|------|---------|
| `RawPath` | `string` | path as discovered (glob match or `$SSH_AUTH_SOCK`) |
| `Path` | `string` | resolved real path after `EvalSymlinks` (R3) |
| `OwnerUID` | `uint32` | owner from `Lstat` of the resolved path |
| `ModTime` | `time.Time` | resolved socket mtime — selection tiebreak (R6) |
| `Trusted` | `bool` | passed socket-type + owner + parent-dir-perm checks (R3) |
| `Live` | `bool` | agent answered `List()` within `Timeout` (R1) |
| `Identities` | `[]Identity` | public keys the live agent holds (empty if not live) |
| `Reject` | `RejectReason` | why dropped, for verbose (`""` if selected/eligible) |

**Identity & uniqueness**: candidates are deduplicated by resolved `Path` (so the
`$SSH_AUTH_SOCK` entry collapses into its glob twin — FR-001).

**Lifecycle (per candidate)**:

```
discovered → resolved(EvalSymlinks) → trust-checked ──fail──▶ rejected (RejectUntrusted/RejectForeign/RejectNotSocket)
                                           │pass
                                           ▼
                                    liveness-probed ──fail──▶ rejected (RejectDead)
                                           │live
                                           ▼
                                     key-filtered ──no match──▶ rejected (RejectWrongKey)
                                           │match (or no key required)
                                           ▼
                                       eligible ──(selection R6)──▶ chosen
```

**Validation rules** (map to FRs): not-a-socket ⇒ `RejectNotSocket` (FR-001); foreign owner ⇒
`RejectForeign` (FR-002, never selected — SC-003); untrusted dir / bad symlink ⇒
`RejectUntrusted` (FR-003); no `List()` response ⇒ `RejectDead` (FR-004, SC-004); key required
but absent ⇒ `RejectWrongKey` (FR-010).

## Identity

A public identity reported by a live agent. Public information only — no private material ever
held (FR-011, SC-006).

| Field | Type | Meaning |
|-------|------|---------|
| `Blob` | `[]byte` | public-key wire bytes — exact match target (R4) |
| `Fingerprint` | `string` | SHA256 fingerprint — for verbose display only |
| `Comment` | `string` | agent-reported comment — for verbose display only |

## RequiredKey

The signing key the selection must honor, when determinable (R4). Absent ⇒ key-agnostic.

| Field | Type | Meaning |
|-------|------|---------|
| `Blob` | `[]byte` | parsed public-key wire bytes to match against `Identity.Blob` |
| `Source` | `enum` | `override` \| `gitconfig` \| `none` |

**Match rule**: a candidate satisfies the key iff some `Identity.Blob` equals `RequiredKey.Blob`
(byte-equal). When `Source == none`, every live candidate satisfies it.

## SelectionOutcome

The result of one resolver run. Contains no secret material (SC-006).

| Field | Type | Meaning |
|-------|------|---------|
| `Chosen` | `*Candidate` | selected socket, or `nil` |
| `ExitCode` | `int` | `0` chosen · `1` none usable · `2` usage/config error (FR-008) |
| `Considered` | `[]Candidate` | all candidates with their `Reject` reasons (verbose, FR-012) |

**Invariant**: `Chosen != nil ⟺ ExitCode == 0`; when `Chosen == nil` nothing is written to
stdout (FR-008, SC-007). In proxy mode the analogous outcome is the resolved `SSH_AUTH_SOCK`
handed to the `exec`'d `ssh-keygen`; a `nil` chosen ⇒ exit `1` before exec.

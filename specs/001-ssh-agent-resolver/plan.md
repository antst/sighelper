# Implementation Plan: SSH Agent Socket Resolver & Signing Helper

**Branch**: `001-ssh-agent-resolver` | **Date**: 2026-06-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-ssh-agent-resolver/spec.md`

## Summary

A tiny stateless Go binary that keeps git commit signing working across tmux/SSH reconnects.
It discovers candidate `ssh-agent` sockets (configurable glob `/tmp/ssh-*/agent.*` plus the
current `$SSH_AUTH_SOCK`), keeps only those owned by the real UID in a trusted directory,
confirms each is live via a non-mutating `ssh-agent` `List()` handshake under a short timeout,
and deterministically selects one (key-holder first, then newest mtime, then lexicographic
path). One executable serves two modes, dispatched on argv: a **resolver** that prints the live
socket path to stdout (`export SSH_AUTH_SOCK=$(sighelper)`), and a **signing proxy** invoked by
git as `gpg.ssh.program` that resolves a live agent, sets `SSH_AUTH_SOCK`, and `exec`s the real
`ssh-keygen -Y sign …` — reusing OpenSSH's audited SSHSIG implementation rather than
reimplementing it. Resolver is the v1 MVP (US1); the proxy is the second increment (US2).

## Technical Context

**Language/Version**: Go 1.26 (current stable; matches sibling `server-go` toolchain)

**Primary Dependencies**: standard library + `golang.org/x/crypto` (`ssh`, `ssh/agent`) — the
constitution-pre-approved single external dependency for the agent protocol. Pin the latest
stable from pkg.go.dev at implementation time (do not assume a version). External runtime
program: OpenSSH `ssh-keygen` (proxy mode only).

**Storage**: none — stateless; no files created, no persistence (constitution Principle I).

**Testing**: `go test ./... -race` (table-driven unit tests for ownership/trust/liveness/
selection with hostile inputs; an integration test against a real/temporary agent socket),
`go vet`, `golangci-lint` (`.golangci.yml`, gosec enabled).

**Target Platform**: POSIX/Unix — Linux primary, macOS/BSD via `--pattern`. Windows
(named-pipe ssh-agent) explicitly out of scope.

**Project Type**: single-binary CLI tool (no client/server split).

**Performance Goals**: socket resolution imperceptible in an interactive `git commit` — under
~1 s common case, bounded even when an agent hangs (SC-002); default per-probe timeout 250 ms,
candidates probed concurrently.

**Constraints**: never select a foreign-owned or dead socket (SC-003/SC-004); deterministic
selection (SC-005); never emit private key material or signatures (SC-006); fail loud (exit 1,
empty stdout) when nothing usable (SC-007); read-only probing — never mutate/create sockets.

**Scale/Scope**: a handful of candidate sockets per invocation; ~5 source files; resolver +
proxy modes.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.1.0. Must pass before Phase 0 and
re-checked after Phase 1.*

| Principle | Gate | Status |
|-----------|------|--------|
| I. Single-Purpose Simplicity | stdlib-first; only `x/crypto/ssh/agent` external (pre-approved, hand-rolling the protocol is demonstrably worse); no daemon/state; one config knob + flags | ✅ PASS |
| II. Security & Least Privilege (NON-NEGOTIABLE) | owner check via `Lstat`/`Stat_t.Uid` vs **real** UID; parent-dir not group/world-writable; symlinks resolved then re-checked; read-only `List()` probe; no secrets logged; proxy `exec`s audited `ssh-keygen` | ✅ PASS |
| III. Fail-Safe Resolution | liveness = real `List()` handshake, 250 ms bounded; deterministic selection (R6); exit 1 + empty stdout when none; refuse when required key absent | ✅ PASS |
| IV. Drop-In Compatibility & Stable Interface | both modes from one binary (resolver stdout / proxy `gpg.ssh.program` passthrough); clean stdout for `$( )`; configurable pattern; documented flag/exit-code contract | ✅ PASS |
| V. Observable & Testable | `--verbose` per-candidate accept/reject (no secrets); table-driven tests incl. foreign/dead/symlink/missing-dir/non-socket; one end-to-end agent integration test; exit codes tested | ✅ PASS |
| Engineering Discipline | DRY (single `internal/resolver` engine shared by both modes); No-Legacy; Meaningful-Tests; No-Invented-Metrics (250 ms/sub-second are UX-justified); Latest-Deps (pin from pkg.go.dev); No-Assumptions (git invocation web-verified); No-Busywork (no JSON/symlink modes) | ✅ PASS |

**Result**: no violations, no deviations. The single external dependency is explicitly
sanctioned by Principle I, not a deviation — Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-ssh-agent-resolver/
├── plan.md              # This file
├── spec.md              # Feature spec (+ Clarifications)
├── research.md          # Phase 0 decisions (R1–R10)
├── data-model.md        # Phase 1 entities & selection lifecycle
├── quickstart.md        # Phase 1 validation guide
├── contracts/
│   └── cli-contract.md  # Resolver CLI + proxy (gpg.ssh.program) contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (16/16)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
go.mod                        # module github.com/antst/sighelper, go 1.26
main.go                       # argv dispatch (resolver vs proxy), flag parsing, stdout/stderr, exit codes
internal/
├── resolver/                 # THE ENGINE (single source of truth, used by both modes)
│   ├── types.go              #   Config, Candidate, Identity, RejectReason, SelectionOutcome
│   ├── candidate.go          #   Discover(pattern, $SSH_AUTH_SOCK), dedupe by resolved path
│   ├── trust.go              #   EvalSymlinks + socket/owner/parent-dir checks (R3)
│   ├── liveness.go           #   concurrent List() probe under timeout (R1/R2)
│   ├── select.go             #   key-filter + (mtime desc, path asc) selection (R6)
│   ├── report.go             #   verbose per-candidate accept/reject, secret-safe (US3, FR-012)
│   └── *_test.go             #   hostile-input + no-mutation (FR-005) + e2e integration tests
├── signkey/                  # parse/match pubkeys (R4) + git-config key detection
│   ├── signkey.go            #   ParseKey, Match (shared by both modes)
│   ├── gitconfig.go          #   DetermineKey via git config (US1 auto-detect)
│   └── signkey_test.go
├── proxy/                    # ssh-keygen passthrough signing proxy (R5)
│   ├── proxy.go
│   └── proxy_test.go
└── env/                      # shared env-var config parsing (used by main + proxy, DRY)
    └── env.go
main_test.go                  # resolver-CLI contract test (stdout/exit/stderr, FR-007/008/015)
.golangci.yml                 # lint config (gosec on; documented exclusions for intentional exec/perm)
```

**Structure Decision**: Flat `internal/*` packages around a single shared resolver engine.
Both the resolver CLI and the proxy call `internal/resolver` — one implementation of
discovery/trust/liveness/selection (DRY). Deliberately **not** hexagonal/ports-and-adapters
(the sibling `server-go` pattern): that weight is unjustified for a stateless single binary
(Principle I, recorded so no reviewer mistakes the omission for an oversight).

## Complexity Tracking

> No constitution violations — no entries required.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |

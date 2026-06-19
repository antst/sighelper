<!--
SYNC IMPACT REPORT
==================
Version change: 1.1.0 → 1.2.0
Bump rationale: MINOR — added a mandatory ≥95% per-package statement-coverage quality
gate (enforced by scripts/check-coverage.sh in `make ci` and CI). The "Meaningful Tests
Only" discipline was reworded to reconcile the new floor with the existing "no
coverage-padding" rule (the floor must be earned by meaningful tests). No principle was
removed; no obligation was weakened.

Modified principles: none (the five core principles retained verbatim).
  I. Single-Purpose Simplicity
  II. Security & Least Privilege (NON-NEGOTIABLE)
  III. Fail-Safe Resolution
  IV. Drop-In Compatibility & Stable Interface
  V. Observable & Testable

Modified sections (1.2.0):
  - Development Workflow & Quality Gates — Testing gate now requires -race + ≥95% coverage
  - Engineering Discipline → Meaningful Tests Only — reconciled with the 95% floor

Added sections (1.1.0): Engineering Discipline.

Removed sections: none.

Templates requiring updates:
  ✅ .specify/templates/plan-template.md — generic "Constitution Check" gate, compatible as-is
  ✅ .specify/templates/spec-template.md — no constitution-specific references
  ✅ .specify/templates/tasks-template.md — no constitution-specific references
  ✅ CLAUDE.md — quality gates (coverage, Makefile, CI) kept in sync

Follow-up TODOs:
  - RATIFICATION_DATE set to 2026-06-19 (today / initial adoption). Adjust if an
    earlier original adoption date is preferred.
-->

# Sighelper Constitution

A helper for resolving a live SSH agent socket (and/or acting as a git SSH
signing proxy) so commit signing keeps working across reconnected tmux/SSH
sessions.

## Core Principles

### I. Single-Purpose Simplicity

The tool MUST do exactly one job well: locate a usable SSH agent socket for the
current user and, optionally, proxy SSH signing requests to it. Scope is bounded
to discovery, validation, selection, and (optional) signing pass-through.

- Written in Go; the standard library is the default. Third-party dependencies
  MUST be justified in the plan and rejected when stdlib suffices (`os`,
  `os/user`, `net`, `path/filepath`, `golang.org/x/crypto/ssh/agent` is the only
  presumptively-acceptable external dependency, and only if hand-rolling the
  agent protocol is demonstrably worse).
- No daemons, no persistent state, no config databases. Behavior is driven by
  flags/environment and a single optional config value (the socket glob pattern).
- YAGNI governs: features not required to keep signing alive are out of scope
  until a concrete need is documented.

Rationale: the failure being solved is small and operational; a small, auditable
binary is easier to trust with signing keys than a large one.

### II. Security & Least Privilege (NON-NEGOTIABLE)

The tool handles paths to SSH agent sockets capable of producing signatures.
Trust decisions MUST be conservative and ownership-based, never path-pattern
based alone.

- A candidate socket MUST be used ONLY if it is owned by the current real UID
  (verified via `stat`/`os.Lstat` + `Sys()`, not by directory naming).
- Sockets not owned by the current user MUST be ignored silently, regardless of
  matching the glob.
- The tool MUST NOT follow symlinks into directories it does not own, and MUST
  treat world-writable or group-writable socket directories as untrusted.
- The tool MUST NEVER print private key material, signatures, or full agent
  payloads to logs. Diagnostics reference sockets by path and identity only.
- The tool MUST NOT modify, delete, or create agent sockets; discovery is
  read-only probing.

Rationale: a malicious socket planted under a matching path could otherwise be
selected and asked to sign; ownership verification is the core safety property.

### III. Fail-Safe Resolution

Signing with the wrong or a dead agent is worse than failing loudly.

- Liveness MUST be confirmed by an actual, benign agent handshake (e.g.,
  list-identities / request-identities) under a bounded timeout, not by mere
  socket existence or `connect()` success.
- If multiple live sockets exist, selection MUST be deterministic and documented
  (e.g., the one whose agent holds the configured/required key, then most
  recently modified). Ambiguity MUST NOT be resolved by guessing silently.
- If no live, owned socket is found, the tool MUST exit non-zero with a clear
  message and MUST NOT emit a path that would cause git to hang or sign blindly.
- Default timeouts MUST be short enough to never block an interactive `git
  commit` noticeably (target: sub-second per probe).

Rationale: the tool sits in the critical path of `git commit`; its failure mode
must be an explicit, fast error, never a silent or hung signature.

### IV. Drop-In Compatibility & Stable Interface

The tool MUST integrate without forcing users to rewrite their git or shell
workflow.

- It MUST support the two intended integration modes without breaking either:
  (a) emit a chosen `SSH_AUTH_SOCK` value for shells/`git`/Claude Code to consume,
  and (b) act as a transparent SSH signing program compatible with git's
  `gpg.ssh.program` contract.
- Text I/O protocol: machine-usable result on stdout, diagnostics on stderr,
  meaningful exit codes. Output intended for `eval`/command substitution MUST be
  clean (no decoration) by default.
- The socket search pattern MUST be configurable (flag/env) with a sane default
  (`/tmp/ssh-*/agent.*`); changing defaults is a breaking change.
- Once published, flag names, output format, and exit-code meanings are a
  contract: breaking them requires a MAJOR version bump and migration notes.

Rationale: it must be invisible to existing tooling — git, tmux, and Claude Code
should not need to know it exists beyond a one-line configuration.

### V. Observable & Testable

The selection decision MUST be explainable and the logic MUST be tested.

- A verbose/debug mode MUST explain why each candidate was accepted or rejected
  (owned? live? holds key? chosen?) without leaking secrets (per Principle II).
- Liveness and ownership logic MUST be unit-tested with table-driven tests,
  including hostile inputs (foreign-owned socket, dead socket, symlink, missing
  directory, glob match that is not a socket).
- At least one integration test MUST exercise resolution against a real or mock
  agent socket end-to-end.
- Exit codes and stderr messages are part of the tested contract.

Rationale: an operator debugging a reconnected session must be able to see the
tool's reasoning, and signing logic is too sensitive to ship unverified.

## Security Requirements

- **Language/toolchain**: Go (current stable release). Build MUST be
  reproducible from `go build` with no codegen or network access at build time.
- **Dependency policy**: stdlib-first; each non-stdlib import is a reviewable
  decision recorded in the plan.
- **Ownership gate**: every socket path MUST pass `Lstat` ownership + permission
  checks (owner == real UID; parent dir not group/world-writable) before any
  bytes are written to it.
- **Probe safety**: liveness probes are limited to non-mutating agent requests
  with a hard timeout and connection close; the tool MUST NOT add, remove, or
  lock identities.
- **No secret exposure**: signatures and key blobs are never logged; verbose
  output is safe to paste into a bug report.
- **Privilege**: the tool MUST run as the unprivileged invoking user and MUST NOT
  require or request elevation.

## Development Workflow & Quality Gates

- **Constitution Check**: every plan (`/speckit-plan`) MUST pass the Constitution
  Check gate; any deviation is recorded in the plan's Complexity Tracking with a
  justification, or the design changes.
- **Testing gate**: ownership, liveness, and selection logic MUST have passing
  unit tests before merge; `go test ./... -race` and `go vet ./...` MUST be clean;
  statement coverage MUST be **≥ 95% per package** (`scripts/check-coverage.sh`),
  enforced locally via `make cover`/`make ci` and in CI on every supported platform.
  A package may carry a documented lower floor only for thin glue whose remaining
  uncovered lines are genuinely untestable (a no-regression ratchet, never lowered).
- **Review gate**: changes touching socket validation or signing pass-through
  require explicit review against Principle II before merge.
- **Simplicity gate**: new flags, dependencies, or modes MUST cite the concrete
  failure they address (Principle I); speculative features are rejected.
- **Docs gate**: integration instructions (git config + shell/tmux usage) MUST
  stay accurate; a behavior change that invalidates them blocks merge.

## Engineering Discipline

These cross-cutting working principles govern how we build day-to-day, complementing
the product principles above. Adapted from the org-standard Go reference
([`alkem-io/file-service`](https://github.com/alkem-io/file-service), via
`alkemio/server-go`). They are authoritative; `CLAUDE.md` operationalizes them.

- **Root Cause First (RCA)**: Never apply speculative fixes. Identify the root cause
  with evidence before touching code. If the cause is unclear, debug and instrument
  first — guessing wastes more time than investigating. This applies especially to
  socket/agent flakiness: prove *why* a probe failed (dead agent vs. wrong owner vs.
  timeout) before changing selection logic.
- **Single Source of Truth (DRY)**: No two functions implement the same logic in
  different packages. Ownership checks, liveness probing, and socket discovery each
  have exactly one canonical implementation. Constants (default glob, timeouts) and
  type definitions live in one place. Search for an existing implementation before
  writing new logic.
- **No Legacy Code**: Every line MUST justify its existence. Dead code, commented-out
  blocks, unused exports, and backward-compatibility shims are removed immediately.
  We control the full stack; there are no unknown consumers requiring silent
  compatibility. Breaking the published CLI contract still follows Principle IV.
- **Meaningful Tests Only**: Tests defend real invariants (ownership gate, liveness
  semantics, selection determinism) or catch real regressions. A **minimum of 95%
  statement coverage per package is a hard floor** (see Quality Gates), but coverage MUST be earned
  by meaningful tests — never by assertion-free padding, trivial pass-through tests, or
  scenarios that cannot fail. Where a statement is genuinely untestable (`os.Exit`, the
  `exec` that replaces the process, defensive syscall-assertion branches), isolate it so
  it is a negligible fraction rather than lowering the bar. Placeholder or "fix later"
  stubs are forbidden.
- **No Invented Metrics**: Success criteria MUST be directly testable in this tool.
  Do not invent performance SLAs without a baseline measurement or an explicit
  requirement. The "sub-second probe" target (Principle III) is a UX requirement of
  the interactive `git commit` path, not an invented number.
- **Latest Dependencies**: When adding or updating a dependency, verify the current
  stable version on pkg.go.dev or the project's releases — never rely on training-data
  version numbers. Each non-stdlib import remains a reviewable decision (Principle I).
- **No Assumptions**: Never assume requirements, behavior, or platform details not
  explicitly defined. If something is unclear, ask. If a fact is needed (a syscall's
  behavior, an agent-protocol detail, a Go API), verify it — read the source or docs.
- **No Busywork**: Every task, test, and artifact MUST deliver demonstrable value. No
  documentation, comments, or abstractions "just in case". Specs and code stay lean —
  only what is necessary to communicate intent and keep signing alive.

## Governance

This constitution supersedes ad-hoc practices for the project. It governs how the
SSH-socket helper is designed, built, and changed.

- **Amendments** MUST be proposed as a documented change to this file, including
  the rationale and the version bump justification, and MUST update any
  dependent templates flagged in the Sync Impact Report.
- **Versioning policy** (semantic):
  - MAJOR: removal/redefinition of a principle, or a breaking change to the
    tool's CLI/output/exit-code contract (Principle IV).
  - MINOR: a new principle or materially expanded guidance.
  - PATCH: clarifications and wording with no change in obligations.
- **Compliance review**: every PR/plan/review MUST verify compliance with these
  principles; the Security & Least Privilege principle is non-negotiable and may
  not be waived via Complexity Tracking.
- **Runtime guidance**: agent and contributor guidance lives in `CLAUDE.md` and
  the per-feature `specs/` artifacts; where they conflict with this constitution,
  the constitution wins.

**Version**: 1.2.0 | **Ratified**: 2026-06-19 | **Last Amended**: 2026-06-19

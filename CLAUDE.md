# CLAUDE.md — sighelper

Implements `.specify/memory/constitution.md`. Does not introduce new governance.

Read `.specify/memory/constitution.md` first — it is the authoritative source of
principles and rules. This file provides operational tooling guidance derived from it.

## Project Overview

`sighelper` resolves a usable SSH agent socket for the current user — and optionally
acts as a git SSH signing proxy — so commit signing keeps working after a tmux/SSH
session reconnects and the original `SSH_AUTH_SOCK` goes dead. It scans a configurable
glob (default `/tmp/ssh-*/agent.*`), keeps only sockets owned by the current real UID,
confirms each is live via a benign agent handshake under a short timeout, selects one
deterministically, and either prints its path or signs through it.

**Stack**: Go (current stable), standard library first. `golang.org/x/crypto/ssh/agent`
is the only presumptively-acceptable external dependency, and only if hand-rolling the
agent protocol is demonstrably worse (constitution Principle I). golangci-lint for lint.

**Module path**: `github.com/antst/sighelper`

## Coding Standards

### Error Handling
Explicit error returns everywhere; wrap with `%w` and context. `panic` is forbidden
outside `main`/startup and truly unrecoverable states.

```go
sock, err := probe(ctx, path)
if err != nil {
    return fmt.Errorf("probe %s: %w", path, err)
}
```

### Context Propagation
Every function that does I/O (dialing a socket, the agent handshake) MUST take
`context.Context` as its first parameter so probe timeouts and cancellation propagate.

### Diagnostics — never leak secrets
Diagnostics go to **stderr**; the machine-usable result (a socket path) goes to
**stdout**, clean by default so it is safe for `eval`/command substitution. Verbose mode
explains accept/reject per candidate (owned? live? holds key? chosen?). NEVER log private
key material, signatures, or full agent payloads — reference sockets by path and identity
only (constitution Principle II). stdlib `log`/`fmt` to stderr is sufficient; a structured
logger is justified only if complexity actually demands it (Principle I, No Busywork).

### Interfaces at the point of use
Define interfaces in the consuming package, not the implementing one (standard Go). Keep
the surface minimal — this is a small binary, not a framework.

## Toolchain

Use the `Makefile` (run `make help` for the list):

```bash
make build               # bin/sighelper (-trimpath -ldflags "-s -w")
make test                # go test -race with a coverage profile
make cover               # enforce >=95% per-package coverage (scripts/check-coverage.sh)
make lint                # golangci-lint (gosec enabled)
make vet                 # go vet ./...
make ci                  # vet + lint + cover — the full local gate (mirrors CI)
make dist GOOS=.. GOARCH=..       # one release binary into dist/
make dist-macos-universal         # fat amd64+arm64 macOS binary (run on macOS)
```

## Quality Gates

All MUST pass before any merge (`make ci` runs them; CI enforces on every supported platform):

- `golangci-lint run` — clean (the security linter `gosec` matters here especially)
- `go vet ./...` — clean
- `go build ./...` — compiles
- `go test ./... -race` — no failing tests; ownership, liveness, and selection logic
  covered with table-driven tests including hostile inputs (foreign-owned socket, dead
  socket, symlink, missing dir, glob match that is not a socket) — constitution Principle V
- **≥ 95% statement coverage per package** — `scripts/check-coverage.sh` (constitution v1.2.0).
  Genuinely-untestable lines (`os.Exit`, the `exec` that replaces the process, defensive
  syscall-type assertions) are isolated so they stay a negligible fraction.

## CI / Release (GitHub Actions)

- **`ci.yml`** — lint (golangci-lint, `go vet`, `gofmt -s` check) + a native test matrix
  across all supported platforms: `ubuntu-latest` (linux/amd64), `ubuntu-24.04-arm`
  (linux/arm64), `macos-13` (darwin/amd64), `macos-latest` (darwin/arm64). No QEMU. The
  coverage gate runs on `ubuntu-latest`.
- **`release.yml`** — on a `v*` tag: native builds of `linux/amd64`, `linux/arm64`, and a
  `macos/universal` (`lipo` of both darwin arches), attached to a GitHub Release with
  `SHA256SUMS`.

## Go Conventions (reference: alkem-io/file-service)

[`alkem-io/file-service`](https://github.com/alkem-io/file-service) is the org-standard
reference implementation for Go services; we adopt its conventions.

**Lint** (`.golangci.yml`, golangci-lint v2): errcheck, errorlint, staticcheck (all minus
ST1000), revive (no `exported`/`package-comments` doc-comment nagging — we don't add
obvious comments), gocritic, gosec, gocyclo (15), prealloc, etc. Zero exclusion presets,
all issues reported, goimports local-prefix `github.com/antst/sighelper`.

## Engineering Principles

The cross-cutting working principles — **Root Cause First (RCA)**, **Single Source of
Truth (DRY)**, **No Legacy Code**, **Meaningful Tests Only**, **No Invented Metrics**,
**Latest Dependencies**, **No Assumptions**, **No Busywork** — are defined authoritatively
in the constitution's **Engineering Discipline** section. They are not restated here (DRY);
read them there and apply them.

## What NOT to Do

- Do not apply speculative fixes — find the root cause of a flaky probe first
- Do not trust a socket by path/glob alone — verify owner == current real UID (Lstat)
- Do not write to, mutate, lock, or delete any agent socket — discovery is read-only
- Do not treat `connect()` success as liveness — require a real benign agent handshake
- Do not log key material, signatures, or agent payloads — verbose output must be
  paste-safe for a bug report
- Do not emit a socket path when no live, owned socket is found — exit non-zero instead
- Do not decorate stdout — the result must be clean for `eval`/command substitution
- Do not duplicate logic — one canonical implementation of ownership/liveness/discovery
- Do not keep dead code or backward-compat shims (No Legacy Code)
- Do not add tests for coverage padding or abstractions for hypothetical needs
- Do not invent performance SLAs without evidence
- Do not rely on AI training data for dependency versions — check pkg.go.dev
- Do not assume syscall/agent-protocol behavior — verify against source or docs

## Governance & Workflow

- **Constitution**: `.specify/memory/constitution.md` — authoritative principles. Read it
  before any non-trivial change. Where this file conflicts with it, the constitution wins.
- **SDD phases**: `/speckit-specify` → `/speckit-plan` → `/speckit-tasks` →
  `/speckit-implement`. Capture requirements in `specs/<NNN-slug>/spec.md` before coding.
- **PRs touching socket validation or signing pass-through** require explicit review
  against constitution Principle II (Security & Least Privilege, non-negotiable).
- **Governance changes**: PRs touching `constitution.md` follow its amendment + versioning
  procedure and note the version bump.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/001-ssh-agent-resolver/plan.md
<!-- SPECKIT END -->

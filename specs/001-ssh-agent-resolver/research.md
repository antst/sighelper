# Phase 0 Research: SSH Agent Socket Resolver & Signing Helper

**Feature**: 001-ssh-agent-resolver | **Date**: 2026-06-19

Resolves the planning-deferred items from clarification plus the technical unknowns. Each
decision is pinned with rationale and rejected alternatives, per the constitution's
Root-Cause-First and No-Assumptions principles. External contracts were verified, not recalled.

## R1 — Liveness probe mechanism

**Decision**: Dial the candidate as a Unix-domain socket with a bounded timeout, wrap the
connection with `golang.org/x/crypto/ssh/agent.NewClient`, and call `List()`
(SSH agent `REQUEST_IDENTITIES`, a read-only request) under a connection deadline. Success ⇒
the agent is live; the returned `[]*agent.Key` are the agent's public identities, reused
directly for key matching (R4). Any error (dial refused, deadline, protocol error) ⇒ rejected
as not live.

**Rationale**: `List()` is non-mutating (satisfies FR-004/FR-005), is one round trip, and
yields the identities needed for selection in the same call — no second probe. A bare
`connect()` is explicitly insufficient (a socket file can accept a connection with no
responding agent behind it).

**Alternatives rejected**: raw hand-rolled `REQUEST_IDENTITIES` framing (more code, security-
sensitive — the constitution pre-approves x/crypto/ssh/agent precisely to avoid this);
`connect()`-only liveness (violates FR-004); sending a `Sign` probe (mutating side effects /
could prompt confirmation — unacceptable).

## R2 — Probe timeout and concurrency

**Decision**: Probe all candidates **concurrently**, each bounded by a default **250 ms**
deadline (`--timeout`, env `SIGHELPER_TIMEOUT`). A live local `AF_UNIX` `List()` returns in
well under a millisecond; a dead socket whose listener is gone fails `connect()` immediately;
only a genuinely hung agent costs the full timeout. Concurrency bounds total wall-clock to
≈ the slowest single probe regardless of how many stale `/tmp/ssh-*` directories exist.

**Rationale**: Satisfies SC-002 (< ~1 s, bounded even when an agent hangs) independent of
candidate count. 250 ms is imperceptible interactively yet tolerant of a loaded host.

**Alternatives rejected**: sequential probing (total grows with candidate count — a host with
many orphaned session dirs could exceed 1 s if several hang); no timeout (violates FR-013).

## R3 — Ownership & trust checks

**Decision**: For each candidate path:
1. `filepath.EvalSymlinks` to resolve the real path (so a legitimately symlinked
   `$SSH_AUTH_SOCK` still works), then operate on the resolved path.
2. `os.Lstat` the resolved path; require it to be a socket (`mode&os.ModeSocket != 0`).
3. Owner check: `stat.Sys().(*syscall.Stat_t).Uid == uint32(os.Getuid())` — the **real** UID
   (constitution II), never inferred from the path.
4. Parent-directory trust: `os.Stat(parentDir)` — owner must equal the real UID and the
   directory MUST NOT be group- or world-writable (`perm&0o022 == 0`).

A candidate failing any check is dropped (silently in normal mode, explained under verbose —
see R8). The `$SSH_AUTH_SOCK` candidate is subject to the identical checks.

**Rationale**: Mirrors OpenSSH's own socket-directory hygiene; resolving-then-checking honors
"MUST NOT follow symlinks into directories it does not own" by validating the *resolved*
location rather than blindly rejecting all symlinks.

**Alternatives rejected**: reject every symlink outright (breaks the common
`~/.ssh/auth_sock → /tmp/...` pattern); trust by path prefix (the exact spoofing risk
Principle II forbids).

## R4 — Signing-key source and matching

**Decision**: Determine the relevant key, in priority order:
1. Explicit override: `--key <path-or-literal>` (resolver) / the `-f` argument (proxy, R5).
2. Else auto-detect from git: `git config --get gpg.format` (must be `ssh`) and
   `git config --get user.signingkey`.
3. Else none ⇒ key-agnostic selection (FR-010 fallback).

`user.signingkey` is either a filesystem path to a public-key file (read it) or a literal
public key (`ssh-ed25519 AAAA… [comment]`, detected by an `ssh-`/`sk-`/`ecdsa-` prefix). Parse
with `golang.org/x/crypto/ssh.ParseAuthorizedKey`/`ParsePublicKey` to wire bytes and match
against each agent identity's `Key.Blob` by exact byte equality.

**Rationale**: Shelling to `git config` is authoritative — it honors repo/global precedence and
`include`/`includeIf` directives that hand-parsing `~/.gitconfig` would miss. Matching on the
public-key wire blob is exact and needs no private material.

**Alternatives rejected**: parsing gitconfig files directly (loses include/precedence
semantics); matching by comment/fingerprint string (fragile vs. exact blob equality).

## R5 — Signing-proxy implementation (US2)

**Verified contract** (web-confirmed): with `gpg.format=ssh`, git signs by invoking
`gpg.ssh.program` as `ssh-keygen -Y sign -n git -f <key-file> <buffer-file>`, reading the
signature from `<buffer-file>.sig`. Verification invokes `-Y find-principals` / `-Y verify` /
`-Y check-novalidate`, none of which need an agent.

**Decision**: The single binary detects proxy invocations by the presence of the `-Y` operation
flag in argv. For `-Y sign`: extract the `-f` key as the required key (R4), resolve a live
owned agent that holds it (the R1–R3 engine), set `SSH_AUTH_SOCK` in the environment, then
`syscall.Exec` the **real** `ssh-keygen` with the identical argv. For any non-`sign` `-Y`
operation: pass through to the real `ssh-keygen` unchanged (no resolution needed). The real
`ssh-keygen` is located via `--ssh-keygen`/`SIGHELPER_SSH_KEYGEN`, else a `PATH` lookup that
excludes our own binary. Proxy-mode configuration (pattern, timeout) comes from environment
variables only, since git owns argv.

**Rationale**: `ssh-keygen -Y sign` already falls back to the agent (via `SSH_AUTH_SOCK`) when
the `-f` public key's private half lives only in an agent — which is exactly the premise.
Reusing OpenSSH's audited SSHSIG implementation is smaller and far safer than reimplementing
signature production (Principles I & II). `exec` (not fork+wait) keeps the proxy transparent:
git sees ssh-keygen's own exit status, stdout, and `.sig` output.

**Alternatives rejected**: reimplement SSHSIG with x/crypto/ssh (large, security-critical,
duplicates OpenSSH — rejected by No-Legacy/Simplicity); fork+capture+rewrite (needless plumbing
and a place for bugs vs. transparent `exec`).

## R6 — Deterministic selection

**Decision**: From the live, owned, trusted candidates — restricted to key-holders when a key
is determined (R4) — sort by `(mtime DESC, resolvedPath ASC)` and take the first. `mtime` is the
resolved socket's `ModTime()`.

**Rationale**: Implements FR-006 exactly (clarified: newest mtime, then lexicographically first
path). A socket's mtime ≈ its bind time, so "newest" ≈ the freshest session; the path tiebreak
guarantees a total order (SC-005) even on mtime collision.

## R7 — Output, streams, exit codes

**Decision**: Resolver prints the bare resolved socket path + `\n` to **stdout** and exits `0`.
All diagnostics (including verbose) go to **stderr**. Exit-code contract (FR-008): `0` resolved
/ signature produced; `1` no usable socket; `2` usage or configuration error. No output
decoration (safe for `$( )`). No JSON output in v1.

**Rationale**: Implements FR-007/FR-008/FR-015 and the clarified exit-code contract. JSON is
omitted as speculative (No-Busywork); a future `--json` can be added without breaking the bare-
path default.

## R8 — Untrusted / rejected candidate reporting

**Decision**: Rejected candidates (foreign-owned, untrusted dir, dead, non-socket, wrong key)
are dropped **silently** in normal mode and listed with their reject reason only under
`-v/--verbose` (to stderr). Verbose output carries paths, fingerprints, and comments only —
never key blobs or signatures (FR-011).

**Rationale**: Resolves the deferred "silent vs warned" item. Silent-by-default keeps stdout/
stderr clean for scripting; verbose serves US3 diagnostics. Not surfacing foreign sockets by
default also avoids leaking other users' session presence.

## R9 — CLI surface & configuration

**Decision**: Single binary, dual dispatch on argv:
- **Resolver** (no `-Y` in argv): flags `--pattern` (env `SIGHELPER_PATTERN`, default
  `/tmp/ssh-*/agent.*`), `--timeout` (env `SIGHELPER_TIMEOUT`, default `250ms`), `--key`,
  `-v/--verbose`. Prints the socket path.
- **Proxy** (`-Y` present): config from env only; behavior per R5.

**Rationale**: One binary serves both integration modes (Principle IV) with zero ambiguity —
git always invokes it with `-Y`, humans/scripts never do. Env-var config lets the proxy be
tuned even though git controls its argv.

## R10 — Project layout, language, dependencies

**Decision**: Single Go module `github.com/antst/sighelper`, Go 1.26 (matches the
sibling `server-go` toolchain; "current stable" per constitution). Layout:

```
go.mod
main.go                       argv dispatch (resolver vs proxy), flag parsing, output, exit codes
internal/resolver/            Candidate, Discover, trust checks, liveness probe, Select  (the engine)
internal/signkey/             determine key (git config / override), parse + match public keys
internal/proxy/               ssh-keygen passthrough signing proxy (R5)
```

`*_test.go` live beside their packages. Sole external dependency:
`golang.org/x/crypto` (`ssh`, `ssh/agent`) — constitution-pre-approved; pin the **latest**
stable from pkg.go.dev at implementation time (Latest-Dependencies principle), do not assume a
version here.

**Rationale**: The engine (`internal/resolver`) is the single source of truth shared by both
modes (DRY). The flat `internal/*` split keeps the binary tiny — no hexagonal layering, which
would be over-architecture for a stateless CLI (Principle I). `go vet` + `golangci-lint`
(`.golangci.yml`, gosec on) gate quality.

**Alternatives rejected**: hexagonal/ports-and-adapters layout (the sibling server's pattern —
unjustified weight for a single-binary stateless tool); vendoring a copy of the agent protocol
(reinvents x/crypto).

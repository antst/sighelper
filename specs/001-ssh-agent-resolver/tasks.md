---

description: "Task list for SSH Agent Socket Resolver & Signing Helper"
---

# Tasks: SSH Agent Socket Resolver & Signing Helper

**Input**: Design documents from `specs/001-ssh-agent-resolver/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-contract.md

**Tests**: INCLUDED — the constitution (Principle V) and spec Success Criteria require
table-driven hostile-input tests plus an end-to-end agent integration test.

**Organization**: grouped by user story. The shared resolution **engine** lives in
Foundational (Phase 2) because the spec states every mode is built on top of it — this lets
US1, US2, and US3 proceed as independent increments afterward.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no incomplete dependencies)
- **[Story]**: US1 / US2 / US3 (story phases only)
- Paths are repository-root-relative; module `github.com/antst/sighelper`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization

- [X] T001 Initialize Go module `github.com/antst/sighelper` targeting go 1.26 in `go.mod`
- [X] T002 Add the `golang.org/x/crypto` dependency at the latest stable version (verify on pkg.go.dev — do NOT assume a version) in `go.mod` / `go.sum`
- [X] T003 [P] Create package skeleton (`internal/resolver/`, `internal/signkey/`, `internal/proxy/`, root `main.go` stub) and confirm `go vet ./...` and `golangci-lint run` pass clean on the skeleton

---

## Phase 2: Foundational (Resolution Engine — Blocking Prerequisites)

**Purpose**: The shared discover→trust→probe→select engine and key primitives that ALL three
stories build on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] Define shared value types (`Config`, `Candidate`, `Identity`, `RejectReason`, `SelectionOutcome`) per data-model.md in `internal/resolver/types.go`
- [X] T005 [P] Implement public-key parse + match (`ParseKey(pathOrLiteral) ([]byte,error)`, `Match(required []byte, ids []Identity) bool`) per research R4 in `internal/signkey/signkey.go`
- [X] T006 Implement discovery (`Discover(cfg)`: glob expansion + `$SSH_AUTH_SOCK` candidate, dedupe by resolved path — FR-001) in `internal/resolver/candidate.go` (depends T004)
- [X] T007 Implement trust checks (`EvalSymlinks` → socket-type + real-UID owner + parent-dir not group/world-writable, sets `RejectReason` — FR-002/FR-003, R3) in `internal/resolver/trust.go` (depends T004)
- [X] T008 Implement liveness probe (concurrent `agent.NewClient(unixDial).List()` under `Config.Timeout`, populate `Live` + `Identities` — FR-004, R1/R2) in `internal/resolver/liveness.go` (depends T004)
- [X] T009 Implement selection (key-filter via `signkey.Match` → sort `(mtime DESC, path ASC)` → choose, build `SelectionOutcome` — FR-006/FR-010, R6) in `internal/resolver/select.go` (depends T004, T005, T006, T007, T008)
- [X] T010 Engine unit tests (table-driven hostile inputs: foreign-owned/SC-003, dead+hung/SC-004, symlink, missing dir, non-socket match, multi-live determinism/SC-005; **plus a read-only/no-mutation invariant test (FR-005): probing leaves each socket's inode + mtime unchanged and creates/removes no files**) in `internal/resolver/*_test.go` and parse/match tests in `internal/signkey/signkey_test.go` (depends T005–T009)
- [X] T011 Implement argv-dispatch + config skeleton in `main.go` (`-Y` present → proxy stub; else resolver stub; parse `Config` from flags/env; exit-code 0/1/2 plumbing — FR-008, R9) (depends T004)

**Checkpoint**: Engine resolves and selects sockets; US1/US2/US3 can now proceed independently (and in parallel, except where they touch `main.go`).

---

## Phase 3: User Story 1 - Recover a usable agent socket on demand (Priority: P1) 🎯 MVP

**Goal**: `sighelper` prints a live, owned socket path so `export SSH_AUTH_SOCK=$(sighelper)` (or a per-command prefix) restores commit signing after a reconnect.

**Independent Test**: with one live and one stale owned agent present, `sighelper` prints the live socket and exits 0; kill it and it exits 1 with empty stdout (quickstart Scenario 1).

- [X] T012 [US1] Implement git-config signing-key auto-detection (`DetermineKey(cfg)` via `git config --get gpg.format` + `--get user.signingkey`, fallback to none — FR-010, R4) in `internal/signkey/gitconfig.go` (depends T005)
- [X] T013 [US1] Wire resolver mode in `main.go`: run engine with `DetermineKey`, print chosen resolved path to stdout, map outcome to exit 0/1/2, write nothing to stdout on failure (FR-007/FR-008/FR-015) (depends T009, T011, T012)
- [X] T014 [US1] Resolver end-to-end integration test (start a real temp agent + a dead socket; assert live socket printed, dead→exit 1 with a clear non-empty **stderr** message and empty stdout (FR-015, constitution V "stderr is tested contract"), foreign→never chosen) in `internal/resolver/integration_test.go` covering quickstart Scenarios 1–3, SC-001/003/004/007 (depends T013)

**Checkpoint**: MVP complete — the resolver independently restores signing.

---

## Phase 4: User Story 2 - Transparent git commit signing (Priority: P2)

**Goal**: configured as git's `gpg.ssh.program`, the helper resolves a live agent and signs transparently across reconnects with no env juggling.

**Independent Test**: configure `gpg.ssh.program`, point `SSH_AUTH_SOCK` at a dead socket, commit `-S` without fixing env → commit is signed by a live agent (quickstart Scenario 4, SC-008).

- [X] T015 [US2] Implement signing proxy (`Run(argv, env)`: classify `-Y` op; for `sign` extract `-f` key→required key, run engine, set `SSH_AUTH_SOCK`, `syscall.Exec` real `ssh-keygen`; non-sign → passthrough exec; locate ssh-keygen via `SIGHELPER_SSH_KEYGEN`/PATH excluding self — FR-009, R5) in `internal/proxy/proxy.go` (depends T009, T005)
- [X] T016 [US2] Wire proxy dispatch in `main.go` (`-Y` present → `proxy.Run`; pre-exec resolution failure → exit 1 clean, no wrong-key signing — FR-010) (depends T011, T015)
- [X] T017 [US2] Proxy tests (argv classification sign vs verify/find-principals; `-f` extraction; ssh-keygen path excludes self; pre-exec fail→exit 1 with a clear non-empty **stderr** message and no exec, constitution V) + signing integration test for quickstart Scenario 4 / SC-008 in `internal/proxy/proxy_test.go` (depends T016)

**Checkpoint**: US1 and US2 both work independently.

---

## Phase 5: User Story 3 - Explain the selection decision (Priority: P3)

**Goal**: a verbose mode that reports, per candidate, accept/reject reasons — secret-safe.

**Independent Test**: with foreign/dead/live sockets present, `sighelper --verbose` lists each with an accurate reason and leaks no secret material (quickstart Scenario 5, SC-006).

- [X] T018 [US3] Implement verbose reporting (`Config.Verbose` → per-candidate path + outcome + reject reason to stderr; fingerprints/comments only, never blobs/signatures — FR-011/FR-012) in `internal/resolver/report.go` (depends T009)
- [X] T019 [US3] Wire `-v/--verbose` flag (and `SIGHELPER_VERBOSE` for the proxy path) in `main.go` to enable reporting in both modes (depends T011, T018)
- [X] T020 [US3] Verbose tests (each candidate listed with correct reason; assert output matches no secret patterns like `PRIVATE KEY`/signature) in `internal/resolver/report_test.go` covering quickstart Scenario 5 / SC-006 (depends T019)

**Checkpoint**: all three stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: quality gates and end-to-end acceptance

- [X] T021 [P] Run `golangci-lint run` (gosec enabled) + `go vet ./...`; resolve all findings to zero (constitution quality gate)
- [X] T022 Run full `go test ./... -race`; ensure green
- [X] T023 Execute every quickstart.md scenario (1–5) end-to-end; confirm SC-001–SC-008 acceptance
- [X] T024 [P] Confirm SC-002 timing bound with a deliberately hung-agent candidate and SC-005 determinism (repeated runs identical)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (P1)**: no dependencies — start immediately
- **Foundational (P2)**: depends on Setup — **BLOCKS all user stories**
- **User Stories (P3–P5)**: each depends only on Foundational, not on each other — independently testable
- **Polish (P6)**: depends on the desired stories being complete

### User Story Dependencies

- **US1 (P1)**: engine (P2) → git-key detect → resolver wiring. No dependency on US2/US3.
- **US2 (P2)**: engine (P2) + signkey (P2) → proxy. No dependency on US1/US3.
- **US3 (P3)**: engine (P2) → verbose reporting. No dependency on US1/US2.

### Shared-file note (prevents conflicts)

`main.go` is touched by T011 (skeleton), T013 (US1), T016 (US2), T019 (US3). These four are
**NOT** parallel with each other — serialize the `main.go` wiring even though the underlying
engine/proxy/report packages were built in parallel.

### Within Each User Story

- Tests defend real invariants; write engine tests (T010) before/with wiring
- Engine packages before `main.go` wiring before integration tests

### Parallel Opportunities

- Setup: T003 ∥ (after T001/T002)
- Foundational: T004 ∥ T005 first; then T006/T007/T008 ∥ each other (different files, all depend on T004); T009 after them; T010 after T009; T011 ∥ early (only needs T004)
- After Phase 2: the package work for US1 (T012), US2 (T015), US3 (T018) can proceed in parallel by different developers — only the `main.go` wiring serializes

---

## Parallel Example: Foundational engine

```bash
# After T004 lands, build the three independent engine facets together:
Task: "Implement trust checks in internal/resolver/trust.go"        # T007
Task: "Implement liveness probe in internal/resolver/liveness.go"   # T008
Task: "Implement discovery in internal/resolver/candidate.go"       # T006
# T005 (internal/signkey/signkey.go) can run alongside all of the above.
```

---

## Implementation Strategy

### MVP First (through User Story 1)

1. Phase 1 Setup → 2. Phase 2 Foundational (engine) → 3. Phase 3 US1
4. **STOP and VALIDATE**: `export SSH_AUTH_SOCK=$(sighelper)` restores signing (quickstart 1–3)
5. Ship — the resolver alone solves the reported pain.

### Incremental Delivery

1. Setup + Foundational → engine ready
2. US1 (resolver) → validate → ship (MVP)
3. US2 (proxy) → validate → ship (`gpg.ssh.program`, zero-env signing)
4. US3 (verbose) → validate → ship (diagnostics)

---

## Notes

- [P] = different files, no incomplete dependencies; the `main.go` wiring tasks are deliberately not [P]
- Pin `golang.org/x/crypto` to the latest stable from pkg.go.dev at T002 — never from memory
- Proxy `exec`s the real `ssh-keygen` (audited SSHSIG) — do NOT reimplement signature crypto
- No secrets on any stream, including verbose (FR-011/SC-006) — assert this in T020
- FR-014 (unprivileged): satisfied by construction — the binary performs no privileged
  syscalls, requests no elevation, and is not setuid; no dedicated task, verified implicitly
  by running the tool as the normal user throughout T014/T017/T023
- Commit after each task or logical group; stop at any checkpoint to validate a story

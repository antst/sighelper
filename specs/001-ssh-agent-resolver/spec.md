# Feature Specification: SSH Agent Socket Resolver & Signing Helper

**Feature Branch**: `001-ssh-agent-resolver`

**Created**: 2026-06-19

**Status**: Draft

**Input**: User description: "with claude code+tmux+ssh we regularly hit the problem when git can not sign commits, as current SSH AUTH sock with which claude was started is dead (as ssh/tmux session is reconnected). Shall we write tiny helper, which will grab list of /tmp/ssh-*/agent.* (or some other configurable pattern), filter sockets owned by current user and evaluate which of them are not dead, so claude will know which one to use. Alternatively it might be signing proxy/agent for git signing with ssh key (so git call this proxy and proxy find alive socket and used this alive socket for signing). it will be written in GO."

## Clarifications

### Session 2026-06-19

- Q: Release scope — build the resolver mode, the signing-proxy mode, or both? → A: Both — resolver mode is the v1 MVP, signing-proxy ships as the second increment.
- Q: How does resolver mode deliver the live socket to the caller? → A: Print the path to standard output only — stateless; no managed symlink or on-disk pointer. Callers use command substitution / per-command prefix.
- Q: In resolver mode, how is the relevant signing key determined? → A: Default to the user's git-configured SSH signing key (auto-detected), overridable by an explicit option; when no key can be determined, fall back to key-agnostic selection among live owned agents.
- Q: Tie-breaker when multiple live owned agents hold the signing key? → A: Most recently modified socket (newest mtime), then lexicographically first socket path as the final tie-break.
- Q: What is the candidate search scope? → A: The configurable glob (default `/tmp/ssh-*/agent.*`) plus the current `$SSH_AUTH_SOCK` as an additional candidate (deduplicated); standard runtime locations are not searched by default.
- Q: What exit-code scheme should the CLI contract define? → A: `0` = resolved; `1` = no usable socket found (none owned/live/key-holding); `2` = usage or configuration error.
- Q: What is the supported-platform scope for v1? → A: POSIX/Unix (Linux primary; macOS/BSD via the same `AF_UNIX` filesystem-socket model and configurable pattern); Windows (named-pipe ssh-agent) is out of scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Recover a usable agent socket on demand (Priority: P1)

A developer (or an automated tool such as Claude Code) is working inside a long-lived
tmux session over SSH. The SSH connection drops and reconnects, which leaves behind a new,
live SSH agent socket while the socket the session was originally started with is now dead.
The next `git commit` fails to sign because the recorded agent socket no longer answers.

The user runs the helper. It scans the known locations for agent sockets, keeps only the
ones that belong to them and are actually responding, picks one, and prints its location.
The user (or tool) points the agent-socket environment variable at that value and signing
works again — without manually hunting through `/tmp` or restarting the session.

**Why this priority**: This is the core capability and the minimum that solves the reported
pain. Every other mode is built on top of this resolution step. Delivered alone it already
unblocks signing via a single command (e.g. used in command substitution to set the
agent-socket environment variable).

**Independent Test**: With one live owned agent and one stale owned agent present, run the
helper and confirm it prints the live one's location and exits success; kill the live agent
and confirm it exits with a non-zero status and prints nothing usable.

**Acceptance Scenarios**:

1. **Given** the recorded agent socket is dead but another live agent owned by the user
   exists, **When** the user runs the helper, **Then** it prints the live socket's location
   on standard output and exits with success.
2. **Given** the printed value is used to set the agent-socket environment variable,
   **When** the user runs `git commit` with signing enabled, **Then** the commit is signed
   successfully.
3. **Given** no live, owned agent socket exists anywhere in the searched locations,
   **When** the user runs the helper, **Then** it exits with a non-zero status, writes a
   clear explanation to the error stream, and writes nothing to standard output.
4. **Given** a socket owned by a different user matches the search pattern, **When** the
   helper runs, **Then** that socket is never selected or printed.

---

### User Story 2 - Transparent git commit signing (Priority: P2)

A user wants commit signing to "just work" across reconnects without ever touching an
environment variable again. They configure git once to call this helper as its signing
program. From then on, each signing operation triggered by git transparently resolves a
live, owned agent that holds the signing key and produces the signature through it. The user
never sees the broken-socket failure again and never re-exports anything.

**Why this priority**: This is the most convenient, "set once and forget" integration and
the second mode the user described. It depends entirely on the P1 resolution engine, so it
is sequenced after it. It is independently valuable: it removes the manual step that P1 still
requires.

**Independent Test**: Configure git to use the helper as its signing program, dead-end the
recorded agent socket, and run a signed commit; confirm the commit is signed using a live
agent and that no environment variable was changed by the user to make it work.

**Acceptance Scenarios**:

1. **Given** git is configured to use the helper for signing and the originally recorded
   agent socket is dead, **When** the user makes a signed commit, **Then** the commit is
   signed using a live, owned agent and succeeds.
2. **Given** a live owned agent that holds the configured signing key, **When** git requests
   a signature, **Then** the produced signature verifies against that key.
3. **Given** no live owned agent holds the configured signing key, **When** git requests a
   signature, **Then** the operation fails clearly and the commit is not signed with a wrong
   key or left silently unsigned.

---

### User Story 3 - Explain the selection decision (Priority: P3)

A user whose signing still fails wants to understand why. They run the helper in a verbose
mode that reports, for each candidate socket it considered, whether it was kept or rejected
and the reason (not owned by me / not responding / does not hold the key / chosen). The
output is safe to paste into a bug report — it never contains private keys or signature
contents.

**Why this priority**: Diagnostics make the tool trustworthy and supportable but are not
required to deliver the core value. They are a thin layer over the same resolution engine.

**Independent Test**: Present a mix of foreign-owned, dead, and live sockets, run the helper
in verbose mode, and confirm each candidate is listed with an accurate accept/reject reason
and that no secret material appears anywhere in the output.

**Acceptance Scenarios**:

1. **Given** several candidate sockets in different states, **When** the helper runs in
   verbose mode, **Then** it reports an accept/reject reason for each candidate and which one
   was chosen.
2. **Given** verbose mode is active, **When** the helper produces diagnostic output, **Then**
   no private key material or signature bytes appear in any output stream.

---

### Edge Cases

- **No matches**: the search pattern matches nothing → fail loudly (non-zero exit, clear
  message), output nothing usable.
- **Only foreign-owned matches**: every matching socket belongs to another user → all are
  ignored; treated as "no usable socket".
- **Existing but dead agent**: the socket file exists and may even accept a connection, but
  no agent responds → rejected as not live.
- **Multiple live owned agents**: more than one usable agent exists → selection is
  deterministic and repeatable for the same inputs (restrict to agents holding the determined
  signing key, then newest socket modification time, then lexicographically first path).
- **Match is not a socket**: the pattern matches a regular file or directory → ignored.
- **Untrusted location**: a matching socket lives in a directory writable by other users, or
  is reached through a symbolic link that leaves the user's own directory → treated as
  untrusted and not used.
- **Key required but absent**: a specific signing key is required but no live owned agent
  holds it → fail clearly rather than sign with a different key.
- **Slow or hung agent**: a candidate agent accepts the connection but never answers → it is
  abandoned within a short bounded time so an interactive commit is not blocked.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The helper MUST discover candidate agent sockets from a configurable search
  pattern (a sensible default location set, overridable via configuration) and MUST
  additionally include the socket named by the current `SSH_AUTH_SOCK` environment variable,
  when set, as a candidate — deduplicated against the pattern matches. The same trust,
  ownership, and liveness checks apply to this extra candidate. Standard runtime locations
  beyond the configured pattern are not searched by default.
- **FR-002**: The helper MUST keep only sockets owned by the current user, determining
  ownership from the operating system's reported owner of the socket, never inferring it from
  the socket's path or name.
- **FR-003**: The helper MUST reject candidate sockets that reside in directories writable by
  other users, or that are reached through a symbolic link leaving the current user's own
  directory tree, treating them as untrusted.
- **FR-004**: The helper MUST confirm that a candidate agent is actually responding using a
  non-destructive liveness check — mere existence of the socket file, or a successful
  connection, MUST NOT be treated as proof of liveness.
- **FR-005**: The helper MUST NOT modify, create, lock, or delete any agent socket or its
  contents; discovery and liveness checking are read-only with respect to other users' and
  the user's own agents.
- **FR-006**: When more than one live owned agent is available, the helper MUST select one
  deterministically: first restricting to agents that hold the determined signing key (see
  FR-010), then choosing the remaining socket with the most recent modification time, and
  breaking any remaining tie by the lexicographically first socket path. The same inputs MUST
  always yield the same choice.
- **FR-007**: In resolver mode, the helper MUST print only the chosen socket's resolved
  (canonical, symlinks-followed) path to standard output, free of decoration, so the value is
  directly usable in command substitution, and exit with success. The helper MUST NOT create
  or maintain any on-disk pointer (such as a stable symlink) to the resolved socket — delivery
  is stateless.
- **FR-008**: The helper MUST honor a defined exit-code contract: `0` when a socket is
  resolved (or, in signing-proxy mode, a signature is produced); `1` when no live, owned,
  trusted, key-holding socket is found; `2` for a usage or configuration error (e.g. invalid
  flags). On any non-zero exit it MUST write a clear explanation to the error stream and write
  nothing usable to standard output.
- **FR-009**: In signing-proxy mode, the helper MUST integrate with git's signing invocation
  and produce a valid signature using a resolved live agent, such that the user does not need
  to manage the agent-socket environment variable themselves.
- **FR-010**: The helper MUST determine the relevant signing key, defaulting to the user's
  git-configured SSH signing key (auto-detected) and overridable by an explicit option. When
  a signing key is determined: in resolver mode the helper MUST select an agent that holds it
  and MUST fail (exit non-zero, emit nothing usable) rather than return a socket whose agent
  cannot sign with it; in signing-proxy mode the helper MUST refuse to produce a signature
  with any other key. When no signing key can be determined, the helper MUST NOT filter by
  key and selects among live owned agents by the tie-breaker alone.
- **FR-011**: The helper MUST never emit private key material or signature contents to any
  diagnostic or log output; candidates are referred to by location and public identity only.
- **FR-012**: The helper MUST offer a verbose mode that reports, per candidate, whether it
  was accepted or rejected and the reason, plus which candidate was chosen.
- **FR-013**: Each liveness check MUST be bounded by a short, configurable time limit so that
  a slow or hung agent cannot block an interactive commit; the default time limit is 250
  milliseconds — short enough to be unnoticeable in normal interactive use.
- **FR-014**: The helper MUST run as the unprivileged invoking user and MUST NOT require or
  request elevated privileges.
- **FR-015**: Communication MUST follow the stream contract: the machine-usable result on
  standard output (content per FR-007), all human-facing diagnostics on the error stream, and
  the exit status per FR-008. Diagnostic output MUST NOT contaminate standard output, on any
  path including failures.

### Key Entities *(include if feature involves data)*

- **Candidate socket**: A potential agent endpoint discovered by the search. Attributes that
  matter to selection: its location, its operating-system owner, the trust level of its
  containing directory, whether it is currently responding, the public identities its agent
  holds, and a recency indicator for tie-breaking. Private key material is never an attribute
  the helper reads or stores.
- **Agent identity**: The public identity (e.g. a key fingerprint or comment) that a live
  agent reports holding. Used to decide whether an agent can satisfy a required signing key.
  Only public information is ever handled.
- **Selection outcome**: The chosen socket together with the reason it was chosen and, for
  diagnostics, the per-candidate accept/reject reasons. Contains no secret material.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a tmux/SSH reconnect that orphaned the original agent socket, the user
  restores working commit signing with a single command and no manual searching through
  temporary directories.
- **SC-002**: Resolving a socket completes fast enough not to noticeably delay an interactive
  commit — under roughly one second in the common case, and bounded even when a candidate
  agent hangs.
- **SC-003**: A socket owned by a different user is never selected or emitted, in 100% of
  cases, verified by test.
- **SC-004**: A dead or unresponsive socket is never returned as the live result, in 100% of
  cases, verified by test.
- **SC-005**: Given identical inputs, the helper selects the same socket every time
  (deterministic), reproducible in 100% of repeated runs.
- **SC-006**: No private key material or signature contents ever appear in any output stream,
  including verbose mode, in 100% of cases.
- **SC-007**: When no usable socket exists, the helper fails loudly — non-zero exit and a
  clear message — rather than emitting an empty or unusable value, in 100% of cases.
- **SC-008**: In signing-proxy mode, a signed commit succeeds across an SSH reconnect with no
  user-performed environment changes between the reconnect and the commit.

## Assumptions

- **Primary deliverable ordering** (confirmed 2026-06-19): Both modes are in scope. The
  resolver mode (User Story 1) is the v1 MVP and the signing-proxy mode (User Story 2) ships
  as the second increment, because the resolver is the simpler slice that already unblocks
  signing and the proxy is built on the same resolution engine.
- **Single-user host context**: The tool serves the invoking user's own agents on the local
  machine. Resolving agents for other users, or across remote hosts, is out of scope.
- **Platform scope** (confirmed 2026-06-19): POSIX/Unix only. Linux is the primary target;
  macOS and BSD are supported through the same filesystem `AF_UNIX` socket model with a
  configurable search pattern. Windows, whose ssh-agent is a named pipe rather than a
  filesystem socket, is out of scope — the whole discovery/liveness/ownership model assumes
  filesystem sockets.
- **Agents hold the signing key**: The relevant SSH signing key is expected to be loaded in
  one of the user's agents (the common case for agent-forwarded or locally-added keys). The
  tool resolves an agent; it does not load keys into agents or start new agents.
- **Git signing configuration is the default key source**: The helper reads the user's git
  signing configuration to learn which key matters by default; an explicit override is
  available, and when no git signing key is configured the helper selects key-agnostically.
  This adds a dependency on the local git configuration being readable.
- **Default search location**: The conventional per-session agent socket locations under the
  system temporary directory (matching the user's described pattern) are the default search
  scope, augmented by the current `$SSH_AUTH_SOCK` when set; the pattern is configurable for
  non-standard setups. Other standard runtime locations are reachable only by overriding the
  pattern, not searched automatically.
- **Liveness via the agent protocol**: "Live" means the agent answers a harmless request, not
  merely that the socket file is present or connectable. The exact harmless request is an
  implementation choice deferred to planning.
- **Out of scope**: starting/stopping agents, adding/removing keys, persisting state across
  runs, maintaining a stable symlink or other on-disk pointer to the resolved socket,
  Windows named-pipe ssh-agents, GUI/prompting, and any privileged operation.

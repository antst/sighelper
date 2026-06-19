# CLI Contract: sighelper

**Feature**: 001-ssh-agent-resolver | **Date**: 2026-06-19

The binary exposes two interfaces from one executable, dispatched on argv: a **resolver** CLI
(human/script-facing) and a **signing proxy** (git-facing, `gpg.ssh.program`). Per constitution
Principle IV, flag names, output format, and exit-code meanings below are a **stable contract** —
breaking them requires a MAJOR version bump.

## Dispatch rule

- argv contains the `-Y` operation flag → **proxy mode** (git's `ssh-keygen` calling convention).
- otherwise → **resolver mode**.

---

## Resolver mode

### Invocation

```
sighelper [--pattern GLOB] [--timeout DUR] [--key PATH|LITERAL] [-v|--verbose]
```

### Inputs

| Flag | Env | Default | Meaning |
|------|-----|---------|---------|
| `--pattern` | `SIGHELPER_PATTERN` | `/tmp/ssh-*/agent.*` | discovery glob (FR-001) |
| `--timeout` | `SIGHELPER_TIMEOUT` | `250ms` | per-probe deadline, Go duration (FR-013) |
| `--key` | `SIGHELPER_KEY` | *(auto from git)* | override signing key; path or literal pubkey (R4) |
| `-v`, `--verbose` | — | off | print per-candidate accept/reject to stderr (FR-012) |

The current `$SSH_AUTH_SOCK`, when set, is always added as an extra candidate (FR-001),
deduplicated against glob matches by resolved path.

### Outputs

- **stdout**: on success, exactly the chosen socket's resolved absolute path followed by a
  single `\n`. Nothing else. Safe for `export SSH_AUTH_SOCK=$(sighelper)`. (FR-007, FR-015)
- **stderr**: human-readable diagnostics; in `--verbose`, one line per candidate with its
  outcome and reason. Never contains key blobs or signatures. (FR-011, FR-012)
- On any non-zero exit, stdout is empty. (FR-008, SC-007)

### Exit codes (contract)

| Code | Meaning |
|------|---------|
| `0` | a usable socket was resolved and printed |
| `1` | no live, owned, trusted, key-holding socket found |
| `2` | usage or configuration error (unknown flag, non-positive timeout, unparseable key) |

### Behavioral guarantees (acceptance-testable)

- A socket not owned by the real UID is never printed (FR-002, SC-003).
- A socket whose agent does not answer `List()` within `--timeout` is never printed (FR-004, SC-004).
- Given identical inputs, the same socket is chosen every run: most recent mtime, then
  lexicographically first path (FR-006, SC-005).
- When a signing key is determined and no live owned agent holds it → exit `1`, stdout empty
  (FR-010).
- No private key material or signature bytes appear on any stream (FR-011, SC-006).

### Examples

```
# common use — set the agent socket for the current shell
export SSH_AUTH_SOCK="$(sighelper)" || echo "no live agent" >&2

# per-command, no env mutation
SSH_AUTH_SOCK="$(sighelper)" git commit -S -m "msg"

# debug why signing fails
sighelper --verbose
#   stderr: /tmp/ssh-AAAA/agent.111  rejected: foreign owner (uid 1002)
#   stderr: /tmp/ssh-BBBB/agent.222  rejected: dead (no response in 250ms)
#   stderr: /tmp/ssh-CCCC/agent.333  CHOSEN  (holds SHA256:… , mtime newest)
#   stdout: /tmp/ssh-CCCC/agent.333
```

---

## Proxy mode (git `gpg.ssh.program`)

### Configured by the user once

```
git config --global gpg.format ssh
git config --global gpg.ssh.program /path/to/sighelper
git config --global commit.gpgsign true       # optional
# user.signingkey already set to the SSH signing key
```

### Invocations handled (from git, verified contract)

| git operation | argv git passes | sighelper action |
|---------------|-----------------|------------------|
| **sign** | `-Y sign -n git -f <key> <buffer>` (sig → `<buffer>.sig`) | extract `<key>` as required key → resolve live owned agent holding it → set `SSH_AUTH_SOCK` → `exec` real `ssh-keygen` with identical argv |
| verify / principals | `-Y verify …`, `-Y find-principals …`, `-Y check-novalidate …` | `exec` real `ssh-keygen` unchanged (no agent needed) |

### Configuration (environment only — git owns argv)

`SIGHELPER_PATTERN`, `SIGHELPER_TIMEOUT`, `SIGHELPER_VERBOSE` (truthy → per-candidate
accept/reject diagnostics on stderr, since git owns argv and `-v` cannot be passed),
`SIGHELPER_SSH_KEYGEN` (path to the real `ssh-keygen`; default: `PATH` lookup excluding
sighelper itself).

### Outputs / exit codes

Transparent: on `exec`, git observes the real `ssh-keygen`'s stdout/stderr/`.sig` output and
exit status. If resolution fails **before** exec (no live owned agent holds the key), sighelper
exits `1` with a clear stderr message and does **not** exec — the commit fails cleanly rather
than being signed with the wrong key or hanging (FR-009, FR-010, SC-008).

### Behavioral guarantees

- A signed commit succeeds across an SSH reconnect with no user env changes (SC-008).
- The signature is produced by the real `ssh-keygen` against the agent holding the configured
  key — byte-for-byte a normal SSH signature (no custom crypto).

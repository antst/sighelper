# Quickstart & Validation: sighelper

**Feature**: 001-ssh-agent-resolver | **Date**: 2026-06-19

Runnable validation that the feature works end-to-end. Maps each scenario to the spec's
acceptance criteria. Implementation lives in `tasks.md` / the implementation phase — this is a
run/validate guide, not code.

## Prerequisites

- Go 1.26+, `ssh-keygen` (OpenSSH 8.2+) on `PATH`.
- A POSIX/Unix host (Linux primary; macOS/BSD via `--pattern`). Windows is out of scope.
- An SSH signing key loaded in an `ssh-agent`.

## Build

```bash
cd /home/antst/sighelper
go build ./...            # produces the sighelper binary
go test ./... -race       # unit + integration tests
golangci-lint run         # lint gate (.golangci.yml)
```

## Scenario 1 — Resolver recovers a live socket (US1 / P1)

Validates FR-001..FR-008, SC-001, SC-003, SC-004, SC-007.

```bash
# Arrange: a live agent and a dead one, both owned by you
eval "$(ssh-agent -s)"; LIVE="$SSH_AUTH_SOCK"; ssh-add ~/.ssh/id_ed25519
DEAD=$(mktemp -u); ln -s /nonexistent "$DEAD" 2>/dev/null || true   # or a stale /tmp/ssh-* sock

# Act
RESOLVED="$(sighelper --pattern '/tmp/ssh-*/agent.*')"

# Assert
test -S "$RESOLVED" && echo "PASS: resolved a real socket → $RESOLVED"
SSH_AUTH_SOCK="$RESOLVED" ssh-add -l >/dev/null && echo "PASS: it is live"
```

Then prove the **no-usable-socket** path (SC-007) — and that failure goes to stderr, not
stdout (FR-015, constitution V):

```bash
kill "$SSH_AGENT_PID"          # kill the live agent
out=$(sighelper --pattern '/tmp/does-not-exist/*' 2>err.txt); code=$?
echo "exit=$code (expect 1)"; test -z "$out" && echo "PASS: stdout empty"
test -s err.txt && echo "PASS: clear message on stderr"
```

**Read-only invariant (FR-005)** — probing must not mutate or create sockets:

```bash
before=$(stat -c '%i %Y' "$LIVE")     # inode + mtime of a live socket
sighelper >/dev/null 2>&1
after=$(stat -c '%i %Y' "$LIVE")
[ "$before" = "$after" ] && echo "PASS: probe left the socket untouched"
```

## Scenario 2 — Foreign / untrusted sockets are never chosen (SC-003, FR-002/FR-003)

```bash
# A socket matching the glob but owned by another user must be ignored.
sighelper --verbose 2>&1 1>/dev/null | grep -i 'foreign\|untrusted'   # shown as rejected, never chosen
```

## Scenario 3 — Signing through the resolved socket (US1 acceptance #2)

```bash
SSH_AUTH_SOCK="$(sighelper)" git -C /tmp/somerepo commit -S --allow-empty -m "signed via sighelper"
git -C /tmp/somerepo log --show-signature -1   # expect a good signature
```

## Scenario 4 — Transparent git signing proxy (US2 / P2, SC-008)

Validates FR-009, FR-010, SC-008.

```bash
# Configure git to call sighelper as its signing program
git config --global gpg.format ssh
git config --global gpg.ssh.program "$(command -v sighelper)"
# user.signingkey already points at your SSH signing key

# Simulate a reconnect: point SSH_AUTH_SOCK at a now-dead socket, then commit WITHOUT fixing env
export SSH_AUTH_SOCK=/tmp/ssh-DEAD/agent.0
git -C /tmp/somerepo commit -S --allow-empty -m "survives reconnect"
git -C /tmp/somerepo log --show-signature -1   # PASS: signed via a live agent, env untouched
```

Negative (FR-010): with **no** live agent holding the configured key, the commit MUST fail
cleanly (exit 1, clear stderr) rather than be signed with another key or hang.

## Scenario 5 — Explainable, secret-safe diagnostics (US3 / P3, SC-006)

```bash
sighelper --verbose 2>verbose.txt 1>chosen.txt
grep -Eiq 'PRIVATE KEY|BEGIN OPENSSH|signature' verbose.txt && echo "FAIL: leaked secret" || echo "PASS: no secrets"
cat chosen.txt   # exactly one socket path
```

## Determinism check (SC-005)

```bash
a=$(sighelper); b=$(sighelper); [ "$a" = "$b" ] && echo "PASS: deterministic"
```

## Reference

- CLI flags, exit codes, proxy invocations → [contracts/cli-contract.md](contracts/cli-contract.md)
- Entities and selection lifecycle → [data-model.md](data-model.md)
- Technical decisions and rationale → [research.md](research.md)

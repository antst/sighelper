# sighelper

[![CI](https://github.com/antst/sighelper/actions/workflows/ci.yml/badge.svg)](https://github.com/antst/sighelper/actions/workflows/ci.yml)

Keep git **commit signing working across tmux/SSH reconnects.**

When you reconnect an SSH/tmux session, the `SSH_AUTH_SOCK` your shell (or Claude Code, or a
long-lived process) was started with goes dead, while a *new*, live `ssh-agent` socket appears
under `/tmp/ssh-*/`. `git commit -S` then fails because it's still pointed at the dead socket.

`sighelper` finds a **live agent socket that you own and that holds your signing key**, and
either prints it or signs through it. It's a single, stateless, dependency-light Go binary.

## How it works

1. **Discover** candidate sockets from a glob (default `/tmp/ssh-*/agent.*`) plus the current
   `$SSH_AUTH_SOCK`.
2. **Trust** — keep only sockets owned by your real UID, in a directory that isn't
   group/world-writable (symlinks resolved first). Foreign or untrusted sockets are ignored.
3. **Liveness** — confirm each agent actually answers a read-only `ssh-agent` list request,
   under a short timeout, probed concurrently. A socket that merely *exists* is not enough.
4. **Select** deterministically — prefer an agent holding the signing key, then newest socket
   mtime, then lexicographic path.

Discovery is strictly read-only: sockets are never written to, created, or deleted, and no
private key material or signatures ever appear in any output.

## Install

```bash
go install github.com/antst/sighelper@latest   # → $GOBIN/sighelper
# or from a clone:
make build && cp bin/sighelper ~/bin/
```

Requires Go 1.26+. Runtime: POSIX/Unix (Linux primary; macOS/BSD via `--pattern`).
Windows (named-pipe agent) is out of scope. Proxy mode also needs OpenSSH `ssh-keygen`.

## Usage

### Mode 1 — resolver (print a live socket)

```bash
export SSH_AUTH_SOCK="$(sighelper)"           # fix the current shell
SSH_AUTH_SOCK="$(sighelper)" git commit -S    # or per-command, no env mutation
```

`sighelper` prints the resolved socket path to stdout and exits `0`; if nothing usable is
found it writes a message to stderr and exits non-zero (stdout stays empty, safe for `$(...)`).

### Mode 2 — transparent git signing proxy

Configure git once; signing then survives reconnects with no env juggling:

```bash
git config --global gpg.format ssh
git config --global gpg.ssh.program "$(command -v sighelper)"
git config --global commit.gpgsign true        # optional
# user.signingkey already points at your SSH signing key
```

git invokes `sighelper -Y sign …`; sighelper resolves a live agent holding the key, sets
`SSH_AUTH_SOCK`, and execs the real `ssh-keygen` — reusing OpenSSH's own signing code.

### Testing the proxy by hand

```bash
# pipe mode: '-' makes ssh-keygen read stdin and write the signature to stdout
echo "hello" | sighelper -Y sign -n test -f ~/.ssh/id_ed25519.pub - > sig.txt
echo "hello" | ssh-keygen -Y check-novalidate -n test -s sig.txt   # → Good "test" signature

# file mode (how git calls it): writes <file>.sig
sighelper -Y sign -n git -f ~/.ssh/id_ed25519.pub payload
```

## Configuration

| Flag | Env | Default | Meaning |
|------|-----|---------|---------|
| `--pattern` | `SIGHELPER_PATTERN` | `/tmp/ssh-*/agent.*` | discovery glob |
| `--timeout` | `SIGHELPER_TIMEOUT` | `250ms` | per-probe deadline |
| `--key` | `SIGHELPER_KEY` | *(git `user.signingkey`)* | signing key override (path or literal pubkey) |
| `-v`, `--verbose` | `SIGHELPER_VERBOSE` | off | per-candidate accept/reject on stderr |
| — | `SIGHELPER_SSH_KEYGEN` | PATH lookup | real `ssh-keygen` (proxy mode) |

In proxy mode git owns argv, so configure it via the environment variables.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | a socket was resolved / a signature was produced |
| `1` | no live, owned, trusted, key-holding socket found |
| `2` | usage or configuration error |

## Development

```bash
make ci      # vet + lint (golangci-lint, gosec) + race tests + >=95% coverage gate
make test    # race tests with coverage
make cover   # enforce the >=95% coverage floor
make build   # bin/sighelper
make help    # list targets
```

Engineering principles, security rules, and the testing/coverage gates are governed by
[`.specify/memory/constitution.md`](.specify/memory/constitution.md); operational guidance is
in [`CLAUDE.md`](CLAUDE.md). The full specification lives under
[`specs/001-ssh-agent-resolver/`](specs/001-ssh-agent-resolver/).

## License

[MIT](LICENSE) © 2026 Anton Starikov

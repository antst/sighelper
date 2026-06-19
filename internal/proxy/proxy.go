// Package proxy implements the git signing-program (gpg.ssh.program) mode: it resolves a
// live agent that holds the signing key, sets SSH_AUTH_SOCK, and execs the real ssh-keygen
// with the same argv — reusing OpenSSH's audited SSHSIG implementation (research R5).
package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/antst/sighelper/internal/env"
	"github.com/antst/sighelper/internal/resolver"
	"github.com/antst/sighelper/internal/signkey"
)

const defaultPattern = "/tmp/ssh-*/agent.*"
const defaultTimeout = 250 * time.Millisecond

// Run handles a git ssh-signing invocation (argv as passed to gpg.ssh.program). For a
// "-Y sign" operation it resolves a live owned agent holding the -f key, sets
// SSH_AUTH_SOCK, then execs ssh-keygen; other operations pass straight through. On success
// it execs and never returns; it returns an exit code only on pre-exec failure.
func Run(args []string) int {
	keygen, err := realSSHKeygen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sighelper: %v\n", err)
		return 2
	}
	if op, keyFile := parseY(args); op == "sign" {
		if code := prepareSign(keyFile); code != 0 {
			return code
		}
	}
	return execKeygen(keygen, args)
}

// execKeygen replaces the current process with the real ssh-keygen, carrying the (possibly
// updated) SSH_AUTH_SOCK. On success it never returns; it returns 2 only if exec itself fails.
func execKeygen(keygen string, args []string) int {
	argv := append([]string{keygen}, args...)
	err := syscall.Exec(keygen, argv, os.Environ()) // G204 suppressed in .golangci.yml: execing ssh-keygen is the proxy's job
	fmt.Fprintf(os.Stderr, "sighelper: exec %s: %v\n", keygen, err)
	return 2
}

// prepareSign resolves a live agent holding the signing key and sets SSH_AUTH_SOCK.
// Returns 0 to proceed, or a non-zero exit code to fail before exec (FR-010).
func prepareSign(keyFile string) int {
	var requiredKey []byte
	if keyFile != "" {
		if k, err := signkey.ParseKey(keyFile); err == nil {
			requiredKey = k
		}
	}
	cfg := resolver.Config{
		Pattern:     env.Or("SIGHELPER_PATTERN", defaultPattern),
		Timeout:     env.Duration("SIGHELPER_TIMEOUT", defaultTimeout),
		Verbose:     env.Bool("SIGHELPER_VERBOSE"),
		RequiredKey: requiredKey,
	}
	out := resolver.Resolve(cfg)
	if cfg.Verbose {
		resolver.Report(os.Stderr, out)
	}
	if out.Chosen == nil {
		fmt.Fprintln(os.Stderr, "sighelper: no live ssh-agent holds the signing key; refusing to sign")
		return 1
	}
	_ = os.Setenv("SSH_AUTH_SOCK", out.Chosen.Path)
	return 0
}

// parseY extracts the -Y operation and the -f key argument from a git signing invocation.
func parseY(args []string) (op, keyFile string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-Y":
			if i+1 < len(args) {
				op = args[i+1]
			}
		case "-f":
			if i+1 < len(args) {
				keyFile = args[i+1]
			}
		}
	}
	return op, keyFile
}

// realSSHKeygen locates the genuine ssh-keygen: an explicit override, else a PATH lookup
// that skips our own binary (research R5).
func realSSHKeygen() (string, error) {
	if p := os.Getenv("SIGHELPER_SSH_KEYGEN"); p != "" {
		return p, nil
	}
	self, _ := os.Executable()
	if rp, err := filepath.EvalSymlinks(self); err == nil {
		self = rp
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, "ssh-keygen")
		fi, err := os.Stat(cand)
		if err != nil || fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
			continue
		}
		if rp, err := filepath.EvalSymlinks(cand); err == nil && rp == self {
			continue
		}
		return cand, nil
	}
	if p, err := exec.LookPath("ssh-keygen"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("real ssh-keygen not found in PATH")
}

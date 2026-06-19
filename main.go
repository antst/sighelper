// Command sighelper resolves a live, owned ssh-agent socket so git commit signing keeps
// working across tmux/SSH reconnects. With no "-Y" in argv it runs in resolver mode and
// prints a usable socket path; when git invokes it as gpg.ssh.program ("-Y …") it runs as
// a transparent signing proxy.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/antst/sighelper/internal/env"
	"github.com/antst/sighelper/internal/proxy"
	"github.com/antst/sighelper/internal/resolver"
	"github.com/antst/sighelper/internal/signkey"
)

const (
	defaultPattern = "/tmp/ssh-*/agent.*"
	defaultTimeout = 250 * time.Millisecond
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if hasYFlag(args) {
		return proxy.Run(args) // proxy execs the real ssh-keygen; streams pass through
	}
	return resolve(args, stdout, stderr)
}

// hasYFlag detects git's ssh-keygen signing invocation, which always carries -Y.
func hasYFlag(args []string) bool {
	for _, a := range args {
		if a == "-Y" {
			return true
		}
	}
	return false
}

func resolve(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sighelper", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pattern := fs.String("pattern", env.Or("SIGHELPER_PATTERN", defaultPattern), "glob for agent sockets")
	timeout := fs.Duration("timeout", env.Duration("SIGHELPER_TIMEOUT", defaultTimeout), "per-probe timeout")
	keyRef := fs.String("key", os.Getenv("SIGHELPER_KEY"), "signing key override (path or literal pubkey)")
	verboseDef := env.Bool("SIGHELPER_VERBOSE")
	var verbose bool
	fs.BoolVar(&verbose, "v", verboseDef, "verbose per-candidate diagnostics to stderr")
	fs.BoolVar(&verbose, "verbose", verboseDef, "verbose per-candidate diagnostics to stderr")
	if err := fs.Parse(args); err != nil {
		return 2 // usage error
	}
	if *timeout <= 0 {
		fmt.Fprintln(stderr, "sighelper: --timeout must be positive")
		return 2
	}

	requiredKey, code := determineKey(*keyRef, stderr)
	if code != 0 {
		return code
	}

	cfg := resolver.Config{Pattern: *pattern, Timeout: *timeout, Verbose: verbose, RequiredKey: requiredKey}
	out := resolver.Resolve(cfg)
	if verbose {
		resolver.Report(stderr, out)
	}
	if out.Chosen == nil {
		fmt.Fprintln(stderr, "sighelper: no live, owned ssh-agent socket found")
		return 1
	}
	fmt.Fprintln(stdout, out.Chosen.Path)
	return 0
}

// determineKey resolves the required signing key: an explicit override (exit 2 on parse
// error), else git's configured key, else nil (key-agnostic).
func determineKey(override string, stderr io.Writer) (key []byte, code int) {
	if override != "" {
		k, err := signkey.ParseKey(override)
		if err != nil {
			fmt.Fprintf(stderr, "sighelper: %v\n", err)
			return nil, 2
		}
		return k, 0
	}
	k, _ := signkey.DetermineKey()
	return k, 0
}

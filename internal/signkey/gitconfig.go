package signkey

import (
	"os/exec"
	"strings"
)

// DetermineKey returns the SSH signing-key wire blob configured for git
// (gpg.format=ssh + user.signingkey), or nil when SSH signing is not configured —
// in which case the resolver selects key-agnostically (FR-010, research R4).
func DetermineKey() ([]byte, error) {
	if strings.TrimSpace(gitConfig("gpg.format")) != "ssh" {
		return nil, nil
	}
	key := strings.TrimSpace(gitConfig("user.signingkey"))
	if key == "" {
		return nil, nil
	}
	return ParseKey(key)
}

// gitConfig returns the value of a git config key, or "" if unset/unavailable. The key is
// a compile-time constant at every call site.
func gitConfig(key string) string {
	out, err := exec.Command("git", "config", "--get", key).Output() // G204 suppressed in .golangci.yml: key is a constant
	if err != nil {
		return ""
	}
	return string(out)
}

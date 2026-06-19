// Package signkey parses SSH public keys to their wire-format blob and matches them
// against agent-held identities. It handles only public information.
package signkey

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseKey resolves a signing-key reference — a path to a public-key file, or a literal
// authorized_keys-format public key — to its SSH wire-format blob (research R4).
func ParseKey(ref string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty signing-key reference")
	}
	// git records a literal (non-file) ssh signing key as "key::<type> <base64> [comment]".
	ref = strings.TrimPrefix(ref, "key::")
	line := ref
	if !isPublicKeyLiteral(ref) {
		data, err := os.ReadFile(expandHome(ref)) // G304 suppressed in .golangci.yml: user's own signing-key path
		if err != nil {
			return nil, fmt.Errorf("read signing key %s: %w", ref, err)
		}
		line = strings.TrimSpace(string(data))
	}
	return blobFromAuthorizedKey(line)
}

// Match reports whether any of the agent identity blobs equals the required key blob.
func Match(required []byte, blobs [][]byte) bool {
	for _, b := range blobs {
		if bytes.Equal(required, b) {
			return true
		}
	}
	return false
}

func isPublicKeyLiteral(s string) bool {
	for _, p := range []string{"ssh-", "sk-", "ecdsa-"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// blobFromAuthorizedKey extracts the wire blob from an authorized_keys-format line
// (`<type> <base64-blob> [comment]`), tolerating leading option fields.
func blobFromAuthorizedKey(line string) ([]byte, error) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		if !isPublicKeyLiteral(fields[i]) {
			continue
		}
		if blob, err := base64.StdEncoding.DecodeString(fields[i+1]); err == nil {
			return blob, nil
		}
	}
	return nil, fmt.Errorf("no ssh public key found in reference")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

package resolver

import (
	"fmt"
	"io"
	"strings"
)

// Report writes per-candidate accept/reject diagnostics to w (FR-012). It is secret-safe:
// it prints only paths, fingerprints, and comments — never key blobs or signatures
// (FR-011, SC-006).
func Report(w io.Writer, out SelectionOutcome) {
	for _, c := range out.Considered {
		switch {
		case out.Chosen != nil && c == out.Chosen:
			fmt.Fprintf(w, "%s  CHOSEN%s\n", c.Path, identitySummary(c))
		case c.Reject != RejectNone:
			fmt.Fprintf(w, "%s  rejected: %s\n", c.RawPath, c.Reject)
		default:
			fmt.Fprintf(w, "%s  eligible%s\n", c.Path, identitySummary(c))
		}
	}
}

func identitySummary(c *Candidate) string {
	if len(c.Identities) == 0 {
		return ""
	}
	fps := make([]string, len(c.Identities))
	for i, id := range c.Identities {
		fps[i] = id.Fingerprint
	}
	return " (holds " + strings.Join(fps, " ") + ")"
}

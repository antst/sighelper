package resolver

import (
	"os"
	"path/filepath"
	"syscall"
)

// checkTrust validates a candidate's (already-resolved) path: it MUST be a socket owned
// by uid, in a directory owned by uid and not group/world-writable (FR-002, FR-003,
// research R3). It sets Reject on failure, or Trusted=true and records OwnerUID/ModTime.
// uid is the current real UID as an int; socket owners (uint32) are widened to int for a
// safe, overflow-free comparison.
func checkTrust(c *Candidate, uid int) {
	fi, err := os.Lstat(c.Path)
	if err != nil {
		c.Reject = RejectNotSocket
		return
	}
	if fi.Mode()&os.ModeSocket == 0 {
		c.Reject = RejectNotSocket
		return
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		c.Reject = RejectUntrusted
		return
	}
	c.OwnerUID = st.Uid
	c.ModTime = fi.ModTime()
	if int(st.Uid) != uid {
		c.Reject = RejectForeign
		return
	}

	dir := filepath.Dir(c.Path)
	dfi, err := os.Stat(dir)
	if err != nil {
		c.Reject = RejectUntrusted
		return
	}
	dst, ok := dfi.Sys().(*syscall.Stat_t)
	if !ok || int(dst.Uid) != uid || dfi.Mode().Perm()&0o022 != 0 {
		c.Reject = RejectUntrusted
		return
	}
	c.Trusted = true
}

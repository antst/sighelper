package resolver

import (
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// probeAll concurrently probes every trusted candidate (research R2): each probe dials the
// socket and issues a read-only List() under cfg.Timeout. Bounding each probe keeps total
// wall-clock ~= the slowest live probe regardless of candidate count (FR-013, SC-002).
func probeAll(cands []*Candidate, cfg Config) {
	var wg sync.WaitGroup
	for _, c := range cands {
		if !c.Trusted {
			continue
		}
		wg.Add(1)
		go func(c *Candidate) {
			defer wg.Done()
			probe(c, cfg)
		}(c)
	}
	wg.Wait()
}

// probe confirms an agent is actually responding via a non-mutating List() request
// (FR-004, FR-005). A bare connection is not proof of liveness. On success it records the
// agent's public identities for key-aware selection.
func probe(c *Candidate, cfg Config) {
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.Dial("unix", c.Path)
	if err != nil {
		c.Reject = RejectDead
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(cfg.Timeout))

	keys, err := agent.NewClient(conn).List()
	if err != nil {
		c.Reject = RejectDead
		return
	}
	c.Live = true
	for _, k := range keys {
		c.Identities = append(c.Identities, Identity{
			Blob:        k.Blob,
			Fingerprint: ssh.FingerprintSHA256(k),
			Comment:     k.Comment,
		})
	}
}

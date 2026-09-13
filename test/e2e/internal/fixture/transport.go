//go:build e2e

// transport.go decides whether a fixture may put a credential on the wire.
//
// Two fixtures reach past the server and call GitLab directly, because the
// endpoints they need are not tools: the alert integration's notify URL takes
// a bearer token, and the Terraform state backend takes the run user's own
// token as basic authentication. Both are credentials, and neither call went
// through any check of what it was about to send them over.
//
// The rule here is the one the server itself applies to a destination the
// operator did not choose (ADR-0022): plain HTTP is fine for an instance that
// is demonstrably local, and is refused for anything else. A suite pointed at
// a public host over http:// would otherwise hand an administrator's token to
// whoever is between.

package fixture

import (
	"context"
	"net"
	"net/url"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// hostLookupTimeout bounds the one name resolution the transport check makes,
// so an unreachable resolver stalls nothing.
const hostLookupTimeout = 5 * time.Second

// requireConfidentialTransport fails the test unless the given URL is one a
// credential may be sent to: https, or http to an instance this run owns.
//
// The Docker stack is the ordinary case and is served over plain http on a
// private address, so it is admitted both by the run's own mode and by the
// address test. A self-hosted instance on a private network is admitted too,
// since it is the operator's own wire; a public host over http is not.
func requireConfidentialTransport(e *harness.Env, raw, what string) {
	e.T.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		e.T.Fatalf("%s: parsing %q before sending a credential to it: %v", what, raw, err)
	}
	if parsed.Scheme == "https" {
		return
	}
	if parsed.Scheme != "http" {
		e.T.Fatalf("%s: refusing to send a credential over %q (%s)", what, parsed.Scheme, raw)
	}
	if e.DockerMode() || localHost(e.Ctx, parsed.Hostname()) {
		return
	}
	e.T.Fatalf("%s: refusing to send a credential in clear text to %s; serve the instance over https "+
		"or run against the Docker stack", what, raw)
}

// localHost reports whether a host names an address this run can treat as its
// own wire: a loopback, link-local or private address, or a name that resolves
// only to those. A name nothing can resolve is not local.
func localHost(ctx context.Context, host string) bool {
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return privateAddress(ip)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, hostLookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, addr := range addrs {
		if !privateAddress(addr.IP) {
			return false
		}
	}
	return true
}

// privateAddress reports whether an address is one that does not leave the
// operator's own network.
func privateAddress(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

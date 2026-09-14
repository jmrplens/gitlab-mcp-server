//go:build e2e

// mcp_http_transport_test.go drives the binary over HTTP against a real
// GitLab, which is the one combination nothing covered.
//
// The suite runs on stdio, and test/e2e/http drives the HTTP handler chain with
// no GitLab and no credentials. Between them sits the deployment shape this
// server is actually run as when it is shared: a listener, a credential in a
// header rather than in the environment, and a per-request client bound from a
// pool entry. Every one of those is HTTP-only machinery, and a tool call that
// reaches GitLab through it is the only thing that exercises the binding.
//
// It is a handful of scenarios rather than a second copy of the suite. The
// actions behave the same whatever carries them — that is what the catalog
// being one catalog means — so what is worth asserting here is the carriage:
// that a call arrives, that it runs under the caller's own credential, and that
// what comes back is the same answer stdio gives.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestHTTPTransport_ACallReachesGitLabUnderTheCallersCredential is the whole
// point of the transport: the server holds no token, so an answer that names
// the run's own user proves the header was read, resolved to a pool entry, and
// bound to the handler that made the call.
//
// user.current is chosen because its answer is the credential: any other action
// would prove the call arrived and say nothing about whose it was.
func TestHTTPTransport_ACallReachesGitLabUnderTheCallersCredential(t *testing.T) {
	e := harness.New(t)
	s := e.Session(harness.ServerConfig{
		Surface:   harness.SurfaceDynamic,
		Transport: harness.TransportHTTP,
	})

	if got := s.Transport(); got != harness.TransportHTTP {
		t.Fatalf("the session reports transport %q, want %q", got, harness.TransportHTTP)
	}

	current := harness.Do[users.Output](s, actionUserCurrent, nil)
	if current.Username != e.Runtime().Username {
		t.Errorf("the call ran as %q and the run's credential is %q: the header did not decide who called",
			current.Username, e.Runtime().Username)
	}
}

// TestHTTPTransport_ServesTheSameSurfaceAsStdio holds the two transports to one
// catalog.
//
// The surfaces are projected from the same catalog and neither transport is
// supposed to touch it, so a difference here would mean the shape a deployment
// serves depends on how it is reached — which is exactly the kind of drift a
// suite that only ever ran stdio could not see.
func TestHTTPTransport_ServesTheSameSurfaceAsStdio(t *testing.T) {
	e := harness.New(t)
	overHTTP := e.Session(harness.ServerConfig{
		Surface:   harness.SurfaceDynamic,
		Transport: harness.TransportHTTP,
	})
	overStdio := e.On(harness.SurfaceDynamic)

	if got, want := len(overHTTP.Actions()), len(overStdio.Actions()); got != want {
		t.Errorf("the HTTP session reaches %d actions and the stdio one %d, want one catalog", got, want)
	}
	if got, want := len(overHTTP.Tools()), len(overStdio.Tools()); got != want {
		t.Errorf("the HTTP session serves %d tools and the stdio one %d", got, want)
	}
	if got, want := overHTTP.Tier(), overStdio.Tier(); got != want {
		t.Errorf("the HTTP session resolved tier %s and the stdio one %s", got, want)
	}
}

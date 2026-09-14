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
	"slices"
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

	// The sets rather than their sizes: two catalogs of the same size can be
	// different catalogs, and a swap of one action for another is exactly the
	// drift this is here to catch.
	assertSameSurface(t, "actions", actionNames(overHTTP.Actions()), actionNames(overStdio.Actions()))
	assertSameSurface(t, "tools", overHTTP.Tools(), overStdio.Tools())
	if got, want := overHTTP.Tier(), overStdio.Tier(); got != want {
		t.Errorf("the HTTP session resolved tier %s and the stdio one %s", got, want)
	}
}

// assertSameSurface fails when the two transports publish different listings,
// naming what each has that the other does not rather than only that they
// differ: the names are what a reader needs to tell a catalog change from a
// transport defect.
func assertSameSurface(t *testing.T, what string, overHTTP, overStdio []string) {
	t.Helper()
	if only := missingFrom(overStdio, overHTTP); len(only) > 0 {
		t.Errorf("the HTTP session serves %s the stdio one does not: %v", what, only)
	}
	if only := missingFrom(overHTTP, overStdio); len(only) > 0 {
		t.Errorf("the stdio session serves %s the HTTP one does not: %v", what, only)
	}
}

// missingFrom returns the entries of have that want does not carry.
func missingFrom(want, have []string) []string {
	present := make(map[string]struct{}, len(want))
	for _, name := range want {
		present[name] = struct{}{}
	}
	var missing []string
	for _, name := range have {
		if _, ok := present[name]; !ok {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

// actionNames renders a listing of action IDs as the strings a comparison
// reads.
func actionNames(actions []harness.ActionID) []string {
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, string(action))
	}
	return names
}

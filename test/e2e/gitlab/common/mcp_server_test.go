//go:build e2e

// mcp_server_test.go covers the gitlab_server diagnostics group: the one
// action every surface serves, and its twin that declares no individual tool.
//
// It is the first scenario of the rebuilt suite on purpose. A status read
// touches nothing, needs no fixture, and still proves the whole chain: the
// binary started from an environment built from nothing, listed what the
// assemblers said it would, took a call spelled the way its surface spells
// it, reached GitLab with the run's credential and answered with what it
// found there.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestServer_Status runs server.status on all three surfaces and holds the
// answer to what the harness's own probe learned about the instance: the
// same URL, the same version, the same user.
func TestServer_Status(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		out := harness.Do[health.Output](s, actionServerStatus, nil)

		assertHealthy(e, out)
	})
}

// TestServer_HealthCheck runs server.health_check, the diagnostics action that
// declares no individual tool, on the two surfaces that can reach it.
//
// It is the test issue 616 found missing: the old suite's meta session was
// built without the diagnostics group, so nothing ever called this action
// through a dispatcher.
func TestServer_HealthCheck(t *testing.T) {
	e := harness.New(t)

	harness.OnSurfaces(e, "server.health_check declares no individual tool, so the individual surface cannot reach it",
		[]harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta},
		func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)

			out := harness.Do[health.Output](s, actionServerHealthCheck, nil)

			assertHealthy(e, out)
		})
}

// assertHealthy holds a diagnostics answer to the instance the harness probed.
//
// The URL is compared as a prefix, because the server reports the API base it
// built from the instance rather than the instance itself.
func assertHealthy(e *harness.Env, out health.Output) {
	e.T.Helper()
	rt := e.Runtime()

	if out.Status != "healthy" {
		e.T.Errorf("status = %q, want healthy (error: %q)", out.Status, out.Error)
	}
	if !out.Authenticated {
		e.T.Error("the server reports it is not authenticated with the run's credential")
	}
	if !strings.HasPrefix(out.GitLabURL, strings.TrimRight(rt.URL, "/")) {
		e.T.Errorf("gitlab_url = %q, want it to name the instance the harness pointed the server at, %q", out.GitLabURL, rt.URL)
	}
	if out.GitLabVersion != rt.Version {
		e.T.Errorf("gitlab_version = %q, want %q, which the probe read from the same instance", out.GitLabVersion, rt.Version)
	}
	if out.Username != rt.Username || out.UserID != rt.UserID {
		e.T.Errorf("authenticated as %q (%d), want %q (%d)", out.Username, out.UserID, rt.Username, rt.UserID)
	}
}

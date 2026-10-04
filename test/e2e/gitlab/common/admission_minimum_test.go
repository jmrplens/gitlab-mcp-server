//go:build e2e

// admission_minimum_test.go covers the read_api admission minimum (issue 952)
// against a real GitLab: a classic token GitLab accepts that carries neither
// read_api nor api reaches no tool, so a server started on one serves nothing,
// on stdio and over HTTP alike.
//
// The stand-ins of test/e2e/stdio and test/e2e/http answer as GitLab 19.4.1
// was measured to answer each scope, and they hold --ignore-scopes, which this
// harness has no knob for. What only a real instance can say is whether that
// measurement still holds: which of GET /api/v4/version, GET /api/v4/user and
// GET /api/v4/personal_access_tokens/self it answers a token carrying
// read_user alone, or self_rotate alone, on the release under test.

package common

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The stable openings of the two refusals of a token below the minimum, as
// register row ADM-001 declares them: the stdio process's own sentence, and
// the legacy HTTP gate's.
const (
	stdioBelowMinimumPrefix = "GitLab accepted the token this server was started with"
	gateBelowMinimumPrefix  = "GitLab accepted this token, which carries neither the read_api nor the api scope"
)

// TestAdmissionMinimum_TokenBelowIt_IsRefusedOnStdioAndOverHTTP mints two
// classic tokens below the minimum for a user of the run's own: read_user,
// which GitLab answers the user and the version with, and self_rotate, which
// it refuses both for want of a scope. On stdio each one's handshake is
// answered and tools/list refused in-band with -40300 and the process's
// sentence, since the process keeps serving rather than exiting; over HTTP the
// gate refuses the handshake itself, 403 with -40300 and its own sentence.
// Each scope is a subtest with an Env of its own, so a failure is reported on
// the goroutine of the test that met it.
func TestAdmissionMinimum_TokenBelowIt_IsRefusedOnStdioAndOverHTTP(t *testing.T) {
	for _, scope := range []string{"read_user", "self_rotate"} {
		t.Run(scope, func(t *testing.T) {
			e := harness.New(t, harness.Needs(harness.NeedAdmin))
			token := fixture.NewToken(e, fixture.NewUser(e, "minimum"), scope)

			stdio := e.RefusedAdmission(harness.ServerConfig{Token: token.Value, Transport: harness.TransportStdio})
			if !stdio.HandshakeAnswered || stdio.Code != tenancy.CodeForbidden || !strings.HasPrefix(stdio.Message, stdioBelowMinimumPrefix) {
				t.Errorf("a %s token on stdio was answered %+v, want the handshake answered and tools/list refused with %d and %q",
					scope, stdio, tenancy.CodeForbidden, stdioBelowMinimumPrefix)
			}

			overHTTP := e.RefusedAdmission(harness.ServerConfig{Token: token.Value, Transport: harness.TransportHTTP})
			if overHTTP.Status != http.StatusForbidden || overHTTP.Code != tenancy.CodeForbidden || !strings.HasPrefix(overHTTP.Message, gateBelowMinimumPrefix) {
				t.Errorf("a %s token over HTTP was answered %+v, want the handshake refused %d with %d and %q",
					scope, overHTTP, http.StatusForbidden, tenancy.CodeForbidden, gateBelowMinimumPrefix)
			}
		})
	}
}

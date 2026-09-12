//go:build e2e

// mcp_scopes_test.go covers what a credential's scopes decide: the server
// reads the token's own scopes at startup and narrows its surface to what
// they allow, before anything is registered, on every surface.
//
// The old suite tested this by building a catalog in its own process with
// the scopes it had detected, which proved that the filter works when called
// and nothing about the binary calling it. Here the session is started with
// the narrow token and the binary does its own narrowing: the harness checks
// what it served against the assemblers, and the calls show the narrowed
// surface from the client's side.

package common

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The scopes this file mints and asks about, spelled as GitLab spells them.
const (
	scopeReadAPI   = "read_api"
	scopeAdminMode = "admin_mode"
)

// TestScope_ReadAPITokenIsReadOnly starts a session on every surface with a
// token that can only read, minted for a user who is not an administrator,
// and checks that the binary served it a read-only surface with the admin
// group withheld: reads work and answer as that user, a write is declined,
// and the admin group is declined too, naming the credential as the cause
// where the surface can say so.
//
// The session's tier is Free on every runtime, licensed ones included: the
// server detects its tier from the license endpoint with the token it was
// started with, and that endpoint answers administrators only. The first
// licensed run of this test found that, when the harness expected the run's
// tier for the session and the server had registered no licensed group.
//
// Replaces: TestScopeFilter_NonAdminToken
func TestScope_ReadAPITokenIsReadOnly(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		user := fixture.NewUser(e, "scope")
		return fixture.NewToken(e, user, scopeReadAPI)
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value})

		if got := s.Mode(); got != harness.ModeReadOnly {
			e.T.Errorf("a %s session on a read_api token is recorded in %s mode, want %s", surface, got, harness.ModeReadOnly)
		}
		if got := s.Tier(); got != edition.Free {
			e.T.Errorf("a %s session on a token that cannot read the license detected tier %s, want %s", surface, got, edition.Free)
		}

		me := harness.Do[users.Output](s, actionUserCurrent, nil)
		if me.ID != token.UserID {
			e.T.Errorf("the session authenticated as user %d (%s), want the token's owner %d", me.ID, me.Username, token.UserID)
		}
		if me.IsAdmin {
			e.T.Errorf("the token's owner %s is an administrator, and the scenario needs one who is not", me.Username)
		}

		if s.Serves(actionIssueCreate) {
			e.T.Errorf("the read_api session serves %s", actionIssueCreate)
		}
		// The arguments never reach GitLab: the refusal comes before any
		// handler runs, so the project is a placeholder.
		declined := harness.Withheld(s, actionIssueCreate, map[string]any{"project_id": "1", "title": "must not be created"})
		assertNamesTheCredential(e, surface, actionIssueCreate, declined)

		if s.Serves(actionAdminMetadataGet) {
			e.T.Errorf("the read_api session serves %s, which needs %s", actionAdminMetadataGet, scopeAdminMode)
		}
		declined = harness.Withheld(s, actionAdminMetadataGet, nil)
		assertNamesTheCredential(e, surface, actionAdminMetadataGet, declined)
	})
}

// assertNamesTheCredential checks that a refusal says the credential is what
// is narrow, on the one surface whose dispatcher can say so.
//
// The dynamic dispatcher answers a withheld action with the reason, because a
// model told "unknown action" concludes the server lacks the capability. The
// meta dispatcher has no route to name and the individual surface has no
// tool, so those two can only decline.
func assertNamesTheCredential(e *harness.Env, surface harness.Surface, id harness.ActionID, declined string) {
	e.T.Helper()
	if surface != harness.SurfaceDynamic {
		return
	}
	if !strings.Contains(declined, "scope") {
		e.T.Errorf("%s was declined without naming the credential's scope as the cause: %q", id, declined)
	}
}

// TestScope_AdminModeToken_IsServedTheAdminGroup checks the other direction:
// the run's own token is served the admin group exactly when its scopes
// carry admin_mode, and the group answers when it is served.
//
// The Docker fixture mints its token with admin_mode, so there this is the
// positive case: nothing the filter reads takes anything away. A self-hosted
// run whose token lacks the scope sees the group withheld instead, and both
// are the binary's own reading of the token.
//
// Replaces: TestScopeFilter_AdminToken
func TestScope_AdminModeToken_IsServedTheAdminGroup(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()
	// Unknown scopes narrow nothing: detection that failed counts as
	// write-capable and admin-capable, for the reason WriteCapable gives.
	wantAdmin := rt.Scopes == nil || slices.Contains(rt.Scopes, scopeAdminMode)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		if got := s.Serves(actionAdminMetadataGet); got != wantAdmin {
			e.T.Fatalf("the %s session serves %s = %t, want %t for a token with scopes %v", surface, actionAdminMetadataGet, got, wantAdmin, rt.Scopes)
		}
		if !wantAdmin {
			declined := harness.Withheld(s, actionAdminMetadataGet, nil)
			assertNamesTheCredential(e, surface, actionAdminMetadataGet, declined)
			return
		}

		out := harness.Do[metadata.GetOutput](s, actionAdminMetadataGet, nil)
		if out.Version != rt.Version {
			e.T.Errorf("metadata version = %q, want %q, which the probe read from the same instance", out.Version, rt.Version)
		}
		if out.Enterprise != rt.Enterprise {
			e.T.Errorf("metadata enterprise = %t, want %t", out.Enterprise, rt.Enterprise)
		}
	})
}

//go:build e2e

// mcp_scopes_test.go covers what a credential's scopes decide: the server
// reads the token's own scopes at startup and narrows its surface to what
// GitLab accepts from them, action by action, before anything is registered,
// on every surface.
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
	markdowntool "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/markdown"
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

// TestScope_ReadAPIToken_IsServedWhatReadAPIReaches starts a session on every
// surface with a token carrying read_api and not api, minted for a user who
// is not an administrator, and checks that the binary served it the actions
// GitLab accepts from read_api, judged per action from what each sends rather
// than from whether it writes (register row AUT-001, ADR-0026).
//
// Four actions show the rule from the client's side. The current user and the
// markdown render are served and answer, the second although it is sent as a
// POST, because GitLab grants read_api to that route; the issue create, a
// write, and the CI lint, a read sent as a POST GitLab answers only from api,
// are declined naming api; and the admin group is declined naming admin_mode.
// The package download, which writes a local file and reads GitLab with GETs,
// is served, which the read-only surface this narrowing replaced did not.
// The session runs in the default mode: the narrowing is the credential's and
// never a protective mode.
//
// The session's tier is Free on every runtime, licensed ones included: the
// server detects its tier from the license endpoint with the token it was
// started with, and that endpoint answers administrators only. The first
// licensed run of this test found that, when the harness expected the run's
// tier for the session and the server had registered no licensed group.
//
// Replaces: TestScopeFilter_NonAdminToken
func TestScope_ReadAPIToken_IsServedWhatReadAPIReaches(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		user := fixture.NewUser(e, "scope")
		return fixture.NewToken(e, user, scopeReadAPI)
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value})

		if got := s.Mode(); got != harness.ModeDefault {
			e.T.Errorf("a %s session on a read_api token is recorded in %s mode, want %s", surface, got, harness.ModeDefault)
		}
		if !s.ReadAPIOnly() {
			e.T.Errorf("a %s session on a read_api token is not recorded as narrowed to what read_api reaches", surface)
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

		rendered := harness.Do[markdowntool.RenderOutput](s, actionRepositoryMarkdownRender, map[string]any{"text": "**bold** text"})
		if !strings.Contains(rendered.HTML, ">bold</strong>") {
			e.T.Errorf("markdown_render on a read_api token answered %q, want the bold element GitLab renders", rendered.HTML)
		}
		if !s.Serves(actionPackageDownload) {
			e.T.Errorf("the read_api session does not serve %s, whose every request GitLab accepts from read_api", actionPackageDownload)
		}

		// The arguments never reach GitLab: the refusal comes before any
		// handler runs, so the project is a placeholder.
		declined := harness.Withheld(s, actionIssueCreate, map[string]any{"project_id": "1", "title": "must not be created"})
		assertNamesTheScope(e, surface, actionIssueCreate, declined, "api")
		declined = harness.Withheld(s, actionTemplateLint, map[string]any{"content": "job:\n  script: echo\n"})
		assertNamesTheScope(e, surface, actionTemplateLint, declined, "api")
		declined = harness.Withheld(s, actionAdminMetadataGet, nil)
		assertNamesTheScope(e, surface, actionAdminMetadataGet, declined, scopeAdminMode)
	})
}

// assertNamesTheScope checks that a refusal names the scope the credential
// lacks, and who asks for it, on the one surface whose dispatcher can say so.
//
// The dynamic dispatcher answers a withheld action with the reason, because a
// model told "unknown action" concludes the server lacks the capability. The
// meta dispatcher has no route to name and the individual surface has no
// tool, so those two can only decline.
//
// admin_mode is this server's demand for the administration groups and not
// GitLab's for each action in them: GitLab serves admin.metadata_get to any
// authenticated token, so the answer must not put admin_mode on GitLab.
func assertNamesTheScope(e *harness.Env, surface harness.Surface, id harness.ActionID, declined, scope string) {
	e.T.Helper()
	if surface != harness.SurfaceDynamic {
		return
	}
	want := "GitLab requires the " + scope + " scope"
	if scope == scopeAdminMode {
		want = "this server serves this action's group only to a credential carrying the " + scope + " scope"
	}
	if !strings.Contains(declined, want) {
		e.T.Errorf("%s was declined without naming the %s scope the credential lacks (want %q): %q", id, scope, want, declined)
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
			assertNamesTheScope(e, surface, actionAdminMetadataGet, declined, scopeAdminMode)
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

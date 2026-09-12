//go:build e2e

// scope_filter_ce_test.go verifies PAT scope-based tool filtering in an
// end-to-end scenario. It creates a non-admin user with a limited-scope
// token and asserts that admin-only meta-tools are removed while regular
// meta-tools remain accessible.
package suite

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
)

// TestScopeFilter_NonAdminToken creates a non-admin user with a read_api
// token and verifies that scope-filtered tools (admin_mode) are removed
// while regular meta-tools remain registered.
//
// The test creates the user via the admin session, issues a read_api PAT,
// detects scopes via the raw GitLab client, builds an in-memory MCP server
// with meta-tools, applies the scope filter, and lists remaining tools.
// Assertions check that admin-only meta-tools are removed, regular
// meta-tools are still present, and the read_api scope is observed.
//
// Build tag: e2e. Mode: CE. Surface: meta.
func TestScopeFilter_NonAdminToken(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not available")
	}

	ctx := context.Background()
	uname := uniqueName("e2e-scope")

	// ── Create non-admin user via admin session ──────────────────────────
	userOut, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
		"action": "create",
		"params": map[string]any{
			"email":                 uname + "@e2e-test.local",
			"name":                  "E2E Scope Test " + uname,
			"username":              uname,
			"password":              "E2eS!Kx9Z#p2mNq$8BcR",
			"skip_confirmation":     true,
			"force_random_password": false,
		},
	})
	requireNoError(t, err, "create non-admin user")
	requireTruef(t, userOut.ID > 0, "non-admin user ID > 0")
	requireTruef(t, !userOut.IsAdmin, "user should not be admin")
	t.Logf("Created non-admin user %s (ID: %d)", uname, userOut.ID)

	defer func() {
		_, _ = callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "unblock",
			"params": map[string]any{"user_id": userOut.ID},
		})
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "delete",
			"params": map[string]any{"user_id": userOut.ID},
		})
		t.Logf("Cleaned up user %s (ID: %d)", uname, userOut.ID)
	}()

	// ── Create PAT with read_api scope for the non-admin user ────────────
	patOut, err := callToolOn[impersonationtokens.PATOutput](ctx, sess.meta, "gitlab_user", map[string]any{
		"action": "create_personal_access_token",
		"params": map[string]any{
			"user_id":    userOut.ID,
			"name":       "e2e-scope-pat",
			"scopes":     []string{"read_api"},
			"expires_at": time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
		},
	})
	requireNoError(t, err, "create PAT for non-admin user")
	requireTruef(t, patOut.Token != "", "PAT token should not be empty")
	t.Logf("Created read_api PAT for user %s", uname)

	// ── Build a GitLab client with the limited-scope token ───────────────
	gitlabURL := os.Getenv("GITLAB_URL")
	if gitlabURL == "" {
		t.Fatal("GITLAB_URL not set")
	}
	limitedClient, err := gitlabclient.NewClientWithToken(gitlabURL, patOut.Token, true)
	if err != nil {
		t.Fatalf("create limited client: %v", err)
	}

	// ── Detect scopes for the limited token ──────────────────────────────
	scopes := gitlabclient.DetectScopes(ctx, limitedClient.GL())
	if scopes == nil {
		t.Fatal("scope detection returned nil — expected scopes for the PAT")
	}
	t.Logf("Detected scopes for non-admin token: %v", scopes)

	// Verify read_api is present but admin_mode is not.
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = struct{}{}
	}
	if _, ok := scopeSet["read_api"]; !ok {
		t.Error("expected read_api in detected scopes")
	}
	if _, ok := scopeSet["admin_mode"]; ok {
		t.Error("non-admin token should not have admin_mode scope")
	}

	// ── Register the meta surface the way the binary does ────────────────
	// The catalog is filtered by the token's scopes before registration,
	// which is the only mechanism there is: a pass over registered names
	// after the fact could not reach the individual surface at all. Building
	// the catalog here through the same assembler cmd/server calls is what
	// keeps this test about the server rather than about a copy of it.
	toolSet := registeredMetaToolNames(t, ctx, limitedClient, scopes)

	// ── Assertions ───────────────────────────────────────────────────────

	// Admin-only tools must be removed. Enterprise groups may not exist at
	// this tier at all, which is why the assertion is one-directional: a
	// group that is absent for either reason satisfies it.
	for name, required := range tools.MetaToolScopes {
		if !slices.Contains(required, "admin_mode") {
			continue
		}
		t.Run("removed/"+name, func(t *testing.T) {
			if _, ok := toolSet[name]; ok {
				t.Errorf("admin-only tool %s should have been removed for non-admin token", name)
			}
		})
	}

	// Regular meta-tools must still be present.
	regularTools := []string{
		"gitlab_project",
		"gitlab_issue",
		"gitlab_merge_request",
		"gitlab_branch",
		"gitlab_user",
	}
	for _, name := range regularTools {
		t.Run("kept/"+name, func(t *testing.T) {
			if _, ok := toolSet[name]; !ok {
				t.Errorf("regular tool %s should still be registered for non-admin token", name)
			}
		})
	}

	t.Logf("Scope filter test passed: %d tools registered for the read_api token", len(toolSet))
}

// TestScopeFilter_AdminToken creates a PAT with admin_mode scope for the
// existing admin user and verifies that the scope filter takes nothing away
// from it.
//
// The test issues an admin_mode PAT for the current admin user, detects scopes
// via the raw GitLab client, and registers the meta surface twice: once with
// those scopes and once with detection reported unavailable, which is the
// filter's own "remove nothing" case. The assertion is that the two listings
// are identical, since admin_mode satisfies every requirement in the map.
// Comparing against the unfiltered listing rather than against a fixed set of
// names keeps the test tier-independent: a group absent on this runtime is
// absent from both sides.
//
// Build tag: e2e. Mode: CE. Surface: meta. Admin token required.
func TestScopeFilter_AdminToken(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not available")
	}

	ctx := context.Background()

	// ── Get admin user ID ────────────────────────────────────────────────
	adminUser, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
		"action": "current",
	})
	requireNoError(t, err, "get current admin user")
	requireTruef(t, adminUser.ID > 0, "admin user ID > 0")

	// ── Create PAT with admin_mode scope for the admin user ──────────────
	adminPAT, err := callToolOn[impersonationtokens.PATOutput](ctx, sess.meta, "gitlab_user", map[string]any{
		"action": "create_personal_access_token",
		"params": map[string]any{
			"user_id":    adminUser.ID,
			"name":       "e2e-scope-admin-pat",
			"scopes":     []string{"api", "admin_mode"},
			"expires_at": time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
		},
	})
	requireNoError(t, err, "create admin_mode PAT")
	requireTruef(t, adminPAT.Token != "", "admin PAT token should not be empty")
	t.Logf("Created api+admin_mode PAT for admin user (ID: %d)", adminUser.ID)

	// ── Build client with the admin_mode token ───────────────────────────
	gitlabURL := os.Getenv("GITLAB_URL")
	if gitlabURL == "" {
		t.Fatal("GITLAB_URL not set")
	}
	adminClient, err := gitlabclient.NewClientWithToken(gitlabURL, adminPAT.Token, true)
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}

	// ── Detect scopes ────────────────────────────────────────────────────
	scopes := gitlabclient.DetectScopes(ctx, adminClient.GL())
	if scopes == nil {
		t.Skip("scope detection unavailable for admin token")
	}
	t.Logf("Admin token scopes: %v", scopes)

	scopeSet := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = struct{}{}
	}
	if _, ok := scopeSet["admin_mode"]; !ok {
		t.Fatal("expected admin_mode in detected scopes")
	}

	// ── Register with the admin scopes and with none, and compare ────────
	withAdmin := registeredMetaToolNames(t, ctx, adminClient, scopes)
	unfiltered := registeredMetaToolNames(t, ctx, adminClient, nil)

	for name := range unfiltered {
		if _, kept := withAdmin[name]; !kept {
			t.Errorf("%s was removed for an admin_mode token", name)
		}
	}
	if len(withAdmin) != len(unfiltered) {
		t.Errorf("admin_mode listing has %d tools, unfiltered has %d", len(withAdmin), len(unfiltered))
	}
	t.Logf("Admin token: %d tools registered, same as with no scope filtering", len(withAdmin))
}

// registeredMetaToolNames builds the meta surface for one client and one
// detected scope list the way cmd/server builds it, and returns the tool names
// a client would be served.
//
// The catalog assembler is the shipped one on purpose. The scope filter is
// applied to the catalog before registration, so a test that registered an
// unfiltered catalog and then removed names from the server would be exercising
// its own copy of a mechanism the binary no longer has.
func registeredMetaToolNames(t *testing.T, ctx context.Context, client *gitlabclient.Client, scopes []string) map[string]struct{} {
	t.Helper()

	catalog, _, err := tools.SharedMetaCatalog(client, &config.ServerConfig{
		Tier:        edition.TierForEnterprise(sess.enterprise),
		TokenScopes: scopes,
	})
	if err != nil {
		t.Fatalf("SharedMetaCatalog(scopes=%v) error = %v", scopes, err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "gitlab-mcp-server-e2e-scope", Version: "test"}, nil)
	tools.RegisterMetaCatalog(server, catalog)
	tools.RegisterMetaStandaloneTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "e2e-scope-client", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.ListTools(ctx, nil)
	requireNoError(t, err, "ListTools on the scope-filtered surface")

	names := make(map[string]struct{}, len(result.Tools))
	for _, tool := range result.Tools {
		names[tool.Name] = struct{}{}
	}
	return names
}

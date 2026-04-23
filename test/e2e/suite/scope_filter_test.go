//go:build e2e

// scope_filter_test.go verifies PAT scope-based tool filtering in an
// end-to-end scenario. It creates a non-admin user with a limited-scope
// token and asserts that admin-only meta-tools are removed while regular
// meta-tools remain accessible.

package suite

import (
	"context"
	"os"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestScopeFilter_NonAdminToken creates a non-admin user with a read_api
// token and verifies that scope-filtered tools (admin_mode) are removed
// while regular meta-tools remain registered.
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
	requireTrue(t, userOut.ID > 0, "non-admin user ID > 0")
	requireTrue(t, !userOut.IsAdmin, "user should not be admin")
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
	requireTrue(t, patOut.Token != "", "PAT token should not be empty")
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

	// ── Create MCP server with meta-tools and apply scope filter ─────────
	scopeServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-scope",
		Version: "test",
	}, nil)
	tools.RegisterAllMeta(scopeServer, limitedClient, sess.enterprise)

	removed := tools.RemoveScopeFilteredTools(scopeServer, scopes)
	t.Logf("Scope filter removed %d tools", removed)

	// ── Connect a client to list remaining tools ─────────────────────────
	st, ct := mcp.NewInMemoryTransports()
	scopeCtx, scopeCancel := context.WithCancel(ctx)
	defer scopeCancel()

	go func() {
		_ = scopeServer.Run(scopeCtx, st)
	}()

	scopeClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-scope-client",
		Version: "test",
	}, nil)
	scopeSession, err := scopeClient.Connect(scopeCtx, ct, nil)
	if err != nil {
		t.Fatalf("connect scope client: %v", err)
	}
	defer scopeSession.Close()

	result, err := scopeSession.ListTools(scopeCtx, nil)
	requireNoError(t, err, "ListTools on scope-filtered server")

	toolSet := make(map[string]struct{}, len(result.Tools))
	for _, tool := range result.Tools {
		toolSet[tool.Name] = struct{}{}
	}

	// ── Assertions ───────────────────────────────────────────────────────

	// Admin-only tools must be removed.
	var adminOnlyTools []string
	for name, scopes := range tools.MetaToolScopes {
		for _, s := range scopes {
			if s == "admin_mode" {
				adminOnlyTools = append(adminOnlyTools, name)
				break
			}
		}
	}
	for _, name := range adminOnlyTools {
		if _, ok := toolSet[name]; ok {
			// Enterprise tools may not be registered at all — only fail
			// if the tool is registered AND should have been removed.
			t.Errorf("admin-only tool %s should have been removed for non-admin token", name)
		}
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
		if _, ok := toolSet[name]; !ok {
			t.Errorf("regular tool %s should still be registered for non-admin token", name)
		}
	}

	t.Logf("Scope filter test passed: %d tools registered, %d removed", len(result.Tools), removed)
}

// TestScopeFilter_AdminToken creates a PAT with admin_mode scope for the
// existing admin user and verifies that NO tools are removed by scope filtering.
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
	requireTrue(t, adminUser.ID > 0, "admin user ID > 0")

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
	requireTrue(t, adminPAT.Token != "", "admin PAT token should not be empty")
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

	// ── Create server, apply scope filter, expect 0 removals ─────────────
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-scope-admin",
		Version: "test",
	}, nil)
	tools.RegisterAllMeta(server, adminClient, sess.enterprise)

	removed := tools.RemoveScopeFilteredTools(server, scopes)
	if removed != 0 {
		t.Errorf("expected 0 tools removed for admin_mode token, got %d", removed)
	}
	t.Logf("Admin token: %d tools removed", removed)
}

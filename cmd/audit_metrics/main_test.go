// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// newAuditMetricsClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for audit_metrics tests.
func newAuditMetricsClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "audit-token"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	return client
}

// TestCountResources_IncludesMetaSchema verifies resource metrics include the
// meta-schema resource registration path used by the audit command.
func TestCountResources_IncludesMetaSchema(t *testing.T) {
	static, templates := countResources(newAuditMetricsClient(t))
	if static == 0 {
		t.Fatal("countResources() static = 0, want registered resources")
	}
	if templates == 0 {
		t.Fatal("countResources() templates = 0, want registered templates")
	}
}

// TestListDynamicTools_ExposesThreeTools verifies audit metrics count the
// dynamic public surface independently from catalog action volume.
func TestListDynamicTools_ExposesThreeTools(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": {Handler: func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{"ok": true}, nil }},
		},
	}

	dynamicTools := listDynamicTools(actionregistry.FromActionMaps(routes))
	if len(dynamicTools) != 3 {
		t.Fatalf("listDynamicTools() count = %d, want 3", len(dynamicTools))
	}
	names := make([]string, 0, len(dynamicTools))
	for _, tool := range dynamicTools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	for _, want := range []string{"gitlab_describe_tools", "gitlab_execute_tool", "gitlab_search_tools"} {
		if !slices.Contains(names, want) {
			t.Fatalf("listDynamicTools() names = %v, missing %q", names, want)
		}
	}
}

// TestCountActionRoutes_CountsCatalogActions verifies catalog route counting is
// independent from MCP tool advertisement.
func TestCountActionRoutes_CountsCatalogActions(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": {}, "list": {}},
		"gitlab_issue":   {"create": {}},
	}

	if got := countActionRoutes(routes); got != 3 {
		t.Fatalf("countActionRoutes() = %d, want 3", got)
	}
}

// TestDynamicSearchMetrics_ReportsIndexAndAliasCounts verifies static dynamic
// search metrics are available without adding visible MCP tools.
func TestDynamicSearchMetrics_ReportsIndexAndAliasCounts(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_repository": {
			"tree": {Handler: func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{"ok": true}, nil }},
		},
		"gitlab_project": {
			"delete":   {Handler: func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{"ok": true}, nil }, Destructive: true},
			"hook_add": {Handler: func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{"ok": true}, nil }},
		},
	}
	catalog := actionregistry.FromActionMaps(routes)

	metrics := dynamicSearchMetrics(catalog)
	if metrics.ActionCount != 3 {
		t.Fatalf("ActionCount = %d, want 3", metrics.ActionCount)
	}
	if metrics.IndexTokenCount == 0 || metrics.IndexPostingCount == 0 {
		t.Fatalf("metrics = %+v, want populated search index metrics", metrics)
	}
	if metrics.AliasCount == 0 || metrics.SearchableAliasCount == 0 {
		t.Fatalf("metrics = %+v, want alias metrics", metrics)
	}
	if metrics.UnsearchableAliasCount == 0 {
		t.Fatalf("metrics = %+v, want repository_tree unsearchable alias counted", metrics)
	}
	if len(listDynamicTools(catalog)) != 3 {
		t.Fatal("dynamic metrics changed advertised dynamic tool count")
	}
}

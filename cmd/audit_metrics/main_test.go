// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
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

	dynamicTools := listDynamicTools(actioncatalog.FromActionMaps(routes))
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
	catalog := actioncatalog.FromActionMaps(routes)

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
		t.Fatalf("metrics = %+v, want non-zero unsearchable alias count", metrics)
	}
	if len(listDynamicTools(catalog)) != 3 {
		t.Fatal("dynamic metrics changed advertised dynamic tool count")
	}
}

// TestPrintDynamicSearchMetrics_IncludesAllSurfaces verifies the audit report
// prints dynamic index and alias rows for base, self-managed enterprise, and
// GitLab.com enterprise surfaces.
func TestPrintDynamicSearchMetrics_IncludesAllSurfaces(t *testing.T) {
	base := dynamictools.RegistryMetrics{IndexTokenCount: 1, IndexPostingCount: 2, AliasCount: 3, SearchableAliasCount: 4, UnsearchableAliasCount: 5, AmbiguousAliasCount: 6}
	enterprise := dynamictools.RegistryMetrics{IndexTokenCount: 7, IndexPostingCount: 8, AliasCount: 9, SearchableAliasCount: 10, UnsearchableAliasCount: 11, AmbiguousAliasCount: 12}
	gitLabCom := dynamictools.RegistryMetrics{IndexTokenCount: 13, IndexPostingCount: 14, AliasCount: 15, SearchableAliasCount: 16, UnsearchableAliasCount: 17, AmbiguousAliasCount: 18}

	output := captureStdout(t, func() {
		printDynamicSearchMetrics(base, enterprise, gitLabCom)
	})
	for _, want := range []string{
		"Dynamic search index tokens (base)",
		"Dynamic search index tokens (self-managed enterprise)",
		"Dynamic search index tokens (GitLab.com enterprise)",
		"Dynamic search index postings (GitLab.com enterprise)",
		"Dynamic aliases (GitLab.com enterprise)",
		"Dynamic aliases searchable (GitLab.com enterprise)",
		"Dynamic aliases unsearchable (GitLab.com enterprise)",
		"Dynamic aliases ambiguous (GitLab.com enterprise)",
		"18",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printDynamicSearchMetrics() output missing %q:\n%s", want, output)
		}
	}
}

// TestAuditEnterpriseActionSpecs_ClassifiesEnterpriseDelta verifies the audit
// separates spec-backed enterprise actions from legacy route-only actions.
func TestAuditEnterpriseActionSpecs_ClassifiesEnterpriseDelta(t *testing.T) {
	base := catalogWithActions(t, catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true})
	selfManagedEnterprise := catalogWithActions(t,
		catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_geo", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_legacy", actionName: "list"},
	)
	gitLabComEnterprise := catalogWithActions(t,
		catalogActionFixture{toolName: "gitlab_project", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_geo", actionName: "list", specBacked: true},
		catalogActionFixture{toolName: "gitlab_orbit", actionName: "status", specBacked: true},
	)

	audit := auditEnterpriseActionSpecs(base, selfManagedEnterprise, gitLabComEnterprise)
	if !slices.Equal(audit.SpecBacked, []string{"geo.list", "orbit.status"}) {
		t.Fatalf("SpecBacked = %v, want [geo.list orbit.status]", audit.SpecBacked)
	}
	if !slices.Equal(audit.LegacyRouteOnly, []string{"legacy.list"}) {
		t.Fatalf("LegacyRouteOnly = %v, want [legacy.list]", audit.LegacyRouteOnly)
	}
}

// TestAuditEnterpriseActionSpecs_RealCatalogHasNoLegacyRoutes verifies phase 7
// completion: every enterprise-only dynamic catalog action is spec-backed.
func TestAuditEnterpriseActionSpecs_RealCatalogHasNoLegacyRoutes(t *testing.T) {
	selfManagedClient := newAuditMetricsClient(t)
	gitLabComClient, err := gitlabclient.NewClientWithToken(config.DefaultGitLabURL, "audit-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken(gitlab.com) error: %v", err)
	}

	audit := auditEnterpriseActionSpecs(
		dynamicActionCatalog(selfManagedClient, false),
		dynamicActionCatalog(selfManagedClient, true),
		dynamicActionCatalog(gitLabComClient, true),
	)
	if len(audit.LegacyRouteOnly) != 0 {
		t.Fatalf("LegacyRouteOnly = %v, want none", audit.LegacyRouteOnly)
	}
	if len(audit.SpecBacked) == 0 {
		t.Fatal("SpecBacked is empty, want enterprise actions")
	}
	if !slices.Contains(audit.SpecBacked, "orbit.status") {
		t.Fatalf("SpecBacked = %v, want orbit.status", audit.SpecBacked)
	}
}

// TestPrintEnterpriseActionSpecAudit_IncludesLegacyZeroSection verifies the
// audit output includes both lists, including the explicit zero legacy state.
func TestPrintEnterpriseActionSpecAudit_IncludesLegacyZeroSection(t *testing.T) {
	output := captureStdout(t, func() {
		printEnterpriseActionSpecAudit(enterpriseActionSpecAudit{SpecBacked: []string{"geo.list"}})
	})
	for _, want := range []string{
		"Enterprise ActionSpec Audit",
		"Spec-backed enterprise actions (1)",
		"geo.list",
		"Legacy route-only enterprise actions (0)",
		"none",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printEnterpriseActionSpecAudit() output missing %q:\n%s", want, output)
		}
	}
}

type catalogActionFixture struct {
	toolName   string
	actionName string
	specBacked bool
}

func catalogWithActions(t *testing.T, fixtures ...catalogActionFixture) *actioncatalog.Catalog {
	t.Helper()
	groups := map[string]actioncatalog.Group{}
	for _, fixture := range fixtures {
		group := groups[fixture.toolName]
		if group.ToolName == "" {
			group = actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: fixture.toolName})
		}
		group.SetAction(actioncatalog.Action{Name: fixture.actionName, SpecBacked: fixture.specBacked})
		groups[fixture.toolName] = group
	}
	catalog := actioncatalog.NewCatalog()
	for _, group := range groups {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error: %v", group.ToolName, err)
		}
	}
	return catalog
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })

	fn()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return string(output)
}

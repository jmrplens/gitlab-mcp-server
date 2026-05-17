// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on registered meta-schema resources.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
)

// newAuditTokensClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for audit_tokens tests.
func newAuditTokensClient(t *testing.T) *gitlabclient.Client {
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

// TestMeasureResources_SeparatesMetaSchema verifies the token audit measures
// individual-mode resources separately from the additional meta-schema catalog
// resources that only appear when meta-tools are enabled.
func TestMeasureResources_SeparatesMetaSchema(t *testing.T) {
	client := newAuditTokensClient(t)
	individualTokens := measureResources(client, nil)
	metaTokens := measureResources(client, buildMetaActionMaps(client, false))
	if individualTokens <= 0 {
		t.Fatalf("measureResources(includeMetaSchema=false) = %d, want positive token estimate", individualTokens)
	}
	if metaTokens <= individualTokens {
		t.Fatalf("measureResources(includeMetaSchema=true) = %d, want greater than individual %d", metaTokens, individualTokens)
	}
}

// TestMeasureResourcesWithOptions_MinimalCandidate verifies the dynamic-minimal
// candidate keeps a measurable project-discovery resource while dropping the
// heavier optional resource groups.
func TestMeasureResourcesWithOptions_MinimalCandidate(t *testing.T) {
	client := newAuditTokensClient(t)
	fullDynamicTokens := measureResources(client, buildMetaActionMaps(client, false))
	minimalTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{WorkspaceRoots: true})

	if minimalTokens <= 0 {
		t.Fatalf("minimal resource tokens = %d, want positive workspace_roots estimate", minimalTokens)
	}
	if minimalTokens >= fullDynamicTokens {
		t.Fatalf("minimal resource tokens = %d, want less than full dynamic %d", minimalTokens, fullDynamicTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the three public tools backed by the canonical action catalog.
func TestListDynamicTools_ExposesLowTokenSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	toolList := listDynamicTools(actioncatalog.FromActionMaps(routes))
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_tool,gitlab_find_action" {
		t.Fatalf("dynamic tools = %q, want find/execute", got)
	}
}

// TestListDynamic2Tools_ExposesFindExecuteSurface verifies the token audit can
// measure the explicit two-tool alias independently from dynamic-3.
func TestListDynamic2Tools_ExposesFindExecuteSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	catalog := actioncatalog.FromActionMaps(routes)
	dynamic3 := listDynamic3Tools(catalog)
	dynamic2 := listDynamic2Tools(catalog)
	names := make([]string, 0, len(dynamic2))
	for _, tool := range dynamic2 {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_tool,gitlab_find_action" {
		t.Fatalf("dynamic-2 tools = %q, want find/execute", got)
	}
	if len(dynamic2) >= len(dynamic3) {
		t.Fatalf("dynamic-2 tool count = %d, want less than dynamic-3 count %d", len(dynamic2), len(dynamic3))
	}
}

// TestListDynamic3Tools_ExposesSearchDescribeExecuteSurface verifies the token
// audit can still measure the explicit three-tool dynamic variant.
func TestListDynamic3Tools_ExposesSearchDescribeExecuteSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	toolList := listDynamic3Tools(actioncatalog.FromActionMaps(routes))
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_describe_tools,gitlab_execute_tool,gitlab_search_tools" {
		t.Fatalf("dynamic-3 tools = %q, want search/describe/execute", got)
	}
}

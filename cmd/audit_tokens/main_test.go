// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on the surface-aware tool manifest resources.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
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

// TestMeasureResources_IncludesToolManifest verifies the token audit measures
// the surface-aware tool manifest in addition to static resources.
func TestMeasureResources_IncludesToolManifest(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	manifestTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest:   true,
		ToolSurface:    config.ToolSurfaceDynamic,
		ToolList:       dynamicTools,
		ToolCatalog:    dynamicCatalog,
		WorkspaceRoots: true,
	})
	rootOnlyTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{WorkspaceRoots: true})
	if rootOnlyTokens <= 0 {
		t.Fatalf("workspace root tokens = %d, want positive token estimate", rootOnlyTokens)
	}
	if manifestTokens <= rootOnlyTokens {
		t.Fatalf("manifest resource tokens = %d, want greater than roots-only %d", manifestTokens, rootOnlyTokens)
	}
}

// TestMeasureResourcesWithOptions_MinimalCandidate verifies the dynamic-minimal
// candidate keeps the tool manifest and workspace roots while dropping the
// heavier optional resource groups.
func TestMeasureResourcesWithOptions_MinimalCandidate(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	fullDynamicTokens := measureResources(client, routes, dynamicCatalog, dynamicTools, config.ToolSurfaceDynamic)
	minimalTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest:   true,
		ToolSurface:    config.ToolSurfaceDynamic,
		ToolList:       dynamicTools,
		ToolCatalog:    dynamicCatalog,
		WorkspaceRoots: true,
	})

	if minimalTokens <= 0 {
		t.Fatalf("minimal resource tokens = %d, want positive workspace_roots estimate", minimalTokens)
	}
	if minimalTokens >= fullDynamicTokens {
		t.Fatalf("minimal resource tokens = %d, want less than full dynamic %d", minimalTokens, fullDynamicTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the find/execute tools backed by the canonical action catalog.
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

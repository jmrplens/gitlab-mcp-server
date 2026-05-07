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
	metaTokens := measureResources(client, captureMetaRoutes(client, false))
	if individualTokens <= 0 {
		t.Fatalf("measureResources(includeMetaSchema=false) = %d, want positive token estimate", individualTokens)
	}
	if metaTokens <= individualTokens {
		t.Fatalf("measureResources(includeMetaSchema=true) = %d, want greater than individual %d", metaTokens, individualTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the three public tools backed by the hidden route registry.
func TestListDynamicTools_ExposesLowTokenSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := captureMetaRoutes(client, false)
	if countActions(routes) == 0 {
		t.Fatal("captureMetaRoutes() returned no hidden actions")
	}

	toolList := listDynamicTools(routes)
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_describe_tools,gitlab_execute_tool,gitlab_search_tools" {
		t.Fatalf("dynamic tools = %q, want search/describe/execute", got)
	}
}

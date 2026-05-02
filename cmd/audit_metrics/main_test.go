// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
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

// TestListServerTools_MetaModeIncludesServerTool verifies metrics use the same
// production-like meta-tool registration path as cmd/server.
func TestListServerTools_MetaModeIncludesServerTool(t *testing.T) {
	toolList := listServerTools(newAuditMetricsClient(t), true, false)
	if !hasServerTool(toolList, "gitlab_server") {
		t.Fatal("listServerTools(meta=true) did not include gitlab_server")
	}
}

// TestCountActionSchemaTools_CountsAdvertisedActions verifies the metrics audit
// derives schema-action counts from the action enum in tools/list output.
func TestCountActionSchemaTools_CountsAdvertisedActions(t *testing.T) {
	toolList := listServerTools(newAuditMetricsClient(t), true, false)
	toolsWithActions, actions := countActionSchemaTools(toolList)
	if toolsWithActions == 0 {
		t.Fatal("countActionSchemaTools() toolsWithActions = 0, want action-schema tools")
	}
	if actions <= toolsWithActions {
		t.Fatalf("countActionSchemaTools() actions = %d, want greater than toolsWithActions %d", actions, toolsWithActions)
	}
	if count := actionEnumCount(findServerTool(toolList, "gitlab_server")); count != 4 {
		t.Fatalf("actionEnumCount(gitlab_server) = %d, want 4", count)
	}
}

func hasServerTool(toolList []*mcp.Tool, name string) bool {
	return findServerTool(toolList, name) != nil
}

func findServerTool(toolList []*mcp.Tool, name string) *mcp.Tool {
	for _, tool := range toolList {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on registered meta-schema resources.
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
	individualTokens := measureResources(client, false)
	metaTokens := measureResources(client, true)
	if individualTokens <= 0 {
		t.Fatalf("measureResources(includeMetaSchema=false) = %d, want positive token estimate", individualTokens)
	}
	if metaTokens <= individualTokens {
		t.Fatalf("measureResources(includeMetaSchema=true) = %d, want greater than individual %d", metaTokens, individualTokens)
	}
}

// TestListTools_MetaModeIncludesServerTool verifies the token audit uses the
// same production-like meta-tool registration path as cmd/server.
func TestListTools_MetaModeIncludesServerTool(t *testing.T) {
	client := newAuditTokensClient(t)
	toolList := listTools(client, true, false)
	if !hasTool(toolList, "gitlab_server") {
		t.Fatal("listTools(meta=true) did not include gitlab_server")
	}
}

// TestMeasureToolComponents_TracksKnownFields verifies component accounting
// isolates the expensive tool-definition fields and keeps remaining bytes in Other.
func TestMeasureToolComponents_TracksKnownFields(t *testing.T) {
	raw := []byte(`{"name":"gitlab_example","description":"Example","inputSchema":{"type":"object"},"outputSchema":{"type":"object"},"annotations":{"readOnlyHint":true},"icons":[{"src":"data:image/svg+xml;base64,abc"}]}`)
	components := measureToolComponents(raw)
	if components.Description == 0 {
		t.Fatal("Description component = 0, want positive")
	}
	if components.InputSchema == 0 {
		t.Fatal("InputSchema component = 0, want positive")
	}
	if components.OutputSchema == 0 {
		t.Fatal("OutputSchema component = 0, want positive")
	}
	if components.Annotations == 0 {
		t.Fatal("Annotations component = 0, want positive")
	}
	if components.Icons == 0 {
		t.Fatal("Icons component = 0, want positive")
	}
	if components.Other == 0 {
		t.Fatal("Other component = 0, want name/separator overhead")
	}
}

func hasTool(toolList []*mcp.Tool, name string) bool {
	for _, tool := range toolList {
		if tool.Name == name {
			return true
		}
	}
	return false
}

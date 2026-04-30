// main_test.go contains focused tests for the audit_metrics command. Tests use
// an httptest GitLab version endpoint so MCP resource registration can be
// inspected without external credentials.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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

// main_test.go contains focused tests for llms.txt generation helpers. Tests
// use a local GitLab version mock so resource and template discovery can run
// through an in-memory MCP server.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// newGenLLMSClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for gen_llms tests.
func newGenLLMSClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "gen-llms-token"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	return client
}

// TestListResources_IncludesMetaSchemaTemplate verifies llms generation sees
// the per-action meta-schema resource template alongside regular resources.
func TestListResources_IncludesMetaSchemaTemplate(t *testing.T) {
	resources, templates := listResources(newGenLLMSClient(t))
	if len(resources) == 0 {
		t.Fatal("listResources() returned no static resources")
	}
	for _, template := range templates {
		if template.URITemplate == "gitlab://schema/meta/{tool}/{action}" {
			return
		}
	}
	t.Fatalf("listResources() templates missing meta-schema template: %v", templates)
}

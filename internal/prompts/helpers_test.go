// helpers_test.go provides shared test utilities for the prompts package,
// including mock GitLab client construction, JSON response helpers, and
// in-memory MCP session setup used across all prompt test files.
package prompts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// newTestClient creates a GitLab client pointed at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		GitLabURL:     srv.URL,
		GitLabToken:   "test-token",
		SkipTLSVerify: false,
	}

	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test gitlab client: %v", err)
	}

	return client
}

// respondJSON writes a JSON response with the given status code and body.
func respondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// newMCPSession creates an in-memory MCP client session connected to a server
// that has all prompts registered against the given mock GitLab handler.
func newMCPSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	client := newTestClient(t, handler)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// EmbedToggle is the signature of the toolutil.EnableEmbeddedResources
// setter, declared here so testutil can drive the toggle without importing
// toolutil (which would create an import cycle through other packages).
type EmbedToggle func(bool)

// RegisterFn is the per-package tool registration callback. Every domain
// sub-package exposes a RegisterTools(server, client) function with this
// shape; tests pass it to NewEmbedTestSession so the helper does not have
// to import any specific tool sub-package.
type RegisterFn func(server *mcp.Server, client *gitlabclient.Client)

// NewEmbedTestSession bootstraps an in-memory MCP session for embed-resource
// integration tests. It builds a mock GitLab client backed by handler,
// instantiates an MCP server, invokes register to wire the package's tools,
// connects an in-memory client, and returns the live session. The session
// is closed via t.Cleanup so callers do not need to track it.
func NewEmbedTestSession(t *testing.T, handler http.Handler, register RegisterFn) (*mcp.ClientSession, context.Context) {
	t.Helper()
	client := NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

// AssertEmbeddedResource invokes the named tool with args twice: first with
// the embed toggle enabled (expecting an *mcp.EmbeddedResource block whose
// URI matches wantURI and MIME type is application/json), then with the
// toggle disabled (expecting no EmbeddedResource blocks). The toggle is
// always restored to enabled (the production default) on test exit.
//
//nolint:revive // *testing.T is conventionally the first parameter for test helpers.
func AssertEmbeddedResource(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any, wantURI string, toggle EmbedToggle) {
	t.Helper()
	t.Run("enabled by default", func(t *testing.T) {
		toggle(true)
		t.Cleanup(func() { toggle(true) })
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", name, err)
		}
		var found *mcp.EmbeddedResource
		for _, c := range result.Content {
			if er, ok := c.(*mcp.EmbeddedResource); ok {
				found = er
				break
			}
		}
		if found == nil || found.Resource == nil {
			t.Fatalf("expected EmbeddedResource for %s, got %d blocks", name, len(result.Content))
		}
		if found.Resource.URI != wantURI {
			t.Errorf("URI = %q, want %q", found.Resource.URI, wantURI)
		}
		if found.Resource.MIMEType != "application/json" {
			t.Errorf("MIMEType = %q, want application/json", found.Resource.MIMEType)
		}
		if found.Resource.Text == "" {
			t.Error("Text is empty, want JSON payload")
		}
	})
	t.Run("disabled produces no embed", func(t *testing.T) {
		toggle(false)
		t.Cleanup(func() { toggle(true) })
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", name, err)
		}
		for _, c := range result.Content {
			if _, ok := c.(*mcp.EmbeddedResource); ok {
				t.Fatalf("expected no EmbeddedResource when disabled (tool=%s)", name)
			}
		}
	})
}

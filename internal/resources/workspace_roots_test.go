// workspace_roots_test.go contains unit tests for the workspace_roots MCP
// resource registered in workspace_roots.go.
//
// Tests use an in-memory MCP transport to register the resource, connect
// a client, and read the resource. Coverage includes populated roots and
// empty root lists.

package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/roots"
)

// newWorkspaceRootsMCPSession creates an in-memory MCP session with only the
// workspace_roots resource registered against the given roots.Manager.
func newWorkspaceRootsMCPSession(t *testing.T, mgr *roots.Manager) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterWorkspaceRoots(server, mgr)

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

// TestWorkspaceRootsResource_WithRoots verifies that the workspace_roots
// resource returns cached workspace roots and the discovery hint.
func TestWorkspaceRootsResource_WithRoots(t *testing.T) {
	mgr := roots.NewManager()
	// Inject roots directly via exported helper for testability
	mgr.SetRootsForTest([]*mcp.Root{
		{URI: "file:///home/user/my-project", Name: "my-project"},
		{URI: "file:///home/user/other-repo", Name: "other-repo"},
	})

	session := newWorkspaceRootsMCPSession(t, mgr)
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://workspace/roots",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	var out WorkspaceRootsOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &out); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(out.Roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(out.Roots))
	}
	if out.Roots[0].URI != "file:///home/user/my-project" {
		t.Errorf("root[0].URI = %q, want %q", out.Roots[0].URI, "file:///home/user/my-project")
	}
	if out.Roots[0].Name != "my-project" {
		t.Errorf("root[0].Name = %q, want %q", out.Roots[0].Name, "my-project")
	}
	if out.Roots[1].URI != "file:///home/user/other-repo" {
		t.Errorf("root[1].URI = %q, want %q", out.Roots[1].URI, "file:///home/user/other-repo")
	}
	if out.Hint == "" {
		t.Error("Hint should not be empty")
	}
}

// TestWorkspaceRootsResource_Empty verifies that the workspace_roots resource
// returns an empty list and the discovery hint when no roots are cached.
func TestWorkspaceRootsResource_Empty(t *testing.T) {
	mgr := roots.NewManager()

	session := newWorkspaceRootsMCPSession(t, mgr)
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://workspace/roots",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var out WorkspaceRootsOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &out); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(out.Roots) != 0 {
		t.Errorf("expected 0 roots, got %d", len(out.Roots))
	}
	if out.Hint == "" {
		t.Error("Hint should not be empty even with no roots")
	}
}

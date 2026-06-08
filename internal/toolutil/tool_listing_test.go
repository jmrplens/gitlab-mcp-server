package toolutil

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestListRegisteredTools_ReturnsTools verifies ListRegisteredTools opens an
// in-memory MCP session and returns the tools advertised by the server.
func TestListRegisteredTools_ReturnsTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddTool(&mcp.Tool{Name: "gitlab_test_tool", InputSchema: &map[string]any{"type": "object"}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})

	tools, err := ListRegisteredTools(t.Context(), server, "test-list-client")
	if err != nil {
		t.Fatalf("ListRegisteredTools() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gitlab_test_tool" {
		t.Fatalf("ListRegisteredTools() = %+v, want gitlab_test_tool", tools)
	}
}

// TestListRegisteredTools_NilServer verifies ListRegisteredTools rejects a nil
// server instead of panicking while setting up the ephemeral session.
func TestListRegisteredTools_NilServer(t *testing.T) {
	_, err := ListRegisteredTools(t.Context(), nil, "test-list-client")
	if err == nil || !strings.Contains(err.Error(), "server is nil") {
		t.Fatalf("ListRegisteredTools(nil) error = %v, want server is nil", err)
	}
}

// TestListRegisteredTools_DefaultClientName verifies that an empty client name
// is replaced with the built-in default and the list still succeeds.
func TestListRegisteredTools_DefaultClientName(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddTool(&mcp.Tool{Name: "gitlab_x", InputSchema: &map[string]any{"type": "object"}}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})

	tools, err := ListRegisteredTools(t.Context(), server, "")
	if err != nil {
		t.Fatalf("ListRegisteredTools(\"\") error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gitlab_x" {
		t.Fatalf("ListRegisteredTools() = %+v, want gitlab_x", tools)
	}
}

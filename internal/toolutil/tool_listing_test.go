package toolutil

import (
	"context"
	"errors"
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

// TestListRegisteredTools_NilServerAndCancelledContext verifies the nil-server
// guard, the default client-name fallback on the happy path, and the connect
// error branch under an already-cancelled context.
func TestListRegisteredTools_NilServerAndCancelledContext(t *testing.T) {
	if _, err := ListRegisteredTools(context.Background(), nil, ""); err == nil {
		t.Error("nil server: expected error, got nil")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	tools, err := ListRegisteredTools(context.Background(), server, "")
	if err != nil {
		t.Fatalf("default client name: error = %v", err)
	}
	if tools == nil {
		t.Log("empty server returned no tools (expected)")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, listErr := ListRegisteredTools(cancelled, mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil), "x"); listErr == nil {
		t.Error("cancelled context: expected connect/list error, got nil")
	}
}

// TestListRegisteredTools_ListToolsErrorIsWrapped verifies that a failure of
// the tools/list RPC itself (as opposed to failing to connect, which
// TestListRegisteredTools_NilServerAndCancelledContext already covers) is
// wrapped with the "list tools:" prefix. Server-side receiving middleware
// intercepts the tools/list method and returns an error deterministically —
// this is the only method affected, so connecting both sessions still
// succeeds and the failure is isolated to the ListTools RPC.
func TestListRegisteredTools_ListToolsErrorIsWrapped(t *testing.T) {
	boom := errors.New("boom: tools/list intentionally broken for test")
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				return nil, boom
			}
			return next(ctx, method, req)
		}
	})

	_, err := ListRegisteredTools(t.Context(), server, "test-list-client")
	if err == nil {
		t.Fatal("ListRegisteredTools() error = nil, want wrapped tools/list error")
	}
	if !strings.Contains(err.Error(), "list tools:") {
		t.Errorf("ListRegisteredTools() error = %q, want it prefixed with %q", err, "list tools:")
	}
}

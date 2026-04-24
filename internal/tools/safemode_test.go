// safemode_test.go contains unit tests for GITLAB_SAFE_MODE behaviour:
// WrapMutatingToolsForSafeMode intercepts mutating tools and returns a
// SafeModePreview, while read-only tools continue to call the real handler.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestWrapMutatingToolsForSafeMode_ReadOnlyToolPassesThrough verifies that
// read-only tools are not wrapped by SafeMode and still call the real handler.
func TestWrapMutatingToolsForSafeMode_ReadOnlyToolPassesThrough(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	called := false
	server.AddTool(&mcp.Tool{
		Name:        "gitlab_list_projects",
		Description: "List projects",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: &map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "real result"}},
		}, nil
	})

	wrapped := WrapMutatingToolsForSafeMode(server)
	if wrapped != 0 {
		t.Fatalf("expected 0 tools wrapped, got %d", wrapped)
	}

	result := callTool(t, server, "gitlab_list_projects", nil)
	if !called {
		t.Fatal("expected real handler to be called for read-only tool")
	}
	text := extractText(t, result)
	if text != "real result" {
		t.Fatalf("expected 'real result', got %q", text)
	}
}

// TestWrapMutatingToolsForSafeMode_MutatingToolReturnsPreview verifies that
// mutating tools return a SafeModePreview instead of executing.
func TestWrapMutatingToolsForSafeMode_MutatingToolReturnsPreview(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "gitlab_create_issue",
		Description: "Create an issue",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false},
		InputSchema: &map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("mutating handler should not be called in safe mode")
		return nil, errors.New("unreachable")
	})

	wrapped := WrapMutatingToolsForSafeMode(server)
	if wrapped != 1 {
		t.Fatalf("expected 1 tool wrapped, got %d", wrapped)
	}

	args := json.RawMessage(`{"project_id":123,"title":"Bug report"}`)
	result := callTool(t, server, "gitlab_create_issue", args)

	if result.IsError {
		t.Fatal("safe mode preview should not be an error")
	}

	var preview SafeModePreview
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		t.Fatalf("failed to unmarshal preview: %v", err)
	}
	if preview.Status != "blocked" {
		t.Errorf("expected status 'blocked', got %q", preview.Status)
	}
	if preview.Mode != "safe" {
		t.Errorf("expected mode 'safe', got %q", preview.Mode)
	}
	if preview.Tool != "gitlab_create_issue" {
		t.Errorf("expected tool 'gitlab_create_issue', got %q", preview.Tool)
	}
	if preview.Hint == "" {
		t.Error("expected non-empty hint")
	}

	var params map[string]any
	if err := json.Unmarshal(preview.Params, &params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}
	if params["project_id"] != float64(123) {
		t.Errorf("expected project_id 123, got %v", params["project_id"])
	}
}

// TestWrapMutatingToolsForSafeMode_NilAnnotations verifies that tools with nil
// annotations are treated as mutating and get wrapped.
func TestWrapMutatingToolsForSafeMode_NilAnnotations(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "gitlab_delete_project",
		Description: "Delete a project",
		InputSchema: &map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("handler should not be called")
		return nil, errors.New("unreachable")
	})

	wrapped := WrapMutatingToolsForSafeMode(server)
	if wrapped != 1 {
		t.Fatalf("expected 1 tool wrapped, got %d", wrapped)
	}

	result := callTool(t, server, "gitlab_delete_project", nil)
	var preview SafeModePreview
	if err := json.Unmarshal([]byte(extractText(t, result)), &preview); err != nil {
		t.Fatalf("failed to unmarshal preview: %v", err)
	}
	if preview.Status != "blocked" {
		t.Errorf("expected status 'blocked', got %q", preview.Status)
	}
}

// TestWrapMutatingToolsForSafeMode_MixedTools verifies that only mutating tools
// are wrapped when a mix of read-only and mutating tools are registered.
func TestWrapMutatingToolsForSafeMode_MixedTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)

	readOnlyCalled := false
	server.AddTool(&mcp.Tool{
		Name:        "gitlab_get_issue",
		Description: "Get an issue",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: &map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		readOnlyCalled = true
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "issue data"}},
		}, nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "gitlab_update_issue",
		Description: "Update an issue",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false},
		InputSchema: &map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Fatal("mutating handler should not be called")
		return nil, errors.New("unreachable")
	})

	wrapped := WrapMutatingToolsForSafeMode(server)
	if wrapped != 1 {
		t.Fatalf("expected 1 tool wrapped, got %d", wrapped)
	}

	result := callTool(t, server, "gitlab_get_issue", nil)
	if !readOnlyCalled {
		t.Fatal("read-only handler should have been called")
	}
	if extractText(t, result) != "issue data" {
		t.Fatalf("unexpected read-only result: %s", extractText(t, result))
	}

	result = callTool(t, server, "gitlab_update_issue", nil)
	var preview SafeModePreview
	if err := json.Unmarshal([]byte(extractText(t, result)), &preview); err != nil {
		t.Fatalf("failed to unmarshal preview: %v", err)
	}
	if preview.Tool != "gitlab_update_issue" {
		t.Errorf("expected tool 'gitlab_update_issue', got %q", preview.Tool)
	}
}

// callTool invokes a tool via an ephemeral in-memory MCP session and returns
// the result.
func callTool(t *testing.T, server *mcp.Server, name string, args json.RawMessage) *mcp.CallToolResult {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	sess, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-caller", Version: "0"}, nil)
	clientSess, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSess.Close() })

	result, err := clientSess.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	return result
}

// extractText returns the text from the first TextContent in a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

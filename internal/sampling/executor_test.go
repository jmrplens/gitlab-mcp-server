// executor_test.go contains unit tests for the sampling request executor.

package sampling

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServerToolExecutor_AllowedTool verifies that [ServerToolExecutor.ExecuteTool]
// dispatches to the registered handler when the tool name is in the allow list.
func TestServerToolExecutor_AllowedTool(t *testing.T) {
	var receivedName string
	var receivedArgs json.RawMessage

	handlers := map[string]mcp.ToolHandler{
		"gitlab_get_file": func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			receivedName = req.Params.Name
			receivedArgs = req.Params.Arguments
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "file content"}},
			}, nil
		},
	}

	executor := NewServerToolExecutor(nil, handlers)
	result, err := executor.ExecuteTool(context.Background(), "gitlab_get_file", map[string]any{"path": "README.md"})
	if err != nil {
		t.Fatalf("ExecuteTool() unexpected error: %v", err)
	}
	if receivedName != "gitlab_get_file" {
		t.Errorf("handler received name = %q, want %q", receivedName, "gitlab_get_file")
	}

	var args map[string]any
	if err = json.Unmarshal(receivedArgs, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["path"] != "README.md" {
		t.Errorf("args[path] = %v, want README.md", args["path"])
	}

	if len(result.Content) != 1 {
		t.Fatalf("result.Content len = %d, want 1", len(result.Content))
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); !ok || tc.Text != "file content" {
		t.Errorf("result content = %v, want 'file content'", result.Content[0])
	}
}

// TestServerToolExecutor_DisallowedTool verifies that [ServerToolExecutor.ExecuteTool]
// returns an error when the tool name is not registered.
func TestServerToolExecutor_DisallowedTool(t *testing.T) {
	handlers := map[string]mcp.ToolHandler{
		"gitlab_get_file": func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	}

	executor := NewServerToolExecutor(nil, handlers)
	_, err := executor.ExecuteTool(context.Background(), "gitlab_delete_project", map[string]any{})
	if err == nil {
		t.Fatal("expected error for disallowed tool")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("error = %v, want 'not in the allowed list'", err)
	}
}

// TestServerToolExecutor_ToolNotFound verifies that [ServerToolExecutor.ExecuteTool]
// returns an error when no handler is registered (empty handlers map).
func TestServerToolExecutor_ToolNotFound(t *testing.T) {
	executor := NewServerToolExecutor(nil, map[string]mcp.ToolHandler{})
	_, err := executor.ExecuteTool(context.Background(), "nonexistent_tool", map[string]any{})
	if err == nil {
		t.Fatal("expected error for unregistered tool")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("error = %v, want 'not in the allowed list'", err)
	}
}

// TestServerToolExecutor_HandlerError verifies that [ServerToolExecutor.ExecuteTool]
// propagates errors returned by the tool handler.
func TestServerToolExecutor_HandlerError(t *testing.T) {
	handlers := map[string]mcp.ToolHandler{
		"failing_tool": func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, &json.SyntaxError{}
		},
	}

	executor := NewServerToolExecutor(nil, handlers)
	_, err := executor.ExecuteTool(context.Background(), "failing_tool", map[string]any{})
	if err == nil {
		t.Fatal("expected error from failing handler")
	}
}

// TestServerToolExecutor_SessionAttached verifies that [ServerToolExecutor.ExecuteTool]
// attaches the session to the CallToolRequest passed to the handler.
func TestServerToolExecutor_SessionAttached(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(testImpl, nil)
	client := mcp.NewClient(testImpl, &mcp.ClientOptions{
		CreateMessageWithToolsHandler: func(_ context.Context, _ *mcp.CreateMessageWithToolsRequest) (*mcp.CreateMessageWithToolsResult, error) {
			return &mcp.CreateMessageWithToolsResult{
				Model:   testModelDefault,
				Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
			}, nil
		},
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf(fmtServerConnect, err)
	}
	defer ss.Close()

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf(fmtClientConnect, err)
	}
	defer cs.Close()

	var receivedSession *mcp.ServerSession
	handlers := map[string]mcp.ToolHandler{
		"test_tool": func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			receivedSession = req.Session
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
			}, nil
		},
	}

	executor := NewServerToolExecutor(ss, handlers)
	_, err = executor.ExecuteTool(ctx, "test_tool", map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool() unexpected error: %v", err)
	}
	if receivedSession != ss {
		t.Error("handler did not receive the expected session")
	}
}

// TestServerToolExecutor_UnmarshalableArgs covers executor.go:43-45
// (json.Marshal error when args contain an unmarshalable type like a channel).
func TestServerToolExecutor_UnmarshalableArgs(t *testing.T) {
	handlers := map[string]mcp.ToolHandler{
		"my_tool": func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	}

	executor := NewServerToolExecutor(nil, handlers)
	_, err := executor.ExecuteTool(context.Background(), "my_tool", map[string]any{
		"bad": make(chan int),
	})
	if err == nil {
		t.Fatal("expected error for unmarshalable args")
	}
	if !strings.Contains(err.Error(), "marshal args") {
		t.Errorf("error = %v, want 'marshal args' context", err)
	}
}

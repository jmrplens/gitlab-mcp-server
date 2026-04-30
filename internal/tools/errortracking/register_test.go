// register_test.go contains integration tests for the error tracking tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.
package errortracking

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_DeleteError verifies that the delete handler returns an error
// result when the GitLab API fails, covering the if-err-not-nil branch in
// the handler closure.
func TestRegisterTools_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_delete_error_tracking_client_key",
		Arguments: map[string]any{
			"project_id": "42",
			"key_id":     float64(1),
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result from delete with failing backend")
	}
}

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the gitlab_delete_error_tracking_client_key handler when the user declines.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_error_tracking_client_key",
		Arguments: map[string]any{"project_id": "p", "key_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if tc.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

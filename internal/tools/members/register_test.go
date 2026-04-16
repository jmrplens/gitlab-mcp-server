package members

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_MutationErrors covers error branches in register.go closures:
// delete (500), add (500), edit (500), get (500), inherited (500).
func TestRegisterTools_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, _ = server.Connect(ctx, st, nil)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_member_add", map[string]any{"project_id": "42", "user_id": 1, "access_level": 30}},
		{"gitlab_project_member_edit", map[string]any{"project_id": "42", "user_id": 1, "access_level": 40}},
		{"gitlab_project_member_delete", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get_inherited", map[string]any{"project_id": "42", "user_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil || !result.IsError {
				t.Errorf("expected error result from %s", tt.name)
			}
		})
	}
}

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the project member delete handler when the user declines.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
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
		Name:      "gitlab_project_member_delete",
		Arguments: map[string]any{"project_id": "42", "user_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

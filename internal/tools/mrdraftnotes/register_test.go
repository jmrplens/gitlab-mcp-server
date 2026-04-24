// register_test.go contains integration tests for the MR draft note tool
// closures in register.go. Tests cover the ConfirmAction early-return branch
// and error paths via an in-memory MCP session.

package mrdraftnotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the draft note delete handler when the user declines confirmation.
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
		Name:      "gitlab_mr_draft_note_delete",
		Arguments: map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestRegisterTools_GetNotFound_Register covers the NotFoundResult branch in the
// gitlab_mr_draft_note_get handler when the API returns 404.
func TestRegisterTools_GetNotFound_Register(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
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
		Name:      "gitlab_mr_draft_note_get",
		Arguments: map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 999},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestCreate_MissingBody covers the validation branch in Create when body is empty.
func TestCreate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "1",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

// TestUpdate_MissingBody covers the validation branch in Update when body is empty.
func TestUpdate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "1",
		MRIID:     1,
		NoteID:    1,
	})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

// TestRegisterTools_ErrorPaths covers the error branches in RegisterTools closures
// for non-destructive tools when the server returns 500.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_draft_note_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_draft_note_create", map[string]any{"project_id": "42", "mr_iid": 1, "body": "x"}},
		{"gitlab_mr_draft_note_update", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1, "body": "x"}},
		{"gitlab_mr_draft_note_publish", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1}},
		{"gitlab_mr_draft_note_publish_all", map[string]any{"project_id": "42", "mr_iid": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatal("expected IsError result for server error response")
			}
		})
	}
}

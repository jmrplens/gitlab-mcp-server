package mrnotes

import (
	"context"
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the MR note delete handler when the user declines confirmation.
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
		Name:      "gitlab_mr_note_delete",
		Arguments: map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestRegisterTools_GetNotFound covers the NotFoundResult branch in the
// gitlab_mr_note_get handler when the API returns 404.
func TestRegisterTools_GetNotFound(t *testing.T) {
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
		Name:      "gitlab_mr_note_get",
		Arguments: map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 999},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestToOutput_ResolvedByAndTimestamps covers the ResolvedBy, ResolvedAt,
// CreatedAt, and UpdatedAt branches in ToOutput when optional fields are set.
func TestToOutput_ResolvedByAndTimestamps(t *testing.T) {
	now := time.Now()
	note := &gl.Note{
		ID:   1,
		Body: "test",
		Author: gl.NoteAuthor{Username: "author"},
		ResolvedBy: gl.NoteResolvedBy{Username: "resolver"},
		ResolvedAt: &now,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	out := ToOutput(note)
	if out.ResolvedBy != "resolver" {
		t.Errorf("ResolvedBy = %q, want %q", out.ResolvedBy, "resolver")
	}
	if out.ResolvedAt == "" {
		t.Error("expected non-empty ResolvedAt")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestRegisterTools_ErrorPaths covers the error branches in RegisterTools
// closures for non-destructive tools against a 500 server.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_notes_list", map[string]any{"project_id": "1", "mr_iid": 1}},
		{"gitlab_mr_note_get", map[string]any{"project_id": "1", "mr_iid": 1, "note_id": 1}},
		{"gitlab_mr_note_create", map[string]any{"project_id": "1", "mr_iid": 1, "body": "x"}},
		{"gitlab_mr_note_update", map[string]any{"project_id": "1", "mr_iid": 1, "note_id": 1, "body": "x"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.name, err)
			}
			if !result.IsError {
				t.Errorf("CallTool(%s) expected IsError=true for 500 response", tt.name)
			}
		})
	}
}

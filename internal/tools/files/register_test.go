// register_test.go contains integration tests for the repository file tool
// closures in register.go. Tests cover the ConfirmAction early-return branch
// for the delete handler and error paths via an in-memory MCP session.
package files

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the file delete handler when the user declines confirmation.
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
		Name:      "gitlab_file_delete",
		Arguments: map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestRegisterTools_GetNotFound covers the NotFoundResult branch in the
// gitlab_file_get handler when the API returns 404.
func TestRegisterTools_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 File Not Found"}`)
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
		Name:      "gitlab_file_get",
		Arguments: map[string]any{"project_id": "42", "file_path": "nonexistent.go"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestFileCreate_OptionalFields covers the optional field branches
// (start_branch, encoding, author_email, author_name, execute_filemode)
// in the Create function.
func TestFileCreate_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"f.go","branch":"main"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "1",
		Branch:        "main",
		CommitMessage: "add",
		Content:       "data",
		StartBranch:   "dev",
		Encoding:      "text",
		AuthorEmail:   "a@b.com",
		AuthorName:    "A",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFileUpdate_OptionalFields covers the optional field branches in Update.
func TestFileUpdate_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"file_path":"f.go","branch":"main"}`)
	})
	client := testutil.NewTestClient(t, mux)
	fm := true
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:       "1",
		FilePath:        "f.go",
		Branch:          "main",
		CommitMessage:   "up",
		Content:         "data",
		StartBranch:     "dev",
		Encoding:        "text",
		AuthorEmail:     "a@b.com",
		AuthorName:      "A",
		LastCommitID:    "abc",
		ExecuteFilemode: &fm,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFileDelete_OptionalFields covers the optional field branches in Delete.
func TestFileDelete_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "1",
		FilePath:      "f.go",
		Branch:        "main",
		CommitMessage: "del",
		StartBranch:   "dev",
		AuthorEmail:   "a@b.com",
		AuthorName:    "A",
		LastCommitID:  "abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

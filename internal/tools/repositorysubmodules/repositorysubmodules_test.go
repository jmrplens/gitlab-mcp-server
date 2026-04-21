// repositorysubmodules_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package repositorysubmodules

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestUpdate_Success verifies that Update handles the success scenario correctly.
func TestUpdate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": "abc123def456",
			"short_id": "abc123d",
			"title": "Update submodule lib to abc123",
			"author_name": "Dev User",
			"author_email": "dev@example.com",
			"message": "Update submodule lib to abc123",
			"created_at": "2026-01-15T10:30:00Z",
			"committed_date": "2026-01-15T10:30:00Z"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "lib/mylib",
		Branch:    "main",
		CommitSHA: "abc123def456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "abc123def456" {
		t.Errorf("expected ID 'abc123def456', got %q", out.ID)
	}
	if out.ShortID != "abc123d" {
		t.Errorf("expected short_id 'abc123d', got %q", out.ShortID)
	}
	if out.AuthorName != "Dev User" {
		t.Errorf("expected author_name 'Dev User', got %q", out.AuthorName)
	}
}

// TestUpdate_WithCommitMessage verifies that Update handles the with commit message scenario correctly.
func TestUpdate_WithCommitMessage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": "abc123",
			"short_id": "abc",
			"title": "Custom message",
			"author_name": "Dev",
			"author_email": "dev@ex.com",
			"message": "Custom message"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID:     "42",
		Submodule:     "lib/mylib",
		Branch:        "main",
		CommitSHA:     "abc123",
		CommitMessage: "Custom message",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Title != "Custom message" {
		t.Errorf("expected title 'Custom message', got %q", out.Title)
	}
}

// TestUpdate_Error verifies that Update handles the error scenario correctly.
func TestUpdate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"error"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Update(t.Context(), client, UpdateInput{
		ProjectID: "42",
		Submodule: "lib/mylib",
		Branch:    "main",
		CommitSHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatUpdateMarkdown verifies the behavior of format update markdown.
func TestFormatUpdateMarkdown(t *testing.T) {
	r := FormatUpdateMarkdown(UpdateOutput{
		ID:         "abc123",
		ShortID:    "abc",
		Title:      "Update submodule",
		AuthorName: "Dev",
	})
	if r == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatUpdateMarkdown_Content verifies that FormatUpdateMarkdown handles the content scenario correctly.
func TestFormatUpdateMarkdown_Content(t *testing.T) {
	out := UpdateOutput{
		ID:          "abc123def456",
		ShortID:     "abc123d",
		Title:       "Update lib",
		AuthorName:  "Alice",
		AuthorEmail: "alice@example.com",
		Message:     "Bump lib to v2",
	}
	r := FormatUpdateMarkdown(out)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "abc123d") {
		t.Error("expected short ID")
	}
	if !strings.Contains(tc.Text, "Alice") {
		t.Error("expected author")
	}
}

// TestUpdate_CancelledContext verifies that Update handles the cancelled context scenario correctly.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestUpdate_EmptyProjectID verifies that Update handles the empty project i d scenario correctly.
func TestUpdate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Update(t.Context(), client, UpdateInput{Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools handles the no panic scenario correctly.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies that RegisterTools handles the call through m c p scenario correctly.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":"c1","short_id":"c1","title":"t","author_name":"A","author_email":"a@t.com","message":"m"}`)
			return
		}
		http.NotFound(w, r)
	}))

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
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_update_repository_submodule",
		Arguments: map[string]any{"project_id": "42", "submodule": "lib", "branch": "main", "commit_sha": "abc"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}

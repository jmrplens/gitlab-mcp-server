// repositorysubmodules_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package repositorysubmodules

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestUpdate_Success verifies Update when success.
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

// TestUpdate_WithCommitMessage verifies Update when with commit message.
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

// TestUpdate_Error verifies Update when error.
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

// TestFormatUpdateMarkdown verifies FormatUpdateMarkdown.
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

// TestFormatUpdateMarkdown_Content verifies FormatUpdateMarkdown when content.
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

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestUpdate_EmptyProjectID verifies Update when empty project ID.
func TestUpdate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Update(t.Context(), client, UpdateInput{Submodule: "lib", Branch: "main", CommitSHA: "abc"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for repository submodule actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(byTool))
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "repositorysubmodules" {
			t.Errorf("OwnerPackage for %s = %q, want repositorysubmodules", spec.Name, spec.OwnerPackage)
		}
	}
	for _, name := range []string{"gitlab_list_repository_submodules", "gitlab_read_repository_submodule_file"} {
		if !byTool[name].ReadOnly || !byTool[name].Idempotent {
			t.Errorf("%s should be read-only and idempotent", name)
		}
	}
	if byTool["gitlab_update_repository_submodule"].ReadOnly || !byTool["gitlab_update_repository_submodule"].Idempotent {
		t.Error("update submodule action should be mutating and idempotent")
	}
}

// TestActionSpecs_UpdateRoute verifies that the update route can be called directly.
func TestActionSpecs_UpdateRoute(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":"c1","short_id":"c1","title":"t","author_name":"A","author_email":"a@t.com","message":"m"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_update_repository_submodule"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "submodule": "lib", "branch": "main", "commit_sha": "abc"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// repositorySubmoduleSpecsByTool supports repository submodule specs by tool assertions in repositorysubmodules tests.
func repositorySubmoduleSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

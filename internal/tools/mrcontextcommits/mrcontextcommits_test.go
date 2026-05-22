// mrcontextcommits_test.go contains unit tests for the merge request context commit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrcontextcommits

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// fmtUnexpMethod identifies the fmt unexp method constant used by this package.
const fmtUnexpMethod = "unexpected method: %s"

// pathMRContextCommits identifies the path MR context commits constant used by this package.
const pathMRContextCommits = "/api/v4/projects/1/merge_requests/10/context_commits"

// testCommitSHA identifies the test commit SHA constant used by this package.
const testCommitSHA = "abc123"

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com"},
			{"id":"def456","short_id":"def4","title":"Second commit","author_name":"Dev2","author_email":"dev2@test.com"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(out.Commits))
	}
	if out.Commits[0].ID != testCommitSHA {
		t.Errorf("expected ID abc123, got %s", out.Commits[0].ID)
	}
	if out.Commits[1].Title != "Second commit" {
		t.Errorf("expected title 'Second commit', got %s", out.Commits[1].Title)
	}
}

// TestList_Empty verifies List when empty.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(out.Commits))
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if out.Commits[0].ID != testCommitSHA {
		t.Errorf("expected ID abc123, got %s", out.Commits[0].ID)
	}
}

// TestCreate_Error verifies Create when error.
func TestCreate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Commits: []CommitItem{
			{ID: "abc123", ShortID: "abc1", Title: "Fix bug", AuthorName: "Dev"},
			{ID: "def456", ShortID: "def4", Title: "Add feature", AuthorName: "Dev2"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// ---------------------------------------------------------------------------
// MRIID required-field validation
// ---------------------------------------------------------------------------.

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

// TestMRIIDRequired_Validation covers MRIIDRequired with table-driven subtests for validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when merge_request_iid is missing")
	}))

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error {
			_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
			return err
		}},
		{"Create", func() error {
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Commits: []string{"abc"}})
			return err
		}},
		{"Delete", func() error {
			return Delete(context.Background(), client, DeleteInput{ProjectID: "42", Commits: []string{"abc"}})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "merge_request_iid")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — CreatedAt branch + canceled context
// ---------------------------------------------------------------------------.

// TestList_WithCreatedAt verifies List when with created at.
func TestList_WithCreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"aaa111","short_id":"aaa1","title":"Commit with date","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-06-15T10:30:00Z"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if out.Commits[0].CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "1", MergeRequest: 5})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Create — CreatedAt branch + canceled context
// ---------------------------------------------------------------------------.

// TestCreate_WithCreatedAt verifies Create when with created at.
func TestCreate_WithCreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"bbb222","short_id":"bbb2","title":"Created with date","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-07-01T08:00:00Z"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"bbb222"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if out.Commits[0].CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Create(ctx, client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"abc123"},
	})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Delete — canceled context
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(ctx, client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"abc123"},
	})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — content validation
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_ContentValidation verifies FormatListMarkdown when content validation.
func TestFormatListMarkdown_ContentValidation(t *testing.T) {
	out := ListOutput{
		Commits: []CommitItem{
			{ID: "abc123", ShortID: "abc1", Title: "First | commit", AuthorName: "Dev"},
			{ID: "def456", ShortID: "def4", Title: "Second commit", AuthorName: "Dev2"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	md := tc.Text

	for _, want := range []string{
		"## MR Context Commits (2)",
		"| SHA | Title | Author |",
		"| abc1 |",
		"| def4 |",
		"Dev2",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	// Pipe in title should be escaped
	if strings.Contains(md, "First | commit") {
		t.Errorf("pipe in title should be escaped:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route execution for all context commit tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for MR context commit actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := newMRContextCommitsSpecsByTool(t)

	if len(byTool) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(byTool))
	}
	if !byTool["gitlab_list_mr_context_commits"].ReadOnly || !byTool["gitlab_list_mr_context_commits"].Idempotent {
		t.Error("list action should be read-only and idempotent")
	}
	if !byTool["gitlab_delete_mr_context_commits"].Destructive || !byTool["gitlab_delete_mr_context_commits"].Idempotent {
		t.Error("delete action should be destructive and idempotent")
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "mrcontextcommits" {
			t.Errorf("OwnerPackage for %s = %q, want mrcontextcommits", spec.Name, spec.OwnerPackage)
		}
	}
}

// TestActionSpecs_CallAllRoutes validates all MR context commit routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newMRContextCommitsSpecsByTool(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10)}},
		{"gitlab_create_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10), "commits": []any{"abc123"}}},
		{"gitlab_delete_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10), "commits": []any{"abc123"}}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newMRContextCommitsSpecsByTool constructs MR context commits specs by tool test fixtures.
func newMRContextCommitsSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	const commitsJSON = `[
		{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-06-15T10:30:00Z"}
	]`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/context_commits"):
			testutil.RespondJSON(w, http.StatusOK, commitsJSON)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/context_commits"):
			testutil.RespondJSON(w, http.StatusOK, commitsJSON)

		case r.Method == http.MethodDelete && strings.HasSuffix(path, "/context_commits"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))

	specs := ActionSpecs(client)
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestList_EmptyProjectID verifies that List returns an error when project_id
// is empty, covering the missed validation branch.
func TestList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "", MergeRequest: 1})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestCreate_EmptyProjectID verifies that Create returns an error when
// project_id is empty.
func TestCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "", MergeRequest: 1, Commits: []string{"abc"}})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestDelete_EmptyProjectID verifies that Delete returns an error when
// project_id is empty.
func TestDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "", MergeRequest: 1, Commits: []string{"abc"}})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestActionSpecs_DeleteError validates the delete route error path against a 403 backend.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	byTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		byTool[spec.IndividualTool.Name] = spec
	}

	_, err := byTool["gitlab_delete_mr_context_commits"].Route.Handler(t.Context(), map[string]any{"project_id": "p", "merge_request_iid": int64(1), "commits": []any{"abc"}})
	if err == nil {
		t.Error("expected delete route error")
	}
}

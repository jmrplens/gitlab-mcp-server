// todos_test.go contains unit tests for the to-do MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package todos

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// fmtUnexpPath identifies the fmt unexp path constant used by this package.
	fmtUnexpPath = "unexpected path: %s"
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// pathTodos identifies the path todos constant used by this package.
	pathTodos = "/api/v4/todos"
	// pathTodoMarkDone identifies the path todo mark done constant used by this package.
	pathTodoMarkDone = "/api/v4/todos/1/mark_as_done"
	// pathTodoMarkAll identifies the path todo mark all constant used by this package.
	pathTodoMarkAll = "/api/v4/todos/mark_as_done"
)

// todoList tests.

// TestTodoList_Success verifies TodoList when success.
func TestTodoList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodos {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{
				"id": 1,
				"action_name": "assigned",
				"target_type": "Issue",
				"target": {"title": "Fix bug"},
				"target_url": "https://gitlab.example.com/proj/-/issues/1",
				"body": "Fix the login bug",
				"state": "pending",
				"project": {"name": "my-project"},
				"author": {"username": "alice"},
				"created_at": "2026-01-15T10:00:00Z"
			},
			{
				"id": 2,
				"action_name": "mentioned",
				"target_type": "MergeRequest",
				"target": {"title": "Add feature"},
				"target_url": "https://gitlab.example.com/proj/-/merge_requests/5",
				"body": "@bob check this",
				"state": "pending",
				"project": {"name": "my-project"},
				"author": {"username": "charlie"},
				"created_at": "2026-01-16T12:00:00Z"
			}
		]`, testutil.PaginationHeaders{Page: "1", Total: "2", TotalPages: "1", PerPage: "20"})
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(out.Todos))
	}
	if out.Todos[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Todos[0].ID)
	}
	if out.Todos[0].ActionName != "assigned" {
		t.Errorf("expected action assigned, got %s", out.Todos[0].ActionName)
	}
	if out.Todos[0].TargetTitle != "Fix bug" {
		t.Errorf("expected target title 'Fix bug', got %s", out.Todos[0].TargetTitle)
	}
	if out.Todos[0].ProjectName != "my-project" {
		t.Errorf("expected project 'my-project', got %s", out.Todos[0].ProjectName)
	}
	if out.Todos[0].AuthorName != "alice" {
		t.Errorf("expected author 'alice', got %s", out.Todos[0].AuthorName)
	}
	if out.Pagination.TotalItems != 2 {
		t.Errorf("expected 2 total items, got %d", out.Pagination.TotalItems)
	}
}

// TestTodoList_WithFilters verifies TodoList when with filters.
func TestTodoList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "assigned" {
			t.Errorf("expected action filter 'assigned', got %s", r.URL.Query().Get("action"))
		}
		if r.URL.Query().Get("state") != "pending" {
			t.Errorf("expected state filter 'pending', got %s", r.URL.Query().Get("state"))
		}
		if r.URL.Query().Get("type") != "Issue" {
			t.Errorf("expected type filter 'Issue', got %s", r.URL.Query().Get("type"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", Total: "0", TotalPages: "1", PerPage: "20"})
	}))

	out, err := List(context.Background(), client, ListInput{
		Action: "assigned",
		State:  "pending",
		Type:   "Issue",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Todos) != 0 {
		t.Fatalf("expected 0 todos, got %d", len(out.Todos))
	}
}

// TestTodoListServer_Error verifies TodoListServer when error.
func TestTodoListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTodoList_CancelledContext verifies TodoList when cancelled context.
func TestTodoList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// todoMarkDone tests.

// TestTodoMarkDone_Success verifies TodoMarkDone when success.
func TestTodoMarkDone_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodoMarkDone {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	out, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestTodoMark_DoneZeroID verifies TodoMark when done zero ID.
func TestTodoMark_DoneZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	_, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 0})
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
}

// TestTodoMarkDone_NotFound verifies TodoMarkDone when not found.
func TestTodoMarkDone_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Todo Not Found"}`)
	}))

	_, err := MarkDone(context.Background(), client, MarkDoneInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

// TestTodoMarkDone_CancelledContext verifies TodoMarkDone when cancelled context.
func TestTodoMarkDone_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := MarkDone(ctx, client, MarkDoneInput{ID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// todoMarkAllDone tests.

// TestTodoMarkAllDone_Success verifies TodoMarkAllDone when success.
func TestTodoMarkAllDone_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathTodoMarkAll {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	out, err := MarkAllDone(context.Background(), client, MarkAllDoneInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestTodoMarkAllDoneServer_Error verifies TodoMarkAllDoneServer when error.
func TestTodoMarkAllDoneServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := MarkAllDone(context.Background(), client, MarkAllDoneInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTodoMarkAllDone_CancelledContext verifies TodoMarkAllDone when cancelled context.
func TestTodoMarkAllDone_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := MarkAllDone(ctx, client, MarkAllDoneInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedNonNilResult identifies the err expected non nil result constant used by this package.
const errExpectedNonNilResult = "expected non-nil result"

// ---------------------------------------------------------------------------
// List with all filter params
// ---------------------------------------------------------------------------.

// TestTodoList_AllFilters verifies TodoList when all filters.
func TestTodoList_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("author_id") != "5" {
			t.Errorf("expected author_id=5, got %q", q.Get("author_id"))
		}
		if q.Get("project_id") != "10" {
			t.Errorf("expected project_id=10, got %q", q.Get("project_id"))
		}
		if q.Get("group_id") != "3" {
			t.Errorf("expected group_id=3, got %q", q.Get("group_id"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", Total: "0", TotalPages: "1", PerPage: "20"})
	}))

	_, err := List(context.Background(), client, ListInput{
		Action:          "assigned",
		AuthorID:        5,
		ProjectID:       10,
		GroupID:         3,
		State:           "pending",
		Type:            "Issue",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 30},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// toOutput with nil fields
// ---------------------------------------------------------------------------.

// TestToOutput_NilTargetProjectAuthorCreatedAt verifies ToOutput when nil target project author created at.
func TestToOutput_NilTargetProjectAuthorCreatedAt(t *testing.T) {
	todo := todoWithNils()
	out := toOutput(&todo)
	if out.TargetTitle != "" {
		t.Errorf("expected empty TargetTitle, got %q", out.TargetTitle)
	}
	if out.ProjectName != "" {
		t.Errorf("expected empty ProjectName, got %q", out.ProjectName)
	}
	if out.AuthorName != "" {
		t.Errorf("expected empty AuthorName, got %q", out.AuthorName)
	}
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
}

// todoWithNils converts the GitLab API response to the tool output format.
func todoWithNils() gl.Todo {
	return gl.Todo{
		ID:         99,
		ActionName: "assigned",
		TargetType: "Issue",
		TargetURL:  "https://x",
		Body:       "body",
		State:      "pending",
		Target:     nil,
		Project:    nil,
		Author:     nil,
		CreatedAt:  nil,
	}
}

// ---------------------------------------------------------------------------
// Markdown formatter tests
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdownString_Full verifies FormatOutputMarkdownString when full.
func TestFormatOutputMarkdownString_Full(t *testing.T) {
	s := FormatOutputMarkdownString(Output{
		ID: 1, ActionName: "assigned", TargetTitle: "Fix", TargetType: "Issue",
		State: "pending", ProjectName: "proj", AuthorName: "alice", CreatedAt: "2026-01-01",
		TargetURL: "https://x", Body: "Some body",
	})
	if !strings.Contains(s, "To-Do #1") {
		t.Error("expected To-Do header")
	}
	if !strings.Contains(s, "alice") {
		t.Error("expected author")
	}
	if !strings.Contains(s, "Some body") {
		t.Error("expected body")
	}
}

// TestFormatOutputMarkdownString_Minimal verifies FormatOutputMarkdownString when minimal.
func TestFormatOutputMarkdownString_Minimal(t *testing.T) {
	s := FormatOutputMarkdownString(Output{ID: 2, ActionName: "mentioned", State: "done"})
	if !strings.Contains(s, "To-Do #2") {
		t.Error("expected To-Do header")
	}
	if strings.Contains(s, "**Author:**") {
		t.Error("should skip author when empty")
	}
}

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
func TestFormatOutputMarkdown(t *testing.T) {
	r := FormatOutputMarkdown(Output{ID: 1})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(s, "No to-do items found") {
		t.Errorf("expected 'No to-do items found', got %q", s)
	}
}

// TestFormatListMarkdownString_WithItems verifies FormatListMarkdownString when with items.
func TestFormatListMarkdownString_WithItems(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{
		Todos: []Output{
			{ID: 1, ActionName: "assigned", TargetTitle: "Fix", TargetType: "Issue", State: "pending", ProjectName: "proj"},
			{ID: 2, ActionName: "mentioned", TargetTitle: "Add", TargetType: "MergeRequest", State: "pending", ProjectName: "proj"},
		},
	})
	if !strings.Contains(s, "assigned") {
		t.Error("expected action in table")
	}
	if !strings.Contains(s, "mentioned") {
		t.Error("expected second item")
	}
}

// TestFormatListMarkdownString_ClickableTargetLinks verifies that list table
// renders target titles as clickable Markdown links when TargetURL is present.
func TestFormatListMarkdownString_ClickableTargetLinks(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{
		Todos: []Output{
			{
				ID: 1, ActionName: "assigned", TargetTitle: "Fix bug", TargetType: "Issue",
				State: "pending", ProjectName: "proj", TargetURL: "https://gitlab.example.com/issues/1",
			},
		},
	})
	if !strings.Contains(s, "[Fix bug](https://gitlab.example.com/issues/1)") {
		t.Errorf("expected clickable target link in list, got:\n%s", s)
	}
}

// TestFormatListMarkdownString_NoLinkWithoutTargetURL verifies that target
// title appears as plain text when TargetURL is empty.
func TestFormatListMarkdownString_NoLinkWithoutTargetURL(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{
		Todos: []Output{
			{
				ID: 1, ActionName: "assigned", TargetTitle: "Fix bug", TargetType: "Issue",
				State: "pending", ProjectName: "proj",
			},
		},
	})
	if strings.Contains(s, "[Fix bug](") {
		t.Errorf("should not contain link when TargetURL is empty, got:\n%s", s)
	}
	if !strings.Contains(s, "Fix bug") {
		t.Errorf("should contain target title as plain text, got:\n%s", s)
	}
}

// TestFormatOutputMarkdownString_ClickableTarget verifies that detail view
// renders target as clickable link when TargetURL is present.
func TestFormatOutputMarkdownString_ClickableTarget(t *testing.T) {
	s := FormatOutputMarkdownString(Output{
		ID: 1, ActionName: "assigned", TargetTitle: "Fix", TargetType: "Issue",
		State: "pending", ProjectName: "proj",
		TargetURL: "https://gitlab.example.com/issues/1",
	})
	if !strings.Contains(s, "[Fix](https://gitlab.example.com/issues/1)") {
		t.Errorf("expected clickable target in detail, got:\n%s", s)
	}
}

// TestFormatOutputMarkdownString_NoLinkWithoutTargetURL verifies that
// target appears as plain text when TargetURL is empty.
func TestFormatOutputMarkdownString_NoLinkWithoutTargetURL(t *testing.T) {
	s := FormatOutputMarkdownString(Output{
		ID: 1, ActionName: "assigned", TargetTitle: "Fix", TargetType: "Issue",
		State: "pending", ProjectName: "proj",
	})
	if strings.Contains(s, "[Fix](") {
		t.Errorf("should not contain link without TargetURL, got:\n%s", s)
	}
	if !strings.Contains(s, "Fix") {
		t.Errorf("should contain target title as plain text, got:\n%s", s)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	r := FormatListMarkdown(ListOutput{})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatMarkDoneMarkdownString verifies FormatMarkDoneMarkdownString.
func TestFormatMarkDoneMarkdownString(t *testing.T) {
	s := FormatMarkDoneMarkdownString(MarkDoneOutput{ID: 1, Message: "To-do 1 marked as done"})
	if !strings.Contains(s, "To-do 1 marked as done") {
		t.Errorf("got %q", s)
	}
}

// TestFormatMarkDoneMarkdown verifies FormatMarkDoneMarkdown.
func TestFormatMarkDoneMarkdown(t *testing.T) {
	r := FormatMarkDoneMarkdown(MarkDoneOutput{Message: "done"})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// TestFormatMarkAllDoneMarkdownString verifies FormatMarkAllDoneMarkdownString.
func TestFormatMarkAllDoneMarkdownString(t *testing.T) {
	s := FormatMarkAllDoneMarkdownString(MarkAllDoneOutput{Message: "All done"})
	if !strings.Contains(s, "All done") {
		t.Errorf("got %q", s)
	}
}

// TestFormatMarkAllDoneMarkdown verifies FormatMarkAllDoneMarkdown.
func TestFormatMarkAllDoneMarkdown(t *testing.T) {
	r := FormatMarkAllDoneMarkdown(MarkAllDoneOutput{Message: "done"})
	if r == nil {
		t.Error(errExpectedNonNilResult)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route tests
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies todo action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "todos" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes covers ActionSpecs with table-driven subtests for call routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == pathTodos && r.Method == http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"action_name":"assigned","target_type":"Issue","target":{"title":"T"},"state":"pending","project":{"name":"p"},"author":{"username":"u"},"created_at":"2026-01-01T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", Total: "1", TotalPages: "1", PerPage: "20"})
		case r.URL.Path == pathTodoMarkDone && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == pathTodoMarkAll && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_todo_list", map[string]any{}},
		{"gitlab_todo_mark_done", map[string]any{"id": float64(1)}},
		{"gitlab_todo_mark_all_done", map[string]any{}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			result, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler %s: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// mr_notes_test.go contains unit tests for merge request note (comment)
// operations (create, list, update, delete). Tests use httptest to mock the
// GitLab Notes API and verify success, error, and pagination paths.
package mrnotes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Test constants for note endpoint paths and reusable body values.
const (
	pathMR1Notes       = "/api/v4/projects/42/merge_requests/1/notes"
	testUpdatedComment = "Updated comment"
	testNoteBody       = "LGTM!"
	testProjectID      = "42"
	fmtBodyWant        = "out.Body = %q, want %q"
)

// TestMRNoteCreate_Success verifies that mrNoteCreate adds a comment to a merge
// request. The mock returns a 201 response and the test asserts the note ID,
// body, and system flag.
func TestMRNoteCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Notes {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":200,"body":"LGTM!","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T12:00:00Z","system":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Body:      testNoteBody,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 200 {
		t.Errorf("out.ID = %d, want 200", out.ID)
	}
	if out.Body != testNoteBody {
		t.Errorf(fmtBodyWant, out.Body, testNoteBody)
	}
	if out.System {
		t.Error("out.System = true, want false")
	}
}

// TestMRNoteCreate_EmptyBody verifies that mrNoteCreate returns an error when
// the GitLab API rejects an empty body with a 400 response.
func TestMRNoteCreate_EmptyBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":{"note":["can't be blank"]}}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Body:      "",
	})
	if err == nil {
		t.Fatal("Create() expected error for empty body, got nil")
	}
}

// TestMRNotesList_Success verifies that mrNotesList returns all notes for a
// merge request. The mock returns two notes and the test asserts the correct
// count.
func TestMRNotesList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1Notes {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":200,"body":"LGTM!","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T12:00:00Z","system":false},{"id":201,"body":"Please address the comment above.","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T13:00:00Z","updated_at":"2026-03-02T13:00:00Z","system":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Notes) != 2 {
		t.Errorf("len(out.Notes) = %d, want 2", len(out.Notes))
	}
}

// TestMRNoteUpdate_Success verifies that mrNoteUpdate modifies a note's body.
// The mock returns the updated note and the test asserts the body matches the
// expected value.
func TestMRNoteUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/notes/200" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":200,"body":"Updated comment","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T14:00:00Z","system":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		NoteID:    200,
		Body:      testUpdatedComment,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Body != testUpdatedComment {
		t.Errorf(fmtBodyWant, out.Body, testUpdatedComment)
	}
}

// TestMRNoteDelete_Success verifies that mrNoteDelete removes a note from a
// merge request. The mock returns 204 No Content and the test asserts no error.
func TestMRNoteDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/merge_requests/1/notes/200" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := Delete(context.Background(), client, DeleteInput{
		ProjectID: testProjectID,
		MRIID:     1,
		NoteID:    200,
	}); err != nil {
		t.Errorf("Delete() unexpected error: %v", err)
	}
}

// TestMRNoteCreateSuccess_EnrichedFields verifies that mrNoteCreate maps
// enriched fields: Resolvable, Resolved, ResolvedBy, Internal, NoteableType, Type.
func TestMRNoteCreate_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Notes {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":300,"body":"Please fix this",
				"author":{"id":5,"username":"reviewer"},
				"created_at":"2026-03-10T12:00:00Z","updated_at":"2026-03-10T12:00:00Z",
				"system":false,
				"resolvable":true,"resolved":true,
				"resolved_by":{"username":"author"},
				"internal":true,
				"noteable_type":"MergeRequest",
				"type":"DiffNote"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Body:      "Please fix this",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if !out.Resolvable {
		t.Error("out.Resolvable = false, want true")
	}
	if !out.Resolved {
		t.Error("out.Resolved = false, want true")
	}
	if out.ResolvedBy != "author" {
		t.Errorf("out.ResolvedBy = %q, want %q", out.ResolvedBy, "author")
	}
	if !out.Internal {
		t.Error("out.Internal = false, want true")
	}
	if out.NoteableType != "MergeRequest" {
		t.Errorf("out.NoteableType = %q, want %q", out.NoteableType, "MergeRequest")
	}
	if out.Type != "DiffNote" {
		t.Errorf("out.Type = %q, want %q", out.Type, "DiffNote")
	}
}

// TestMRNotesList_PaginationQueryParamsAndMetadata verifies that mrNotesList
// forwards page and per_page query parameters to the GitLab API and correctly
// parses pagination metadata (page, total, prev_page) from response headers.
func TestMRNotesList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1Notes {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("per_page"); got != "5" {
				t.Errorf("query param per_page = %q, want %q", got, "5")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":300,"body":"Note on page 2","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T12:00:00Z","system":false}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "12", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, MRIID: 1, PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5}})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 12 {
		t.Errorf("Pagination.TotalItems = %d, want 12", out.Pagination.TotalItems)
	}
	if out.Pagination.PrevPage != 1 {
		t.Errorf("Pagination.PrevPage = %d, want 1", out.Pagination.PrevPage)
	}
}

// Tests for GetNote.

// TestGetNote_Success verifies the behavior of get note success.
func TestGetNote_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1/notes/300" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":300,"body":"This needs review","author":{"id":5,"username":"reviewer"},"created_at":"2026-03-10T12:00:00Z","updated_at":"2026-03-10T12:30:00Z","system":false,"resolvable":true,"resolved":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetNote(context.Background(), client, GetInput{
		ProjectID: testProjectID,
		MRIID:     1,
		NoteID:    300,
	})
	if err != nil {
		t.Fatalf("GetNote() unexpected error: %v", err)
	}
	if out.ID != 300 {
		t.Errorf("out.ID = %d, want 300", out.ID)
	}
	if out.Body != "This needs review" {
		t.Errorf(fmtBodyWant, out.Body, "This needs review")
	}
	if out.Author != "reviewer" {
		t.Errorf("out.Author = %q, want %q", out.Author, "reviewer")
	}
}

// TestGetNote_MissingProjectID verifies the behavior of get note missing project i d.
func TestGetNote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetNote(context.Background(), client, GetInput{
		MRIID:  1,
		NoteID: 300,
	})
	if err == nil {
		t.Fatal("GetNote() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// int64 validation tests
// ---------------------------------------------------------------------------.

// assertContains is a test helper that verifies err is non-nil and its
// message contains the expected substring.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies that all functions requiring merge_request_iid
// return an error when MRIID is 0.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt(testProjectID)
	const wantSubstr = "merge_request_iid"

	t.Run("Create", func(t *testing.T) {
		_, err := Create(ctx, client, CreateInput{ProjectID: pid, MRIID: 0, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("List", func(t *testing.T) {
		_, err := List(ctx, client, ListInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Update", func(t *testing.T) {
		_, err := Update(ctx, client, UpdateInput{ProjectID: pid, MRIID: 0, NoteID: 1, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("GetNote", func(t *testing.T) {
		_, err := GetNote(ctx, client, GetInput{ProjectID: pid, MRIID: 0, NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Delete", func(t *testing.T) {
		err := Delete(ctx, client, DeleteInput{ProjectID: pid, MRIID: 0, NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
}

// TestNoteIDRequired_Validation verifies that functions requiring note_id
// return an error when NoteID is 0.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when NoteID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt(testProjectID)
	const wantSubstr = "note_id"

	t.Run("Update", func(t *testing.T) {
		_, err := Update(ctx, client, UpdateInput{ProjectID: pid, MRIID: 1, NoteID: 0, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("GetNote", func(t *testing.T) {
		_, err := GetNote(ctx, client, GetInput{ProjectID: pid, MRIID: 1, NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Delete", func(t *testing.T) {
		err := Delete(ctx, client, DeleteInput{ProjectID: pid, MRIID: 1, NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ToOutput — cover remaining branches (ExpiresAt)
// ---------------------------------------------------------------------------.

// TestToOutput_ExpiresAtSet verifies the behavior of to output expires at set.
func TestToOutput_ExpiresAtSet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":400,"body":"expiring note",
			"author":{"id":1,"username":"jmrplens"},
			"created_at":"2026-03-02T12:00:00Z",
			"updated_at":"2026-03-02T12:00:00Z",
			"system":false,
			"expires_at":"2026-06-01T00:00:00Z",
			"attachment":"file.txt",
			"file_name":"file.txt",
			"noteable_id":99,
			"noteable_iid":10,
			"commit_id":"abc123",
			"project_id":42
		}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", MRIID: 1, Body: "expiring note",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ExpiresAt == "" {
		t.Error("ExpiresAt should be set, got empty")
	}
	if out.Attachment != "file.txt" {
		t.Errorf("Attachment = %q, want %q", out.Attachment, "file.txt")
	}
	if out.FileName != "file.txt" {
		t.Errorf("FileName = %q, want %q", out.FileName, "file.txt")
	}
	if out.NoteableID != 99 {
		t.Errorf("NoteableID = %d, want 99", out.NoteableID)
	}
	if out.NoteableIID != 10 {
		t.Errorf("NoteableIID = %d, want 10", out.NoteableIID)
	}
	if out.CommitID != "abc123" {
		t.Errorf("CommitID = %q, want %q", out.CommitID, "abc123")
	}
	if out.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", out.ProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create — missing project_id + canceled context
// ---------------------------------------------------------------------------.

// TestCreate_MissingProjectID verifies the behavior of create missing project i d.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — missing project_id, canceled context, sort/order_by forwarding
// ---------------------------------------------------------------------------.

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestList_SortAndOrderByForwarded verifies the behavior of list sort and order by forwarded.
func TestList_SortAndOrderByForwarded(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("order_by"); got != "updated_at" {
			t.Errorf("order_by = %q, want %q", got, "updated_at")
		}
		if got := r.URL.Query().Get("sort"); got != "desc" {
			t.Errorf("sort = %q, want %q", got, "desc")
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"body":"b","author":{"username":"u"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false}]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42", MRIID: 1, OrderBy: "updated_at", Sort: "desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Update — missing project_id, canceled context, API error
// ---------------------------------------------------------------------------.

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{MRIID: 1, NoteID: 200, Body: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 200, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 200, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetNote — canceled context, API error
// ---------------------------------------------------------------------------.

// TestGetNote_CancelledContext verifies the behavior of get note cancelled context.
func TestGetNote_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetNote(ctx, client, GetInput{ProjectID: "42", MRIID: 1, NoteID: 300})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGetNote_APIError verifies the behavior of get note a p i error.
func TestGetNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := GetNote(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1, NoteID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Delete — missing project_id, canceled context, API error
// ---------------------------------------------------------------------------.

// TestDelete_MissingProjectID verifies the behavior of delete missing project i d.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{MRIID: 1, NoteID: 200})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 200})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 200})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Full verifies the behavior of format output markdown full.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	out := Output{
		ID:         500,
		Body:       "Looks good!",
		Author:     "reviewer",
		CreatedAt:  "2026-03-02T12:00:00Z",
		System:     true,
		Internal:   true,
		Resolvable: true,
		Resolved:   true,
		ResolvedBy: "author",
	}
	md := FormatOutputMarkdown(out)

	for _, want := range []string{
		"## MR Note #500",
		"reviewer",
		"2 Mar 2026 12:00 UTC",
		"System note",
		"Internal note",
		"resolved",
		"@author",
		"Looks good!",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_Minimal verifies the behavior of format output markdown minimal.
func TestFormatOutputMarkdown_Minimal(t *testing.T) {
	out := Output{
		ID:        1,
		Body:      "hi",
		Author:    "u",
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	md := FormatOutputMarkdown(out)
	if !strings.Contains(md, "## MR Note #1") {
		t.Errorf("missing header:\n%s", md)
	}
	if strings.Contains(md, "System note") {
		t.Error("should not contain System note for non-system note")
	}
	if strings.Contains(md, "Internal note") {
		t.Error("should not contain Internal note for non-internal note")
	}
	if strings.Contains(md, "Resolvable") {
		t.Error("should not contain Resolvable for non-resolvable note")
	}
}

// TestFormatOutputMarkdown_ResolvableUnresolved verifies the behavior of format output markdown resolvable unresolved.
func TestFormatOutputMarkdown_ResolvableUnresolved(t *testing.T) {
	out := Output{
		ID:         2,
		Body:       "thread",
		Author:     "u",
		CreatedAt:  "2026-01-01T00:00:00Z",
		Resolvable: true,
		Resolved:   false,
	}
	md := FormatOutputMarkdown(out)
	if !strings.Contains(md, "unresolved") {
		t.Errorf("expected 'unresolved' in markdown:\n%s", md)
	}
	if strings.Contains(md, "Resolved By") {
		t.Error("should not show Resolved By when no resolver set")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithNotes verifies the behavior of format list markdown with notes.
func TestFormatListMarkdown_WithNotes(t *testing.T) {
	out := ListOutput{
		Notes: []Output{
			{ID: 1, Author: "a", CreatedAt: "2026-01-01T00:00:00Z", System: false},
			{ID: 2, Author: "b", CreatedAt: "2026-01-02T00:00:00Z", System: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	for _, want := range []string{"## MR Notes (2)", "| 1 |", "| 2 |", "| ID |"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Notes:      []Output{},
		Pagination: toolutil.PaginationOutput{},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No merge request notes found.") {
		t.Errorf("expected 'No merge request notes found.' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// TestRegisterTools_CallAllThroughMCP — full MCP roundtrip
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newMRNotesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_note_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "body": "comment"}},
		{"gitlab_mr_notes_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_note_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 200, "body": "updated"}},
		{"gitlab_mr_note_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 200}},
		{"gitlab_mr_note_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "note_id": 200}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newMRNotesMCPSession is an internal helper for the mrnotes package.
func newMRNotesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	noteJSON := `{"id":200,"body":"comment","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T12:00:00Z","system":false}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusOK, "["+noteJSON+"]")

		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSON)

		case r.Method == http.MethodGet && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSON)

		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

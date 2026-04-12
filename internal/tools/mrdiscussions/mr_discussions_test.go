// mr_discussions_test.go contains unit tests for merge request discussion
// operations (create general, create inline, resolve, reply, list). Tests use
// httptest to mock the GitLab Discussions API and verify success paths and
// pagination behavior.
package mrdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Test constants for discussion endpoint paths and reusable body values.
const (
	pathMR1Discussions     = "/api/v4/projects/42/merge_requests/1/discussions"
	testRefactoringComment = "This needs refactoring"
	testHelperReply        = "Done, extracted to helper function"
	testDiscussionID       = "abc123"
	testProjectID          = "42"
	testUpdatedComment     = "Updated comment"
	fmtIDWant              = "out.ID = %q, want %q"
)

// TestMRDiscussion_CreateGeneral verifies that mrDiscussionCreate creates a
// general (non-inline) discussion. The mock returns a 201 response and the
// test asserts the discussion ID and note body.
func TestMRDiscussion_CreateGeneral(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Discussions {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":"abc123","individual_note":false,"notes":[{"id":300,"body":"This needs refactoring","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Body:      testRefactoringComment,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != testDiscussionID {
		t.Errorf(fmtIDWant, out.ID, testDiscussionID)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(out.Notes) = %d, want 1", len(out.Notes))
	}
	if out.Notes[0].Body != testRefactoringComment {
		t.Errorf("out.Notes[0].Body = %q, want %q", out.Notes[0].Body, testRefactoringComment)
	}
}

// TestMRDiscussion_CreateInline verifies that mrDiscussionCreate creates an
// inline diff comment when a DiffPosition is provided. The mock returns a 201
// response and the test asserts the discussion ID.
func TestMRDiscussion_CreateInline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Discussions {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":"def456","individual_note":false,"notes":[{"id":301,"body":"Consider extracting this method","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Body:      "Consider extracting this method",
		Position: &DiffPosition{
			BaseSHA:  "base000",
			StartSHA: "start111",
			HeadSHA:  "head222",
			NewPath:  "internal/tools/repositories.go",
			NewLine:  42,
		},
	})
	if err != nil {
		t.Fatalf("Create() (inline) unexpected error: %v", err)
	}
	if out.ID != "def456" {
		t.Errorf(fmtIDWant, out.ID, "def456")
	}
}

// TestMRDiscussionResolve_Success verifies that mrDiscussionResolve marks a
// discussion as resolved. The mock returns the discussion with resolved=true
// and the test asserts the first note's resolved flag.
func TestMRDiscussionResolve_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/discussions/abc123" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":"abc123","individual_note":false,"notes":[{"id":300,"body":"This needs refactoring","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":true}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Resolve(context.Background(), client, ResolveInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
		Resolved:     true,
	})
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if len(out.Notes) == 0 || !out.Notes[0].Resolved {
		t.Error("expected discussion to be resolved")
	}
}

// TestMRDiscussionReply_Success verifies that mrDiscussionReply adds a reply
// note to an existing discussion. The mock returns a 201 response and the test
// asserts the reply body matches the expected value.
func TestMRDiscussionReply_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/merge_requests/1/discussions/abc123/notes" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":302,"body":"Done, extracted to helper function","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T15:00:00Z","resolved":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Reply(context.Background(), client, ReplyInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
		Body:         testHelperReply,
	})
	if err != nil {
		t.Fatalf("Reply() unexpected error: %v", err)
	}
	if out.Body != testHelperReply {
		t.Errorf("out.Body = %q, want %q", out.Body, testHelperReply)
	}
}

// TestMRDiscussionList_Success verifies that mrDiscussionList returns all
// discussion threads for a merge request. The mock returns two discussions
// and the test asserts the correct count.
func TestMRDiscussionList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1Discussions {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":"abc123","individual_note":false,"notes":[{"id":300,"body":"Comment","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false}]},{"id":"def456","individual_note":true,"notes":[{"id":301,"body":"Another","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T13:00:00Z","resolved":true}]}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Discussions) != 2 {
		t.Errorf("len(out.Discussions) = %d, want 2", len(out.Discussions))
	}
}

// TestMRDiscussionList_PaginationQueryParamsAndMetadata verifies that
// mrDiscussionList forwards page and per_page query parameters to the GitLab
// API and correctly parses pagination metadata from response headers.
func TestMRDiscussionList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1Discussions {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "3" {
				t.Errorf("query param per_page = %q, want %q", got, "3")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":"abc123","individual_note":false,"notes":[{"id":300,"body":"Comment","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false}]}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "3", Total: "8", TotalPages: "3", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, MRIID: 1, PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 3}})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if out.Pagination.Page != 1 {
		t.Errorf("Pagination.Page = %d, want 1", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 8 {
		t.Errorf("Pagination.TotalItems = %d, want 8", out.Pagination.TotalItems)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("Pagination.NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// Tests for Get, UpdateNote, DeleteNote.

const (
	pathMR1Discussion1     = "/api/v4/projects/42/merge_requests/1/discussions/abc123"
	pathMR1Discussion1Note = "/api/v4/projects/42/merge_requests/1/discussions/abc123/notes/300"
)

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1Discussion1 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":"abc123","individual_note":false,"notes":[{"id":300,"body":"This needs refactoring","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false},{"id":301,"body":"Agreed","author":{"id":1,"username":"jmrplens"},"created_at":"2026-03-02T13:00:00Z","resolved":false}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != testDiscussionID {
		t.Errorf(fmtIDWant, out.ID, testDiscussionID)
	}
	if len(out.Notes) != 2 {
		t.Errorf("len(out.Notes) = %d, want 2", len(out.Notes))
	}
}

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{
		MRIID:        1,
		DiscussionID: testDiscussionID,
	})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id, got nil")
	}
}

// TestUpdateNote_Body verifies the behavior of update note body.
func TestUpdateNote_Body(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1Discussion1Note {
			testutil.RespondJSON(w, http.StatusOK, `{"id":300,"body":"Updated comment","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","updated_at":"2026-03-02T14:00:00Z","resolved":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateNote(context.Background(), client, UpdateNoteInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
		NoteID:       300,
		Body:         testUpdatedComment,
	})
	if err != nil {
		t.Fatalf("UpdateNote() unexpected error: %v", err)
	}
	if out.Body != testUpdatedComment {
		t.Errorf("out.Body = %q, want %q", out.Body, testUpdatedComment)
	}
	if out.ID != 300 {
		t.Errorf("out.ID = %d, want 300", out.ID)
	}
}

// TestUpdateNote_Resolved verifies the behavior of update note resolved.
func TestUpdateNote_Resolved(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1Discussion1Note {
			testutil.RespondJSON(w, http.StatusOK, `{"id":300,"body":"This needs refactoring","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":true,"resolvable":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	resolved := true
	out, err := UpdateNote(context.Background(), client, UpdateNoteInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
		NoteID:       300,
		Resolved:     &resolved,
	})
	if err != nil {
		t.Fatalf("UpdateNote() unexpected error: %v", err)
	}
	if !out.Resolved {
		t.Error("expected note to be resolved")
	}
}

// TestUpdateNote_MissingProjectID verifies the behavior of update note missing project i d.
func TestUpdateNote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := UpdateNote(context.Background(), client, UpdateNoteInput{
		MRIID:        1,
		DiscussionID: testDiscussionID,
		NoteID:       300,
		Body:         "test",
	})
	if err == nil {
		t.Fatal("UpdateNote() expected error for missing project_id, got nil")
	}
}

// TestDeleteNote_Success verifies the behavior of delete note success.
func TestDeleteNote_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathMR1Discussion1Note {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteNote(context.Background(), client, DeleteNoteInput{
		ProjectID:    testProjectID,
		MRIID:        1,
		DiscussionID: testDiscussionID,
		NoteID:       300,
	})
	if err != nil {
		t.Fatalf("DeleteNote() unexpected error: %v", err)
	}
}

// TestDeleteNote_MissingProjectID verifies the behavior of delete note missing project i d.
func TestDeleteNote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteNote(context.Background(), client, DeleteNoteInput{
		MRIID:        1,
		DiscussionID: testDiscussionID,
		NoteID:       300,
	})
	if err == nil {
		t.Fatal("DeleteNote() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// int64 validation tests
// ---------------------------------------------------------------------------.

// assertContains is an internal helper for the mrdiscussions package.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies the behavior of m r i i d required validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt(testProjectID)
	const wantSubstr = "mr_iid"

	t.Run("Create", func(t *testing.T) {
		_, err := Create(ctx, client, CreateInput{ProjectID: pid, MRIID: 0, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Resolve", func(t *testing.T) {
		_, err := Resolve(ctx, client, ResolveInput{ProjectID: pid, MRIID: 0, DiscussionID: "d1", Resolved: true})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Reply", func(t *testing.T) {
		_, err := Reply(ctx, client, ReplyInput{ProjectID: pid, MRIID: 0, DiscussionID: "d1", Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("List", func(t *testing.T) {
		_, err := List(ctx, client, ListInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Get", func(t *testing.T) {
		_, err := Get(ctx, client, GetInput{ProjectID: pid, MRIID: 0, DiscussionID: "d1"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("UpdateNote", func(t *testing.T) {
		_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, MRIID: 0, DiscussionID: "d1", NoteID: 1, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("DeleteNote", func(t *testing.T) {
		err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, MRIID: 0, DiscussionID: "d1", NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
}

// TestNoteIDRequired_Validation verifies the behavior of note i d required validation.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when NoteID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt(testProjectID)
	const wantSubstr = "note_id"

	t.Run("UpdateNote", func(t *testing.T) {
		_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, MRIID: 1, DiscussionID: "d1", NoteID: 0, Body: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("DeleteNote", func(t *testing.T) {
		err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, MRIID: 1, DiscussionID: "d1", NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Create — missing project_id, canceled context, OldPath/OldLine branches, API error
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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestCreate_InlineWithOldPathAndOldLine verifies the behavior of create inline with old path and old line.
func TestCreate_InlineWithOldPathAndOldLine(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Discussions {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"pos789",
				"individual_note":false,
				"notes":[{"id":310,"body":"old path comment","author":{"id":2,"username":"reviewer"},"created_at":"2026-03-02T12:00:00Z","resolved":false}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Body:      "old path comment",
		Position: &DiffPosition{
			BaseSHA:  "base000",
			StartSHA: "start111",
			HeadSHA:  "head222",
			OldPath:  "old/file.go",
			NewPath:  "new/file.go",
			OldLine:  10,
			NewLine:  15,
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != "pos789" {
		t.Errorf("out.ID = %q, want %q", out.ID, "pos789")
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Resolve — missing project_id, canceled context, API error
// ---------------------------------------------------------------------------.

// TestResolve_MissingProjectID verifies the behavior of resolve missing project i d.
func TestResolve_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Resolve(context.Background(), client, ResolveInput{MRIID: 1, DiscussionID: "abc123", Resolved: true})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestResolve_CancelledContext verifies the behavior of resolve cancelled context.
func TestResolve_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Resolve(ctx, client, ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Resolved: true})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestResolve_APIError verifies the behavior of resolve a p i error.
func TestResolve_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Resolve(context.Background(), client, ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Resolved: true})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Reply — missing project_id, canceled context, API error
// ---------------------------------------------------------------------------.

// TestReply_MissingProjectID verifies the behavior of reply missing project i d.
func TestReply_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Reply(context.Background(), client, ReplyInput{MRIID: 1, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestReply_CancelledContext verifies the behavior of reply cancelled context.
func TestReply_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Reply(ctx, client, ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestReply_APIError verifies the behavior of reply a p i error.
func TestReply_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Reply(context.Background(), client, ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// List — missing project_id, canceled context, API error
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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
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
// Get — canceled context, API error
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1, DiscussionID: "notfound"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// UpdateNote — canceled context, API error
// ---------------------------------------------------------------------------.

// TestUpdateNote_CancelledContext verifies the behavior of update note cancelled context.
func TestUpdateNote_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestUpdateNote_APIError verifies the behavior of update note a p i error.
func TestUpdateNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := UpdateNote(context.Background(), client, UpdateNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteNote — canceled context, API error
// ---------------------------------------------------------------------------.

// TestDeleteNote_CancelledContext verifies the behavior of delete note cancelled context.
func TestDeleteNote_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestDeleteNote_APIError verifies the behavior of delete note a p i error.
func TestDeleteNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := DeleteNote(context.Background(), client, DeleteNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// FormatNoteMarkdown
// ---------------------------------------------------------------------------.

// TestFormatNoteMarkdown_Full verifies the behavior of format note markdown full.
func TestFormatNoteMarkdown_Full(t *testing.T) {
	n := NoteOutput{
		ID:        500,
		Body:      "Looks good!",
		Author:    "reviewer",
		CreatedAt: "2026-03-02T12:00:00Z",
		Resolved:  true,
	}
	md := FormatNoteMarkdown(n)

	for _, want := range []string{
		"## Discussion Note #500",
		"reviewer",
		"2 Mar 2026 12:00 UTC",
		"**Resolved**: true",
		"Looks good!",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatNoteMarkdown_Minimal verifies the behavior of format note markdown minimal.
func TestFormatNoteMarkdown_Minimal(t *testing.T) {
	n := NoteOutput{ID: 1, Body: "hi", Author: "u", CreatedAt: "2026-01-01T00:00:00Z"}
	md := FormatNoteMarkdown(n)
	if !strings.Contains(md, "## Discussion Note #1") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Resolved**: false") {
		t.Errorf("should show resolved false:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Full verifies the behavior of format output markdown full.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	d := Output{
		ID:             "disc-abc",
		IndividualNote: false,
		Notes: []NoteOutput{
			{ID: 1, Body: "First", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Body: "Reply", Author: "bob", CreatedAt: "2026-01-02T00:00:00Z"},
		},
	}
	md := FormatOutputMarkdown(d)

	for _, want := range []string{
		"## Discussion disc-abc",
		"**Notes**: 2",
		"**Individual Note**: false",
		"### Note 1 (by alice)",
		"First",
		"### Note 2 (by bob)",
		"Reply",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	d := Output{ID: "empty-disc", IndividualNote: true, Notes: nil}
	md := FormatOutputMarkdown(d)
	if !strings.Contains(md, "## Discussion empty-disc") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Notes**: 0") {
		t.Errorf("should show 0 notes:\n%s", md)
	}
	if !strings.Contains(md, "**Individual Note**: true") {
		t.Errorf("should show individual note:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDiscussions verifies the behavior of format list markdown with discussions.
func TestFormatListMarkdown_WithDiscussions(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{ID: "d1", IndividualNote: false, Notes: []NoteOutput{{ID: 1}, {ID: 2}}},
			{ID: "d2", IndividualNote: true, Notes: []NoteOutput{{ID: 3}}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 5, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	for _, want := range []string{
		"## MR Discussions (5)",
		"| ID |",
		"| d1 |",
		"| d2 |",
		"2",
		"1",
		"false",
		"true",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{},
		Pagination:  toolutil.PaginationOutput{},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No merge request discussions found.") {
		t.Errorf("expected 'No merge request discussions found.' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// NoteToOutput — verify UpdatedAt formatting
// ---------------------------------------------------------------------------.

// TestNoteToOutput_NilTimestamps verifies the behavior of note to output nil timestamps.
func TestNoteToOutput_NilTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1Discussions {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"ts001",
				"individual_note":false,
				"notes":[{
					"id":400,
					"body":"no timestamps",
					"author":{"id":1,"username":"tester"},
					"resolved":false,
					"resolvable":true,
					"system":true,
					"internal":true,
					"noteable_type":"MergeRequest",
					"noteable_id":99,
					"noteable_iid":10,
					"commit_id":"sha123",
					"project_id":42
				}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{ProjectID: "42", MRIID: 1, Body: "no timestamps"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	n := out.Notes[0]
	if n.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty for nil timestamp", n.CreatedAt)
	}
	if n.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty for nil timestamp", n.UpdatedAt)
	}
	if !n.System {
		t.Error("expected System = true")
	}
	if !n.Internal {
		t.Error("expected Internal = true")
	}
	if n.Type != "MergeRequest" {
		t.Errorf("Type = %q, want %q", n.Type, "MergeRequest")
	}
	if n.NoteableID != 99 {
		t.Errorf("NoteableID = %d, want 99", n.NoteableID)
	}
	if n.NoteableIID != 10 {
		t.Errorf("NoteableIID = %d, want 10", n.NoteableIID)
	}
	if n.CommitID != "sha123" {
		t.Errorf("CommitID = %q, want %q", n.CommitID, "sha123")
	}
	if n.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", n.ProjectID)
	}
}

// ---------------------------------------------------------------------------
// TestRegisterTools_CallAllThroughMCP — full MCP roundtrip for all 7 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newMRDiscussionsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_discussion_create", map[string]any{"project_id": "42", "mr_iid": 1, "body": "new discussion"}},
		{"gitlab_mr_discussion_resolve", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc123", "resolved": true}},
		{"gitlab_mr_discussion_reply", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc123", "body": "reply"}},
		{"gitlab_mr_discussion_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_discussion_get", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc123"}},
		{"gitlab_mr_discussion_note_update", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc123", "note_id": 300, "body": "updated"}},
		{"gitlab_mr_discussion_note_delete", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc123", "note_id": 300}},
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

// newMRDiscussionsMCPSession is an internal helper for the mrdiscussions package.
func newMRDiscussionsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	discussionJSON := `{
		"id":"abc123",
		"individual_note":false,
		"notes":[{
			"id":300,
			"body":"comment",
			"author":{"id":1,"username":"jmrplens"},
			"created_at":"2026-03-02T12:00:00Z",
			"updated_at":"2026-03-02T12:00:00Z",
			"resolved":false
		}]
	}`

	noteJSON := `{
		"id":300,
		"body":"reply",
		"author":{"id":1,"username":"jmrplens"},
		"created_at":"2026-03-02T12:00:00Z",
		"updated_at":"2026-03-02T12:00:00Z",
		"resolved":false
	}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		// POST .../discussions → create discussion
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusCreated, discussionJSON)

		// POST .../discussions/{id}/notes → reply to discussion
		case r.Method == http.MethodPost && strings.Contains(path, "/discussions/") && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)

		// PUT .../discussions/{id}/notes/{noteID} → update note
		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSON)

		// DELETE .../discussions/{id}/notes/{noteID} → delete note
		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)

		// PUT .../discussions/{id} → resolve/unresolve
		case r.Method == http.MethodPut && strings.Contains(path, "/discussions/"):
			testutil.RespondJSON(w, http.StatusOK, discussionJSON)

		// GET .../discussions/{id} → get single discussion
		case r.Method == http.MethodGet && strings.Contains(path, "/discussions/"):
			testutil.RespondJSON(w, http.StatusOK, discussionJSON)

		// GET .../discussions → list discussions
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusOK, "["+discussionJSON+"]")

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

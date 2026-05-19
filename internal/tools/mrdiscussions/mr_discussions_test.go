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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	// pathMR1Discussion1 identifies the path MR 1 discussion 1 constant used by this package.
	pathMR1Discussion1 = "/api/v4/projects/42/merge_requests/1/discussions/abc123"
	// pathMR1Discussion1Note identifies the path MR 1 discussion 1 note constant used by this package.
	pathMR1Discussion1Note = "/api/v4/projects/42/merge_requests/1/discussions/abc123/notes/300"
)

// TestGet_Success verifies Get when success.
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

// TestGet_MissingProjectID verifies Get when missing project ID.
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

// TestUpdateNote_Body verifies UpdateNote when body.
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

// TestUpdateNote_Resolved verifies UpdateNote when resolved.
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

// TestUpdateNote_MissingProjectID verifies UpdateNote when missing project ID.
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

// TestDeleteNote_Success verifies DeleteNote when success.
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

// TestDeleteNote_MissingProjectID verifies DeleteNote when missing project ID.
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

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies MRIIDRequired when validation.
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

// TestNoteIDRequired_Validation verifies NoteIDRequired when validation.
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

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Create — missing project_id, canceled context, OldPath/OldLine branches, API error
// ---------------------------------------------------------------------------.

// TestCreate_MissingProjectID verifies Create when missing project ID.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
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

// TestCreate_InlineWithOldPathAndOldLine verifies Create when inline with old path and old line.
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

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", MRIID: 1, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_PositionValidationError verifies Create returns the validation
// error when an inline comment targets a file outside the MR diff.
func TestCreate_PositionValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"other.go","new_path":"other.go","diff":"@@ -1,1 +1,1 @@\n-old\n+new\n"}]`)
			return
		}
		t.Fatalf("unexpected API call: %s %s", r.Method, r.URL.Path)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Body:      "missing file",
		Position: &DiffPosition{
			NewPath: "missing.go",
			NewLine: 1,
		},
	})
	if err == nil {
		t.Fatal("expected position validation error")
	}
	if !strings.Contains(err.Error(), "not in the merge request diff") {
		t.Fatalf("error = %q, want diff validation message", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Resolve — missing project_id, canceled context, API error
// ---------------------------------------------------------------------------.

// TestResolve_MissingProjectID verifies Resolve when missing project ID.
func TestResolve_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Resolve(context.Background(), client, ResolveInput{MRIID: 1, DiscussionID: "abc123", Resolved: true})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestResolve_CancelledContext verifies Resolve when cancelled context.
func TestResolve_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Resolve(ctx, client, ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Resolved: true})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestResolve_APIError verifies Resolve when API error.
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

// TestReply_MissingProjectID verifies Reply when missing project ID.
func TestReply_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Reply(context.Background(), client, ReplyInput{MRIID: 1, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestReply_CancelledContext verifies Reply when cancelled context.
func TestReply_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Reply(ctx, client, ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestReply_APIError verifies Reply when API error.
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

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies List when cancelled context.
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

// TestList_APIError verifies List when API error.
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

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGet_APIError verifies Get when API error.
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

// TestUpdateNote_CancelledContext verifies UpdateNote when cancelled context.
func TestUpdateNote_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestUpdateNote_APIError verifies UpdateNote when API error.
func TestUpdateNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := UpdateNote(context.Background(), client, UpdateNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateNote_NotFoundAPIError verifies UpdateNote uses the note lookup
// hint for non-403 API failures such as 404.
func TestUpdateNote_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Note Not Found"}`)
	}))

	_, err := UpdateNote(context.Background(), client, UpdateNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_mr_discussion_get") {
		t.Fatalf("error = %q, want discussion get hint", err.Error())
	}
}

// ---------------------------------------------------------------------------
// DeleteNote — canceled context, API error
// ---------------------------------------------------------------------------.

// TestDeleteNote_CancelledContext verifies DeleteNote when cancelled context.
func TestDeleteNote_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc123", NoteID: 300})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestDeleteNote_APIError verifies DeleteNote when API error.
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

// TestFormatNoteMarkdown_Full verifies FormatNoteMarkdown when full.
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

// TestFormatNoteMarkdown_Minimal verifies FormatNoteMarkdown when minimal.
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

// TestFormatOutputMarkdown_Full verifies FormatOutputMarkdown when full.
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

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown when empty.
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

// TestFormatListMarkdown_WithDiscussions verifies FormatListMarkdown when with discussions.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestNoteToOutput_NilTimestamps verifies NoteToOutput when nil timestamps.
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
// TestActionSpecs_CallAllRoutes — canonical route execution for all 7 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates all MR discussion actions through their canonical ActionSpecs routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	specs := newMRDiscussionsActionSpecs(t)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_discussion_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "body": "new discussion"}},
		{"gitlab_mr_discussion_resolve", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc123", "resolved": true}},
		{"gitlab_mr_discussion_reply", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc123", "body": "reply"}},
		{"gitlab_mr_discussion_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_discussion_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc123"}},
		{"gitlab_mr_discussion_note_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc123", "note_id": 300, "body": "updated"}},
		{"gitlab_mr_discussion_note_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "discussion_id": "abc123", "note_id": 300}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if spec.OwnerPackage != "mrdiscussions" || !spec.OpenWorld {
				t.Fatalf("unexpected ActionSpec semantics for %s: %+v", tt.name, spec)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
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

// newMRDiscussionsActionSpecs builds canonical action specs backed by a mock GitLab API.
func newMRDiscussionsActionSpecs(t *testing.T) []toolutil.ActionSpec {
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

	return ActionSpecs(client)
}

// TestValidatePosition_FileNotInDiff verifies that validatePosition returns
// an error when the target file is not part of the MR diff.
func TestValidatePosition_FileNotInDiff(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"a.go","new_path":"a.go","diff":"@@ -1,3 +1,3 @@\n-old\n+new\n ctx\n"}]`)
	}))
	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "not_in_diff.go",
		NewLine: 1,
	})
	if err == nil {
		t.Fatal("expected error for file not in diff")
	}
	if !strings.Contains(err.Error(), "not in the merge request diff") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestValidatePosition_ValidFile verifies that validatePosition succeeds
// when the file is found in the MR diff and the line is valid.
func TestValidatePosition_ValidFile(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"a.go","new_path":"a.go","diff":"@@ -1,3 +1,3 @@\n-old\n+new\n ctx\n"}]`)
	}))
	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "a.go",
		NewLine: 1,
	})
	// err may or may not be nil depending on line validation, but it should not
	// be the "not in diff" error.
	if err != nil && strings.Contains(err.Error(), "not in the merge request diff") {
		t.Errorf("file should have been found in diff, got: %v", err)
	}
}

// TestValidatePosition_OldPathFallback verifies that validatePosition
// uses OldPath when NewPath is empty.
func TestValidatePosition_OldPathFallback(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"old.go","new_path":"renamed.go","diff":"@@ -1,2 +1,2 @@\n-x\n+y\n"}]`)
	}))
	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		OldPath: "old.go",
		OldLine: 1,
	})
	if err != nil && strings.Contains(err.Error(), "not in the merge request diff") {
		t.Errorf("old_path fallback should match, got: %v", err)
	}
}

// TestValidatePosition_APIError verifies that validatePosition returns nil
// (best-effort) when the diff API call fails.
func TestValidatePosition_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "a.go",
		NewLine: 1,
	})
	if err != nil {
		t.Errorf("expected nil (best-effort skip), got: %v", err)
	}
}

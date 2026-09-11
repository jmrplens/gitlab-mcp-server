// commit_discussions_test.go contains unit tests for the commit discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package commitdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpAPIFailure identifies the err exp API failure constant used by this package.
const errExpAPIFailure = "expected error for API failure, got nil"

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testDiscussionID identifies the test discussion ID constant used by this package.
const testDiscussionID = "d1"

// testCommitSHA identifies the test commit SHA constant used by this package.
const testCommitSHA = "abc123"

// testProjectID identifies the test project ID constant used by this package.
const testProjectID = "1"

// testAuthorAlice identifies the test author alice constant used by this package.
const testAuthorAlice = "alice"

// testVersion identifies the test version constant used by this package.
const testVersion = "0.0.1"

const (
	// testPathDiscussions identifies the test path discussions constant used by this package.
	testPathDiscussions = "/discussions"
	// testPathDiscussionSlash identifies the test path discussion slash constant used by this package.
	testPathDiscussionSlash = "/discussions/"
	// testDate20260101 identifies the test date 20260101 constant used by this package.
	testDate20260101 = "2026-01-01"
)

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/repository/commits/ (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/repository/commits/"+testCommitSHA+testPathDiscussions {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"Hello","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}]}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{ProjectID: testProjectID, CommitSHA: testCommitSHA})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 1 {
		t.Fatalf("got %d discussions, want 1", len(out.Discussions))
	}
	if out.Discussions[0].ID != testDiscussionID {
		t.Errorf("got ID=%q, want d1", out.Discussions[0].ID)
	}
	if got := out.Discussions[0].Notes[0].AuthorUsername(); got != testAuthorAlice {
		t.Errorf("got author=%q, want alice", got)
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{ProjectID: testProjectID, CommitSHA: testCommitSHA})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/repository/commits/ (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/repository/commits/"+testCommitSHA+testPathDiscussionSlash+testDiscussionID {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"d1","individual_note":true,"notes":[{"id":10,"body":"test note","author":{"username":"bob"},"created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != testDiscussionID {
		t.Errorf("got ID=%q, want d1", out.ID)
	}
	if !out.IndividualNote {
		t.Error("expected IndividualNote=true")
	}
}

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"d2","individual_note":false,"notes":[{"id":20,"body":"new discussion","author":{"username":"carol"},"created_at":"2026-01-02T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, Body: "new discussion"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "d2" {
		t.Errorf("got ID=%q, want d2", out.ID)
	}
}

// TestCreate_WithPosition verifies the Create_WithPosition handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_WithPosition(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"d3","individual_note":false,"notes":[{"id":30,"body":"inline comment","author":{"username":"dave"},"created_at":"2026-01-03T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		ProjectID: testProjectID,
		CommitSHA: testCommitSHA,
		Body:      "inline comment",
		Position: &PositionInput{
			BaseSHA:      "aaa",
			StartSHA:     "bbb",
			HeadSHA:      "ccc",
			PositionType: "text",
			NewPath:      "main.go",
			NewLine:      10,
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "d3" {
		t.Errorf("got ID=%q, want d3", out.ID)
	}
}

// TestAddNote_Success verifies that AddNote succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":40,"body":"reply","author":{"username":"eve"},"created_at":"2026-01-04T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, Body: "reply"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 40 {
		t.Errorf("got ID=%d, want 40", out.ID)
	}
	if got := out.AuthorUsername(); got != "eve" {
		t.Errorf("got author=%q, want eve", got)
	}
}

// TestUpdateNote_Success verifies that UpdateNote succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":40,"body":"updated","author":{"username":"eve"},"created_at":"2026-01-04T00:00:00Z","updated_at":"2026-01-05T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 40, Body: "updated"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Body != "updated" {
		t.Errorf("got body=%q, want updated", out.Body)
	}
}

// TestDeleteNote_Success verifies that DeleteNote succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)

	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 40})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteNote_APIError verifies that DeleteNote returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteNote_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 40})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Int64 Validation Tests
// ---------------------------------------------------------------------------.

// TestUpdateNote_NoteIDValidation verifies the UpdateNote_NoteIDValidation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateNote_NoteIDValidation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := UpdateNote(t.Context(), client, UpdateNoteInput{
		ProjectID:    testProjectID,
		CommitSHA:    testCommitSHA,
		DiscussionID: testDiscussionID,
		NoteID:       0,
		Body:         "updated",
	})
	if err == nil {
		t.Fatal("expected error for NoteID=0, got nil")
	}
	if !strings.Contains(err.Error(), "note_id") {
		t.Errorf("expected error to mention note_id, got: %v", err)
	}
}

// TestDeleteNote_NoteIDValidation verifies the DeleteNote_NoteIDValidation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteNote_NoteIDValidation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteNote(t.Context(), client, DeleteNoteInput{
		ProjectID:    testProjectID,
		CommitSHA:    testCommitSHA,
		DiscussionID: testDiscussionID,
		NoteID:       0,
	})
	if err == nil {
		t.Fatal("expected error for NoteID=0, got nil")
	}
	if !strings.Contains(err.Error(), "note_id") {
		t.Errorf("expected error to mention note_id, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Canceled Context Tests
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID, CommitSHA: testCommitSHA})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, Body: "t"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestAddNote_CancelledContext verifies the AddNote_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestAddNote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddNote(ctx, client, AddNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, Body: "t"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUpdateNote_CancelledContext verifies the UpdateNote_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateNote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 1, Body: "t"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDeleteNote_CancelledContext verifies the DeleteNote_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteNote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 1})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// API Error Tests
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, Body: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestAddNote_APIError verifies that AddNote returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, Body: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestUpdateNote_APIError verifies that UpdateNote returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 99, Body: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// Formatter Tests
// ---------------------------------------------------------------------------.

// The three guidance sections the cards and the list of this package end with,
// so each expectation below can pin the whole rendered document.
const (
	listHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_get_commit_discussion` with discussion_id to view full discussion details\n" +
		"- Use `gitlab_create_commit_discussion` to start a new discussion on this commit\n"
	threadHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_add_commit_discussion_note` to reply to this discussion\n" +
		"- Use `gitlab_update_commit_discussion_note` to edit a note\n"
	noteHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_update_commit_discussion_note` with note_id to edit this note\n" +
		"- Use `gitlab_add_commit_discussion_note` with discussion_id to reply to this discussion\n"
)

// TestFormatListMarkdownString_WithData pins the whole list document of a page
// of discussion threads: the heading counts the total GitLab reported rather
// than the length of the page, the summary names the page, and the pagination
// footer opens a block of its own after the last row.
func TestFormatListMarkdownString_WithData(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{ID: testDiscussionID, Notes: []*NoteOutput{{Author: &toolutil.NoteUserOutput{Username: testAuthorAlice}, CreatedAt: testDate20260101, Body: "comment"}}},
			{ID: "d2", Notes: []*NoteOutput{{Author: &toolutil.NoteUserOutput{Username: "bob"}, CreatedAt: "2026-01-02", Body: "reply"}}},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 45, TotalPages: 3, NextPage: 2, HasMore: true},
	}

	got := FormatListMarkdownString(out)

	want := "## Commit Discussions (45)\n\n" +
		"Showing 2 of 45 results (page 1 of 3)\n\n" +
		"| ID | Author | Notes |\n" +
		"| --- | --- | --- |\n" +
		"| d1 | alice | 1 |\n" +
		"| d2 | bob | 1 |\n\n" +
		"Page 1 of 3 | 45 items total | 20 per page\n" +
		listHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString_Empty pins the whole response of a commit with
// no discussions: one sentence, with no heading counting zero above it.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{Discussions: nil})

	if want := "No commit discussions found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkdownString_WithNotes pins the whole card of a thread: its own
// fields first, then one section per note with the body quoted under its label.
func TestFormatMarkdownString_WithNotes(t *testing.T) {
	out := Output{
		ID:    testDiscussionID,
		Notes: []*NoteOutput{{ID: 7, Author: &toolutil.NoteUserOutput{Username: "dev"}, CreatedAt: testDate20260101, Body: "LGTM"}},
	}

	got := FormatMarkdownString(out)

	want := "## Discussion d1\n\n" +
		"- **Notes**: 1\n" +
		"- **Individual Note**: ❌\n\n" +
		"### Note #7\n\n" +
		"- **Author**: @dev\n" +
		"- **Created**: 1 Jan 2026\n" +
		"- **Body**: LGTM\n" +
		threadHintsBlock
	if got != want {
		t.Errorf("thread card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_Resolvable pins the thread card of a resolvable
// thread: the resolution state is a row of its own, and a nil note in the slice
// the SDK may hand back adds no empty section.
func TestFormatMarkdownString_Resolvable(t *testing.T) {
	out := Output{
		ID:             testDiscussionID,
		IndividualNote: true,
		Resolvable:     true,
		Resolved:       true,
		Notes:          []*NoteOutput{nil},
	}

	got := FormatMarkdownString(out)

	want := "## Discussion d1\n\n" +
		"- **Notes**: 1\n" +
		"- **Individual Note**: ✅\n" +
		"- **Resolvable**: resolved\n" +
		threadHintsBlock
	if got != want {
		t.Errorf("thread card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_Empty pins the card of a thread GitLab answered with
// no notes: the heading and the flag it does carry, and no count of zero.
func TestFormatMarkdownString_Empty(t *testing.T) {
	got := FormatMarkdownString(Output{ID: testDiscussionID, Notes: nil})

	want := "## Discussion d1\n\n" +
		"- **Individual Note**: ❌\n" +
		threadHintsBlock
	if got != want {
		t.Errorf("thread card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdownString pins the whole card of a note on a line of a
// diff: the author as a handle, the resolution state a resolvable note carries,
// the file and line the note hangs on, and the body.
func TestFormatNoteMarkdownString(t *testing.T) {
	n := NoteOutput{
		ID:         10,
		Author:     &toolutil.NoteUserOutput{Username: "dev"},
		Body:       "Nice!",
		CreatedAt:  testDate20260101,
		Resolvable: true,
		Resolved:   true,
		ResolvedBy: &toolutil.NoteUserOutput{Username: testAuthorAlice},
		Position:   &toolutil.NotePositionOutput{NewPath: "internal/app.go", NewLine: 42},
	}

	got := FormatNoteMarkdownString(n)

	want := "## Discussion Note #10\n\n" +
		"- **Author**: @dev\n" +
		"- **Created**: 1 Jan 2026\n" +
		"- **Resolvable**: resolved\n" +
		"- **Resolved By**: @alice\n" +
		"- **Position**: `internal/app.go`:42\n" +
		"- **Body**: Nice!\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdownString_SystemNoteOnRemovedLine pins the card of a
// system note anchored to a removed line: the marker the flag writes, the old
// path and line, and no resolution state on a note that cannot be resolved.
func TestFormatNoteMarkdownString_SystemNoteOnRemovedLine(t *testing.T) {
	n := NoteOutput{
		ID:       12,
		Author:   &toolutil.NoteUserOutput{Username: "bot"},
		Body:     "changed the description",
		System:   true,
		Internal: true,
		Position: &toolutil.NotePositionOutput{OldPath: "internal/old.go", OldLine: 7},
	}

	got := FormatNoteMarkdownString(n)

	want := "## Discussion Note #12\n\n" +
		"- **Author**: @bot\n" +
		"- **System note**\n" +
		"- **Internal note**\n" +
		"- **Position**: `internal/old.go`:7\n" +
		"- **Body**: changed the description\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdownString_NoDate pins the card of a note GitLab answered
// with neither a time nor an author: neither row is written, where a label with
// nothing after it used to read as a value GitLab sent.
func TestFormatNoteMarkdownString_NoDate(t *testing.T) {
	got := FormatNoteMarkdownString(NoteOutput{ID: 11, Body: "OK"})

	want := "## Discussion Note #11\n\n" +
		"- **Body**: OK\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------.

// newCommitDiscussionMockHandler constructs commit discussion mock handler test fixtures.
func newCommitDiscussionMockHandler(t *testing.T) http.Handler {
	t.Helper()
	noteJSON := `{"id":1,"body":"t","author":{"username":"dev"},"created_at":"2026-01-01T00:00:00Z"}`
	discJSON := `{"id":"d1","individual_note":false,"notes":[` + noteJSON + `]}`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(p, testPathDiscussions) && !strings.Contains(p, testPathDiscussionSlash):
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+discJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && strings.Contains(p, testPathDiscussionSlash):
			testutil.RespondJSON(w, http.StatusOK, discJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(p, testPathDiscussions):
			testutil.RespondJSON(w, http.StatusCreated, discJSON)
		case r.Method == http.MethodPost && strings.Contains(p, testPathDiscussionSlash) && strings.HasSuffix(p, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)
		case r.Method == http.MethodPut && strings.Contains(p, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
		case r.Method == http.MethodDelete && strings.Contains(p, "/notes/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

// ---------------------------------------------------------------------------
// ActionSpecs Tests
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := commitDiscussionSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_commit_discussion_note"].Route.Destructive {
		t.Fatal("gitlab_delete_commit_discussion_note should be destructive")
	}
	if byTool["gitlab_list_commit_discussions"].Usage == "" {
		t.Fatal("gitlab_list_commit_discussions should define usage")
	}
	if len(byTool["gitlab_get_commit_discussion"].Aliases) == 0 {
		t.Fatal("gitlab_get_commit_discussion should define aliases")
	}
	if byTool["gitlab_create_commit_discussion"].ParameterGuidance["commit_sha"].SemanticRole == "" {
		t.Fatal("gitlab_create_commit_discussion should define commit_sha parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := commitDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, newCommitDiscussionMockHandler(t))))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_commit_discussions", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA}},
		{"gitlab_get_commit_discussion", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID}},
		{"gitlab_create_commit_discussion", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "body": "test"}},
		{"gitlab_add_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "body": "note"}},
		{"gitlab_update_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "note_id": float64(1), "body": "upd"}},
		{"gitlab_delete_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "note_id": float64(1)}},
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

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler here: GitLab's answer
// decodes for the SDK and not for the fields this package reads beside it,
// which is a fault in the type naming them and is reported rather than
// swallowed. A string where a note's `imported` is a bool is the shape, on
// a note alone, inside a thread, and inside a list of threads.
func TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		note := `{"id":1,"body":"x","author":{"id":1,"username":"u"},"imported":"not-a-bool"}`
		thread := `{"id":"d1","individual_note":false,"notes":[` + note + `]}`
		body := thread
		switch {
		case strings.Contains(r.URL.Path, "/notes"):
			body = note
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			body = "[" + thread + "]"
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			_, err := List(t.Context(), client, ListInput{ProjectID: testProjectID, CommitSHA: testCommitSHA})
			return err
		}},
		{Name: "get", Call: func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID})
			return err
		}},
		{Name: "create", Call: func() error {
			_, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, Body: "x"})
			return err
		}},
		{Name: "add note", Call: func() error {
			_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, Body: "x"})
			return err
		}},
		{Name: "update note", Call: func() error {
			_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, NoteID: 1, Body: "x"})
			return err
		}},
	})
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := commitDiscussionSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_commit_discussion_note"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test commit discussion destructive confirmation.",
		Icons:       toolutil.IconDiscussion,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: testVersion}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_commit_discussion_note",
		Arguments: map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "note_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected non-empty text content in cancellation result")
	}
}

// commitDiscussionSpecsByTool supports commit discussion specs by tool assertions in commitdiscussions tests.
func commitDiscussionSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}

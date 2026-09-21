// issue_discussions_test.go contains unit tests for the issue discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuediscussions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// testDiscussionID identifies the test discussion ID constant used by this package.
	testDiscussionID = "abc123"
	// testProjectID identifies the test project ID constant used by this package.
	testProjectID = "1"
	// testProjectPath identifies the test project path constant used by this package.
	testProjectPath = "my/project"
	// fmtIDWant identifies the fmt ID want constant used by this package.
	fmtIDWant = "ID = %q, want %q"
)

// The operation each handler signs its errors with. They are spelled out here
// rather than exported from the handler file so a test compares the message a
// caller reads against a name written down independently: sharing one
// constant with the code would make every crossing agree with itself.
const (
	opList       = "issue_discussion_list"
	opGet        = "issue_discussion_get"
	opCreate     = "issue_discussion_create"
	opAddNote    = "issue_discussion_add_note"
	opUpdateNote = "issue_discussion_update_note"
	opDeleteNote = "issue_discussion_delete_note"
)

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/issues/10/discussions (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/issues/10/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"Hello","author":{"username":"admin"},"created_at":"2026-01-01T00:00:00Z"}]}]`,
			testutil.PaginationHeaders{Page: "1", TotalPages: "1", Total: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 1 {
		t.Fatalf("got %d discussions, want 1", len(out.Discussions))
	}
	if out.Discussions[0].ID != testDiscussionID {
		t.Errorf(fmtIDWant, out.Discussions[0].ID, testDiscussionID)
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/issues/10/discussions/abc123 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/issues/10/discussions/abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"test","author":{"username":"user1"},"created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != testDiscussionID {
		t.Errorf(fmtIDWant, out.ID, testDiscussionID)
	}
}

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"new123","individual_note":false,"notes":[{"id":5,"body":"New thread","author":{"username":"admin"},"created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, IssueIID: 10, Body: "New thread"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "new123" {
		t.Errorf(fmtIDWant, out.ID, "new123")
	}
}

// TestAddNote_Success verifies that AddNote succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":99,"body":"Reply","author":{"username":"admin"},"created_at":"2026-01-01T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID, Body: "Reply"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 99 {
		t.Errorf("ID = %d, want 99", out.ID)
	}
}

// TestUpdateNote_Success verifies that UpdateNote succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":99,"body":"Updated","author":{"username":"admin"},"created_at":"2026-01-01T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 99, Body: "Updated"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Body != "Updated" {
		t.Errorf("body = %q, want %q", out.Body, "Updated")
	}
}

// TestDeleteNote_Success verifies that DeleteNote succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that a 204 is reported as success; the handler publishes no output.
func TestDeleteNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)

	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_AppliesListAndKeysetOptions verifies that List wires order_by,
// sort, page/per_page, and keyset pagination (pagination + page_token) into
// the outbound request query, covering the toolutil.ApplyListOptions path and
// the new 1:1-audit list fields.
func TestList_AppliesListAndKeysetOptions(t *testing.T) {
	var gotQuery url.Values
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{
		ProjectID: "42",
		IssueIID:  10,
		OrderBy:   "created_at",
		Sort:      "desc",
		Page:      3, PerPage: 25,
		Pagination: "keyset", PageToken: "cursor-99",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	wants := map[string]string{
		"order_by":   "created_at",
		"sort":       "desc",
		"page":       "3",
		"per_page":   "25",
		"pagination": "keyset",
		"page_token": "cursor-99",
	}
	for key, want := range wants {
		t.Run(key, func(t *testing.T) {
			if got := gotQuery.Get(key); got != want {
				t.Errorf("query %q = %q, want %q", key, got, want)
			}
		})
	}
}

// sentBody runs call against a client whose GitLab records the request body
// and answers with response, and returns that body decoded.
func sentBody(t *testing.T, response string, call func(*gitlabclient.Client) error) map[string]any {
	t.Helper()
	var raw []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "unreadable request body", http.StatusInternalServerError)
			return
		}
		raw = b
		testutil.RespondJSON(w, http.StatusOK, response)
	}))
	if err := call(client); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var sent map[string]any
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("request body %q is not a JSON object: %v", raw, err)
	}
	return sent
}

// TestHandlers_SendTheBodyAndTheBackdateApart asserts that each writing
// handler puts the caller's Markdown under body and the backdate under
// created_at, in their own keys.
//
// The tests this replaced looked for the timestamp anywhere in the request,
// which the two fields trading places satisfies just as well: both are
// optional strings on the input, so a handler sending the timestamp as the
// note's text and the text as the backdate passed: the note would read
// "2025-01-01T00:00:00Z" and the backdate would be dropped as unparseable.
// Neither gate can see it, because swapping two assignments changes no branch.
func TestHandlers_SendTheBodyAndTheBackdateApart(t *testing.T) {
	const (
		backdate = "2025-01-01T00:00:00Z"
		text     = "the note a caller wrote"
	)

	tests := []struct {
		name     string
		response string
		call     func(*gitlabclient.Client) error
	}{
		{"Create", discussionJSONCoverage, func(c *gitlabclient.Client) error {
			_, err := Create(context.Background(), c, CreateInput{
				ProjectID: testProjectID, IssueIID: 10, Body: text, CreatedAt: backdate,
			})
			return err
		}},
		{"AddNote", noteJSONCoverage, func(c *gitlabclient.Client) error {
			_, err := AddNote(context.Background(), c, AddNoteInput{
				ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID, Body: text, CreatedAt: backdate,
			})
			return err
		}},
		{"UpdateNote", noteJSONCoverage, func(c *gitlabclient.Client) error {
			_, err := UpdateNote(context.Background(), c, UpdateNoteInput{
				ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 99, Body: text, CreatedAt: backdate,
			})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent := sentBody(t, tt.response, tt.call)
			if sent["body"] != text {
				t.Errorf("body = %v, want %q", sent["body"], text)
			}
			if sent["created_at"] != backdate {
				t.Errorf("created_at = %v, want %q", sent["created_at"], backdate)
			}
		})
	}
}

// TestFormatListMarkdownString_Empty verifies that a list with no discussions
// renders the empty message alone: no heading, no pagination line and no
// guidance section, since there is nothing for a reader to act on.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md != "No issue discussions found.\n" {
		t.Errorf("got %q, want empty message", md)
	}
}

// ---------------------------------------------------------------------------
// assertRequiredField verifies that err is the refusal a handler writes for a
// missing required field: it opens with the operation that declined and the
// field it wanted, in that order.
//
// The operation is asserted, not just the field, because it is the only part
// of the message that says which of the six handlers refused. Every handler
// here writes the same four field names, so a message carrying a sibling's
// operation tells the caller its call went somewhere it never went, and no
// gate sees it: an operation label is a string literal, not a branch.
// ---------------------------------------------------------------------------.
func assertRequiredField(t *testing.T, err error, operation, field string) {
	t.Helper()
	want := operation + ": " + field + " is required"
	if err == nil {
		t.Fatalf("expected error starting with %q, got nil", want)
	}
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err.Error(), want)
	}
}

// TestIssueIIDRequired_Validation asserts that every handler refuses an
// omitted issue_iid itself, naming its own operation and the field, and that
// none of them reaches GitLab: the mock is a [testutil.ForbiddenHandler], so
// a guard that let the zero through would fail the subtest twice over.
func TestIssueIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = testProjectPath

	tests := []struct {
		name string
		op   string
		fn   func() error
	}{
		{"List", opList, func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Get", opGet, func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID})
			return e
		}},
		{"Create", opCreate, func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, IssueIID: 0, Body: "x"})
			return e
		}},
		{"AddNote", opAddNote, func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, Body: "x"})
			return e
		}},
		{"UpdateNote", opUpdateNote, func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", opDeleteNote, func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRequiredField(t, tt.fn(), tt.op, "issue_iid")
		})
	}
}

// TestNoteIDRequired_Validation asserts that both handlers taking a note_id
// name themselves and the missing field, for the value an omitted field
// decodes to (zero) as well as for a negative one, and that neither reaches
// GitLab: the mock is a [testutil.ForbiddenHandler], so a guard that let
// either value through would fail the subtest and the no-request assertion
// alike.
//
// Each handler is held at both values because the guard is a boundary, and a
// test that pins only one side of it leaves the other free to move. An MCP
// caller who leaves note_id out sends the zero, never a negative, so a guard
// narrowed to "< 0" would send GitLab a request against note 0 and answer the
// model with a remote 404 about permissions instead of naming the field it
// forgot — while a suite checking DeleteNote at -1 alone stayed green.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = testProjectPath

	updateNote := func(noteID int64) error {
		_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: noteID, Body: "x"})
		return e
	}
	deleteNote := func(noteID int64) error {
		return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: noteID})
	}

	tests := []struct {
		name   string
		op     string
		fn     func(int64) error
		noteID int64
	}{
		{"UpdateNote_Omitted", opUpdateNote, updateNote, 0},
		{"UpdateNote_Negative", opUpdateNote, updateNote, -1},
		{"DeleteNote_Omitted", opDeleteNote, deleteNote, 0},
		{"DeleteNote_Negative", opDeleteNote, deleteNote, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRequiredField(t, tt.fn(tt.noteID), tt.op, "note_id")
		})
	}
}

// TestProjectIDRequired_Validation asserts that every handler refuses an
// omitted project_id itself, naming its own operation and the field, without
// reaching GitLab.
func TestProjectIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	tests := []struct {
		name string
		op   string
		fn   func() error
	}{
		{"List", opList, func() error { _, e := List(ctx, client, ListInput{IssueIID: 10}); return e }},
		{"Get", opGet, func() error {
			_, e := Get(ctx, client, GetInput{IssueIID: 10, DiscussionID: testDiscussionID})
			return e
		}},
		{"Create", opCreate, func() error { _, e := Create(ctx, client, CreateInput{IssueIID: 10, Body: "x"}); return e }},
		{"AddNote", opAddNote, func() error {
			_, e := AddNote(ctx, client, AddNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, Body: "x"})
			return e
		}},
		{"UpdateNote", opUpdateNote, func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", opDeleteNote, func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRequiredField(t, tt.fn(), tt.op, "project_id")
		})
	}
}

// TestDiscussionIDRequired_Validation asserts that every discussion-scoped
// handler refuses an omitted discussion_id itself, naming its own operation
// and the field, without reaching GitLab.
func TestDiscussionIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = testProjectPath

	tests := []struct {
		name string
		op   string
		fn   func() error
	}{
		{"Get", opGet, func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 10}); return e }},
		{"AddNote", opAddNote, func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, IssueIID: 10, Body: "x"})
			return e
		}},
		{"UpdateNote", opUpdateNote, func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 10, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", opDeleteNote, func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 10, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRequiredField(t, tt.fn(), tt.op, "discussion_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Format*Markdown tests — populated + empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_Populated renders two threads of three notes
// between them and asserts the heading, every thread id, every author, every
// body and every timestamp reach the page, and that it closes with this
// formatter's own guidance.
func TestFormatListMarkdownString_Populated(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{
				ID:             "disc1",
				IndividualNote: false,
				Notes: []*NoteOutput{
					{ID: 1, Body: "First note", Author: &toolutil.NoteUserOutput{Username: "alice"}, CreatedAt: "2026-01-01T00:00:00Z"},
					{ID: 2, Body: "Second note", Author: &toolutil.NoteUserOutput{Username: "bob"}, CreatedAt: "2026-01-02T00:00:00Z"},
				},
			},
			{
				ID:             "disc2",
				IndividualNote: true,
				Notes: []*NoteOutput{
					{ID: 3, Body: "Solo note", Author: &toolutil.NoteUserOutput{Username: "carol"}, CreatedAt: "2026-01-03T00:00:00Z"},
				},
			},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdownString(out)
	for _, want := range []string{
		"Issue Discussions (2)",
		"disc1", "disc2",
		"@alice", "@bob", "@carol",
		"First note", "Second note", "Solo note",
		"1 Jan 2026 00:00 UTC", "2 Jan 2026 00:00 UTC", "3 Jan 2026 00:00 UTC",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("FormatListMarkdownString missing %q", want)
			}
		})
	}
	t.Run("hints", func(t *testing.T) {
		if !strings.HasSuffix(md, discussionListHints) {
			t.Errorf("list guidance:\n got %q\nwant suffix %q", md, discussionListHints)
		}
	})
}

// discussionListHints is the guidance section the issue discussion list ends
// with, in the order the formatter passes it. It is asserted whole because a
// hint is a string literal no gate reads: the two could trade places, or one
// could be the thread card's, and every mutant would still die.
const discussionListHints = "\n\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'discussion_get' with discussion_id to see full discussion\n" +
	"- Use action 'discussion_add_note' to reply to a discussion\n"

// discussionCardHints is the same for the single-thread card, whose two hints
// are deliberately not the list's: a reader holding one thread is told how to
// reply to it and how to edit a note in it, not how to fetch it again.
const discussionCardHints = "\n\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'discussion_add_note' to reply to this discussion\n" +
	"- Use action 'discussion_update_note' with note_id to edit a note\n"

// TestFormatMarkdownString_Populated renders one thread and asserts its
// heading, both authors, both bodies and both timestamps reach the card, and
// that it closes with the thread card's own guidance rather than the list's.
func TestFormatMarkdownString_Populated(t *testing.T) {
	out := Output{
		ID:             "disc-abc",
		IndividualNote: false,
		Notes: []*NoteOutput{
			{ID: 10, Body: "Hello world", Author: &toolutil.NoteUserOutput{Username: "alice"}, CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 11, Body: "Reply here", Author: &toolutil.NoteUserOutput{Username: "bob"}, CreatedAt: "2026-01-02T00:00:00Z"},
		},
	}
	md := FormatMarkdownString(out)
	for _, want := range []string{
		"Discussion disc-abc",
		"@alice", "@bob",
		"Hello world", "Reply here",
		"1 Jan 2026 00:00 UTC", "2 Jan 2026 00:00 UTC",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("FormatMarkdownString missing %q", want)
			}
		})
	}
	t.Run("hints", func(t *testing.T) {
		if !strings.HasSuffix(md, discussionCardHints) {
			t.Errorf("thread card guidance:\n got %q\nwant suffix %q", md, discussionCardHints)
		}
	})
}

// TestFormatMarkdownString_Empty verifies that a zero thread still renders the
// card's heading, so a caller reading the result sees what it is looking at
// rather than an empty response.
func TestFormatMarkdownString_Empty(t *testing.T) {
	md := FormatMarkdownString(Output{})
	if !strings.Contains(md, "Discussion") {
		t.Error("FormatMarkdownString should contain Discussion header for empty output")
	}
}

// TestFormatNoteMarkdownString_Populated renders one note and asserts the card
// whole (heading, author, time, body and the two hints in order) because a
// card assembled from the right pieces in the wrong places passes every
// substring check.
func TestFormatNoteMarkdownString_Populated(t *testing.T) {
	out := NoteOutput{
		ID:        42,
		Body:      "Great work!",
		Author:    &toolutil.NoteUserOutput{Username: "reviewer"},
		CreatedAt: "2026-03-01T10:00:00Z",
	}
	md := FormatNoteMarkdownString(out)
	want := "## Discussion Note #42\n\n" +
		"- **Author**: @reviewer\n" +
		"- **Created**: 1 Mar 2026 10:00 UTC\n" +
		"- **Body**: Great work!\n" +
		discussionNoteCardHints
	if md != want {
		t.Errorf("note card:\n got %q\nwant %q", md, want)
	}
}

// discussionNoteCardHints is the guidance section every issue discussion
// note card ends with.
const discussionNoteCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'discussion_update_note' with note_id to edit this note\n" +
	"- Use action 'discussion_delete_note' with note_id to remove this note\n"

// TestFormatNoteMarkdownString_Empty verifies that a zero note renders the
// heading and the hints alone: no author, time or body row is written for a
// value GitLab did not send.
func TestFormatNoteMarkdownString_Empty(t *testing.T) {
	md := FormatNoteMarkdownString(NoteOutput{})
	want := "## Discussion Note #0\n\n" + strings.TrimPrefix(discussionNoteCardHints, "\n")
	if md != want {
		t.Errorf("empty note card:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// Converter tests — noteToOutput, toOutput, toListOutput
// ---------------------------------------------------------------------------.

// TestNoteToOutput_AllFields drives Create against a thread whose single note
// carries every field GitLab sends, and asserts each one arrives on the
// published note: id, body, author, the system flag and both timestamps.
func TestNoteToOutput_AllFields(t *testing.T) {
	// Exercise noteToOutput via Create which returns noteToOutput(note).
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
"id":"d1",
"individual_note":false,
"notes":[{
"id":500,
"body":"Full note",
"author":{"username":"alice"},
"system":true,
"created_at":"2026-01-15T10:30:00Z",
"updated_at":"2026-01-16T11:00:00Z"
}]
}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(out.Notes))
	}
	n := out.Notes[0]
	if n.ID != 500 {
		t.Errorf("ID = %d, want 500", n.ID)
	}
	if n.Body != "Full note" {
		t.Errorf("Body = %q, want %q", n.Body, "Full note")
	}
	if n.Author == nil || n.Author.Username != "alice" {
		t.Errorf("Author = %+v, want username %q", n.Author, "alice")
	}
	if !n.System {
		t.Error("System = false, want true")
	}
	if n.CreatedAt != "2026-01-15T10:30:00Z" {
		t.Errorf("CreatedAt = %q, want %q", n.CreatedAt, "2026-01-15T10:30:00Z")
	}
	if n.UpdatedAt != "2026-01-16T11:00:00Z" {
		t.Errorf("UpdatedAt = %q, want %q", n.UpdatedAt, "2026-01-16T11:00:00Z")
	}
}

// TestNoteToOutput_NoUpdatedAt asserts that a note GitLab sends without
// updated_at publishes an empty one rather than borrowing created_at, and
// that the fields beside it still arrive.
func TestNoteToOutput_NoUpdatedAt(t *testing.T) {
	// GitLab always returns created_at; updated_at may be absent.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":"ts001",
			"individual_note":false,
			"notes":[{
				"id":400,
				"body":"no updated_at",
				"author":{"id":1,"username":"tester"},
				"system":false,
				"created_at":"2026-01-15T10:30:00Z"
			}]
		}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "no updated_at"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	n := out.Notes[0]
	if n.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty when not provided", n.UpdatedAt)
	}
	if n.Author == nil || n.Author.Username != "tester" {
		t.Errorf("Author = %+v, want username %q", n.Author, "tester")
	}
	if n.CreatedAt != "2026-01-15T10:30:00Z" {
		t.Errorf("CreatedAt = %q, want %q", n.CreatedAt, "2026-01-15T10:30:00Z")
	}
}

// TestNoteToOutput_EmptyAuthor asserts that the author object GitLab sends
// empty on a system note is published as an empty object rather than dropped,
// so a reader dereferencing it does not meet a nil.
func TestNoteToOutput_EmptyAuthor(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
"id":"ea001",
"individual_note":false,
"notes":[{
"id":401,
"body":"system note",
"author":{},
"system":true,
"created_at":"2026-01-01T00:00:00Z"
}]
}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "system note"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	n := out.Notes[0]
	// The author object is always present on a note; an empty one has no
	// username.
	if n.Author == nil || n.Author.Username != "" {
		t.Errorf("Author = %+v, want an empty author object", n.Author)
	}
}

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler here: GitLab's answer
// decodes for the SDK and not for the fields this package reads beside it,
// which is a fault in the type naming them and is reported rather than
// swallowed. A string where a note's `imported` is a bool is the shape, on
// a note alone, inside a thread, and inside a list of threads.
//
// Each refusal is then held to naming the handler that made the call. The
// operation reaches [toolutil.CapturedThread] and its two siblings as an
// argument, so it is an assignment rather than a branch and no gate can be
// wrong about it, while a caller reading "issue_discussion_get" after asking
// for a list is told about a call it never made.
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
	calls := []struct {
		name string
		op   string
		call func() error
	}{
		{"list", opList, func() error {
			_, err := List(t.Context(), client, ListInput{ProjectID: "42", IssueIID: 10})
			return err
		}},
		{"get", opGet, func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectID: "42", IssueIID: 10, DiscussionID: "d1"})
			return err
		}},
		{"create", opCreate, func() error {
			_, err := Create(t.Context(), client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
			return err
		}},
		{"add note", opAddNote, func() error {
			_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "d1", Body: "x"})
			return err
		}},
		{"update note", opUpdateNote, func() error {
			_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "d1", NoteID: 1, Body: "x"})
			return err
		}},
	}

	cases := make([]testutil.CapturedCase, 0, len(calls))
	for _, c := range calls {
		cases = append(cases, testutil.CapturedCase{Name: c.name, Call: c.call})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)

	for _, c := range calls {
		t.Run(c.name+" names its operation", func(t *testing.T) {
			err := c.call()
			if err == nil {
				t.Fatalf("expected the capture's decode failure from %s, got nil", c.op)
			}
			if !strings.HasPrefix(err.Error(), c.op+": ") {
				t.Errorf("error %q does not name the operation %q", err.Error(), c.op)
			}
		})
	}
}

// TestToOutput_MultipleNotes asserts that a thread of three notes reaches the
// caller whole and in order: the thread's id and individual_note flag, the
// note count, and the authors at both ends of the slice.
func TestToOutput_MultipleNotes(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
"id":"multi001",
"individual_note":true,
"notes":[
{"id":1,"body":"First","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"},
{"id":2,"body":"Second","author":{"username":"bob"},"created_at":"2026-01-02T00:00:00Z"},
{"id":3,"body":"Third","author":{"username":"carol"},"created_at":"2026-01-03T00:00:00Z"}
]
}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: "42", IssueIID: 10, DiscussionID: "multi001"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "multi001" {
		t.Errorf("ID = %q, want %q", out.ID, "multi001")
	}
	if !out.IndividualNote {
		t.Error("IndividualNote = false, want true")
	}
	if len(out.Notes) != 3 {
		t.Fatalf("got %d notes, want 3", len(out.Notes))
	}
	if out.Notes[0].Author == nil || out.Notes[0].Author.Username != "alice" {
		t.Errorf("Notes[0].Author = %+v, want username %q", out.Notes[0].Author, "alice")
	}
	if out.Notes[2].Author == nil || out.Notes[2].Author.Username != "carol" {
		t.Errorf("Notes[2].Author = %+v, want username %q", out.Notes[2].Author, "carol")
	}
}

// TestToListOutput_MultipleDiscussions asserts that a page of two threads
// keeps its order and that the pagination block is filled from GitLab's own
// headers rather than from the page's length.
func TestToListOutput_MultipleDiscussions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[
{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"A","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}]},
{"id":"d2","individual_note":true,"notes":[{"id":2,"body":"B","author":{"username":"bob"},"created_at":"2026-01-02T00:00:00Z"}]}
]`,
			testutil.PaginationHeaders{Page: "1", TotalPages: "2", Total: "5", PerPage: "2", NextPage: "2"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{
		ProjectID: "42",
		IssueIID:  10,
		Page:      1, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 2 {
		t.Fatalf("got %d discussions, want 2", len(out.Discussions))
	}
	if out.Discussions[0].ID != "d1" {
		t.Errorf("Discussions[0].ID = %q, want %q", out.Discussions[0].ID, "d1")
	}
	if out.Discussions[1].ID != "d2" {
		t.Errorf("Discussions[1].ID = %q, want %q", out.Discussions[1].ID, "d2")
	}
	if out.Pagination.TotalItems != 5 {
		t.Errorf("Pagination.TotalItems = %d, want 5", out.Pagination.TotalItems)
	}
}

// TestToListOutput_EmptyList asserts that an issue with no discussions is
// answered with an empty list rather than an error.
func TestToListOutput_EmptyList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", TotalPages: "1", Total: "0", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{ProjectID: "42", IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 0 {
		t.Errorf("got %d discussions, want 0", len(out.Discussions))
	}
}

// ---------------------------------------------------------------------------
// Context cancellation for all 6 handlers
// ---------------------------------------------------------------------------.

// TestHandlers_CancelledContext_RefuseBeforeTheCall asserts that every handler
// checks the context first and hands the caller the cancellation unchanged.
//
// The mock is a [testutil.ForbiddenHandler], so nothing may reach GitLab, and
// the error must be [context.Canceled] itself rather than an operation-signed
// wrap. The second half is what the tests this replaced were missing: they
// asserted only that some error came back, which a handler that skipped the
// guard would produce anyway: the transport refuses a canceled request on its
// own, and the handler would then report it as a GitLab failure, telling the
// caller its issue or thread could not be found when the call was simply
// abandoned.
func TestHandlers_CancelledContext_RefuseBeforeTheCall(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{"List", func(ctx context.Context) error {
			_, err := List(ctx, client, ListInput{ProjectID: "42", IssueIID: 10})
			return err
		}},
		{"Get", func(ctx context.Context) error {
			_, err := Get(ctx, client, GetInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID})
			return err
		}},
		{"Create", func(ctx context.Context) error {
			_, err := Create(ctx, client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
			return err
		}},
		{"AddNote", func(ctx context.Context) error {
			_, err := AddNote(ctx, client, AddNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, Body: "x"})
			return err
		}},
		{"UpdateNote", func(ctx context.Context) error {
			_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 100, Body: "x"})
			return err
		}},
		{"DeleteNote", func(ctx context.Context) error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 100})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(testutil.CancelledCtx(t))
			if err == nil {
				t.Fatal("expected context.Canceled error, got nil")
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want context.Canceled", err)
			}
			if err.Error() != context.Canceled.Error() {
				t.Errorf("error %q was reported as a GitLab failure; want the bare cancellation", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// API error paths for all 6 handlers
// ---------------------------------------------------------------------------.

// apiErrorCase is one handler's refusal contract: the operation it signs its
// errors with, the status it classifies with a hint of its own, the hint it
// attaches there, a status it does not classify, and the call that drives it
// against a client whose GitLab answers a given status.
type apiErrorCase struct {
	name     string
	op       string
	hint     string
	call     func(*gitlabclient.Client) error
	hinted   int
	unhinted int
}

// apiErrorCases returns that contract for each of the six handlers.
func apiErrorCases() []apiErrorCase {
	return []apiErrorCase{
		{
			name: "List", op: opList,
			hinted: http.StatusNotFound, unhinted: http.StatusForbidden,
			hint: "verify project_id and issue_iid with gitlab_issue_get",
			call: func(c *gitlabclient.Client) error {
				_, err := List(context.Background(), c, ListInput{ProjectID: "42", IssueIID: 10})
				return err
			},
		},
		{
			name: "Get", op: opGet,
			hinted: http.StatusNotFound, unhinted: http.StatusForbidden,
			hint: "verify discussion_id with gitlab_list_issue_discussions",
			call: func(c *gitlabclient.Client) error {
				_, err := Get(context.Background(), c, GetInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID})
				return err
			},
		},
		{
			name: "Create", op: opCreate,
			hinted: http.StatusNotFound, unhinted: http.StatusForbidden,
			hint: "verify project_id and issue_iid with gitlab_issue_get; creating discussions requires Reporter role or higher",
			call: func(c *gitlabclient.Client) error {
				_, err := Create(context.Background(), c, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
				return err
			},
		},
		{
			name: "AddNote", op: opAddNote,
			hinted: http.StatusNotFound, unhinted: http.StatusForbidden,
			hint: "verify discussion_id with gitlab_list_issue_discussions",
			call: func(c *gitlabclient.Client) error {
				_, err := AddNote(context.Background(), c, AddNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, Body: "x"})
				return err
			},
		},
		{
			name: "UpdateNote", op: opUpdateNote,
			hinted: http.StatusForbidden, unhinted: http.StatusNotFound,
			hint: "only the note author can edit a discussion note",
			call: func(c *gitlabclient.Client) error {
				_, err := UpdateNote(context.Background(), c, UpdateNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 100, Body: "x"})
				return err
			},
		},
		{
			name: "DeleteNote", op: opDeleteNote,
			hinted: http.StatusForbidden, unhinted: http.StatusNotFound,
			hint: "only the note author or a Maintainer can delete a discussion note",
			call: func(c *gitlabclient.Client) error {
				return DeleteNote(context.Background(), c, DeleteNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 100})
			},
		},
	}
}

// statusClient returns a client whose GitLab answers every request with code.
func statusClient(t *testing.T, code int) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, code, `{"message":"refused"}`)
	}))
}

// TestHandlers_APIError_CarryTheirOwnOperationAndHint asserts what a caller
// actually reads when GitLab refuses: the error opens with the operation that
// made the call, and it carries that handler's own suggestion at the status
// that handler classifies: 404 for the four reads and writes that can be
// pointed at the wrong project, issue or thread, 403 for the two note actions
// GitLab refuses on authorship.
//
// The second half of each case is what makes the first half mean something. A
// handler that classified any status would attach its hint to a refusal it
// knows nothing about, so the same call is driven at a status it does not
// classify and the hint must be absent there.
//
// Both halves are invisible to the mutation and condition gates, which see a
// branch taken and not the literal it hands the caller: the six hints could be
// dealt out to the wrong handlers, or the two status constants swapped, with
// every gate still green. The suite this replaced asserted only that an error
// came back.
func TestHandlers_APIError_CarryTheirOwnOperationAndHint(t *testing.T) {
	for _, tt := range apiErrorCases() {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(statusClient(t, tt.hinted))
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.HasPrefix(err.Error(), tt.op+": ") {
				t.Errorf("error %q does not name the operation %q", err.Error(), tt.op)
			}
			if !strings.Contains(err.Error(), "Suggestion: "+tt.hint) {
				t.Errorf("error %q missing its own hint %q", err.Error(), tt.hint)
			}

			other := tt.call(statusClient(t, tt.unhinted))
			if other == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.HasPrefix(other.Error(), tt.op+": ") {
				t.Errorf("error %q does not name the operation %q", other.Error(), tt.op)
			}
			if strings.Contains(other.Error(), tt.hint) {
				t.Errorf("error %q carries the %d hint on a %d refusal", other.Error(), tt.hinted, tt.unhinted)
			}
		})
	}
}

const (
	// errExpectedAPI identifies the err expected API constant used by this package.
	errExpectedAPI = "expected API error, got nil"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// discussionJSONCoverage identifies the discussion JSON coverage constant used by this package.
	discussionJSONCoverage = `{
"id":"abc123",
"individual_note":false,
"notes":[{
"id":300,
"body":"comment",
"author":{"id":1,"username":"jmrplens"},
"created_at":"2026-03-02T12:00:00Z",
"updated_at":"2026-03-02T12:00:00Z"
}]
}`

	// noteJSONCoverage identifies the note JSON coverage constant used by this package.
	noteJSONCoverage = `{
"id":300,
"body":"reply",
"author":{"id":1,"username":"jmrplens"},
"created_at":"2026-03-02T12:00:00Z",
"updated_at":"2026-03-02T12:00:00Z"
}`
)

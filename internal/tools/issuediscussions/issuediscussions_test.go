// issuediscussions_test.go contains unit tests for the issue discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuediscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testDiscussionID = "abc123"
	testProjectID    = "1"
	testProjectPath  = "my/project"
	fmtIDWant        = "ID = %q, want %q"
)

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/issues/10/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"Hello","author":{"username":"admin"},"created_at":"2024-01-01T00:00:00Z"}]}]`,
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

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/issues/10/discussions/abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"test","author":{"username":"user1"},"created_at":"2024-01-01T00:00:00Z"}]}`)
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

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"new123","individual_note":false,"notes":[{"id":5,"body":"New thread","author":{"username":"admin"},"created_at":"2024-01-01T00:00:00Z"}]}`)
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

// TestAddNote_Success verifies the behavior of add note success.
func TestAddNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":99,"body":"Reply","author":{"username":"admin"},"created_at":"2024-01-01T00:00:00Z"}`)
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

// TestUpdateNote_Success verifies the behavior of update note success.
func TestUpdateNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":99,"body":"Updated","author":{"username":"admin"},"created_at":"2024-01-01T00:00:00Z"}`)
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

// TestDeleteNote_Success verifies the behavior of delete note success.
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

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, IssueIID: 10, DiscussionID: testDiscussionID})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md != "No issue discussions found.\n" {
		t.Errorf("got %q, want empty message", md)
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestIssueIIDRequired_Validation ensures all handlers reject zero/negative issue_iid.
func TestIssueIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_iid is invalid")
	}))
	ctx := context.Background()
	const pid = testProjectPath

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID})
			return e
		}},
		{"Create", func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, IssueIID: 0, Body: "x"})
			return e
		}},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 0, DiscussionID: testDiscussionID, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "issue_iid")
		})
	}
}

// TestNoteIDRequired_Validation ensures UpdateNote and DeleteNote reject zero/negative note_id.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when note_id is invalid")
	}))
	ctx := context.Background()
	const pid = testProjectPath

	tests := []struct {
		name string
		fn   func() error
	}{
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 0, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 10, DiscussionID: testDiscussionID, NoteID: -1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "note_id")
		})
	}
}

// TestProjectIDRequired_Validation ensures all handlers reject empty project_id.
func TestProjectIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when project_id is empty")
	}))
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{IssueIID: 10}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{IssueIID: 10, DiscussionID: testDiscussionID})
			return e
		}},
		{"Create", func() error { _, e := Create(ctx, client, CreateInput{IssueIID: 10, Body: "x"}); return e }},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{IssueIID: 10, DiscussionID: testDiscussionID, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "project_id")
		})
	}
}

// TestDiscussionIDRequired_Validation ensures Get/AddNote/UpdateNote/DeleteNote reject empty discussion_id.
func TestDiscussionIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when discussion_id is empty")
	}))
	ctx := context.Background()
	const pid = testProjectPath

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 10}); return e }},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, IssueIID: 10, Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, IssueIID: 10, NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, IssueIID: 10, NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "discussion_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Format*Markdown tests — populated + empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_Populated verifies the behavior of format list markdown string populated.
func TestFormatListMarkdownString_Populated(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{
				ID:             "disc1",
				IndividualNote: false,
				Notes: []NoteOutput{
					{ID: 1, Body: "First note", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
					{ID: 2, Body: "Second note", Author: "bob", CreatedAt: "2026-01-02T00:00:00Z"},
				},
			},
			{
				ID:             "disc2",
				IndividualNote: true,
				Notes: []NoteOutput{
					{ID: 3, Body: "Solo note", Author: "carol", CreatedAt: "2026-01-03T00:00:00Z"},
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
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdownString missing %q", want)
		}
	}
}

// TestFormatListMarkdown_ReturnsCallToolResult verifies the behavior of format list markdown returns call tool result.
func TestFormatListMarkdown_ReturnsCallToolResult(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil")
	}
}

// TestFormatMarkdownString_Populated verifies the behavior of format markdown string populated.
func TestFormatMarkdownString_Populated(t *testing.T) {
	out := Output{
		ID:             "disc-abc",
		IndividualNote: false,
		Notes: []NoteOutput{
			{ID: 10, Body: "Hello world", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 11, Body: "Reply here", Author: "bob", CreatedAt: "2026-01-02T00:00:00Z"},
		},
	}
	md := FormatMarkdownString(out)
	for _, want := range []string{
		"Discussion disc-abc",
		"@alice", "@bob",
		"Hello world", "Reply here",
		"1 Jan 2026 00:00 UTC", "2 Jan 2026 00:00 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdownString missing %q", want)
		}
	}
}

// TestFormatMarkdownString_Empty verifies the behavior of format markdown string empty.
func TestFormatMarkdownString_Empty(t *testing.T) {
	md := FormatMarkdownString(Output{})
	if !strings.Contains(md, "Discussion") {
		t.Error("FormatMarkdownString should contain Discussion header for empty output")
	}
}

// TestFormatMarkdown_ReturnsCallToolResult verifies the behavior of format markdown returns call tool result.
func TestFormatMarkdown_ReturnsCallToolResult(t *testing.T) {
	result := FormatMarkdown(Output{ID: "x"})
	if result == nil {
		t.Fatal("FormatMarkdown returned nil")
	}
}

// TestFormatNoteMarkdownString_Populated verifies the behavior of format note markdown string populated.
func TestFormatNoteMarkdownString_Populated(t *testing.T) {
	out := NoteOutput{
		ID:        42,
		Body:      "Great work!",
		Author:    "reviewer",
		CreatedAt: "2026-03-01T10:00:00Z",
	}
	md := FormatNoteMarkdownString(out)
	for _, want := range []string{
		"## Note",
		"42",
		"@reviewer",
		"Great work!",
		"1 Mar 2026 10:00 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatNoteMarkdownString missing %q", want)
		}
	}
}

// TestFormatNoteMarkdownString_Empty verifies the behavior of format note markdown string empty.
func TestFormatNoteMarkdownString_Empty(t *testing.T) {
	md := FormatNoteMarkdownString(NoteOutput{})
	if !strings.Contains(md, "## Note") {
		t.Error("FormatNoteMarkdownString should contain Note header for empty output")
	}
	// CreatedAt is empty, so "Created" line should not appear.
	if strings.Contains(md, "**Created**") {
		t.Error("should not contain Created line when CreatedAt is empty")
	}
}

// TestFormatNoteMarkdown_ReturnsCallToolResult verifies the behavior of format note markdown returns call tool result.
func TestFormatNoteMarkdown_ReturnsCallToolResult(t *testing.T) {
	result := FormatNoteMarkdown(NoteOutput{ID: 1, Body: "x", Author: "u"})
	if result == nil {
		t.Fatal("FormatNoteMarkdown returned nil")
	}
}

// ---------------------------------------------------------------------------
// Converter tests — noteToOutput, toOutput, toListOutput
// ---------------------------------------------------------------------------.

// TestNoteToOutput_AllFields verifies the behavior of note to output all fields.
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
	if n.Author != "alice" {
		t.Errorf("Author = %q, want %q", n.Author, "alice")
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

// TestNoteToOutput_NoUpdatedAt verifies the behavior of note to output no updated at.
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
	if n.Author != "tester" {
		t.Errorf("Author = %q, want %q", n.Author, "tester")
	}
	if n.CreatedAt != "2026-01-15T10:30:00Z" {
		t.Errorf("CreatedAt = %q, want %q", n.CreatedAt, "2026-01-15T10:30:00Z")
	}
}

// TestNoteToOutput_EmptyAuthor verifies the behavior of note to output empty author.
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
	if n.Author != "" {
		t.Errorf("Author = %q, want empty for empty author", n.Author)
	}
}

// TestToOutput_MultipleNotes verifies the behavior of to output multiple notes.
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
	if out.Notes[0].Author != "alice" {
		t.Errorf("Notes[0].Author = %q, want %q", out.Notes[0].Author, "alice")
	}
	if out.Notes[2].Author != "carol" {
		t.Errorf("Notes[2].Author = %q, want %q", out.Notes[2].Author, "carol")
	}
}

// TestToListOutput_MultipleDiscussions verifies the behavior of to list output multiple discussions.
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

	out, err := List(t.Context(), client, ListInput{ProjectID: "42", IssueIID: 10, Page: 1, PerPage: 2})
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

// TestToListOutput_EmptyList verifies the behavior of to list output empty list.
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

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "42", IssueIID: 10})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(ctx, client, GetInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestAddNote_CancelledContext verifies the behavior of add note cancelled context.
func TestAddNote_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddNote(ctx, client, AddNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestUpdateNote_CancelledContext verifies the behavior of update note cancelled context.
func TestUpdateNote_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestDeleteNote_CancelledContext verifies the behavior of delete note cancelled context.
func TestDeleteNote_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteNote(ctx, client, DeleteNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", NoteID: 100})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// API error paths for all 6 handlers
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "42", IssueIID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "42", IssueIID: 10, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddNote_APIError verifies the behavior of add note a p i error.
func TestAddNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateNote_APIError verifies the behavior of update note a p i error.
func TestUpdateNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteNote_APIError verifies the behavior of delete note a p i error.
func TestDeleteNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: "42", IssueIID: 10, DiscussionID: "abc123", NoteID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools MCP integration test
// ---------------------------------------------------------------------------.

const (
	errExpectedAPI         = "expected API error, got nil"
	fmtUnexpErr            = "unexpected error: %v"
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

	noteJSONCoverage = `{
"id":300,
"body":"reply",
"author":{"id":1,"username":"jmrplens"},
"created_at":"2026-03-02T12:00:00Z",
"updated_at":"2026-03-02T12:00:00Z"
}`
)

// newIssueDiscussionsMCPSession is an internal helper for the issuediscussions package.
func newIssueDiscussionsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		// POST .../discussions → create discussion.
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusCreated, discussionJSONCoverage)

		// POST .../discussions/{id}/notes → add note.
		case r.Method == http.MethodPost && strings.Contains(path, "/discussions/") && strings.HasSuffix(path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, noteJSONCoverage)

		// PUT .../discussions/{id}/notes/{noteID} → update note.
		case r.Method == http.MethodPut && strings.Contains(path, "/notes/"):
			testutil.RespondJSON(w, http.StatusOK, noteJSONCoverage)

		// DELETE .../discussions/{id}/notes/{noteID} → delete note.
		case r.Method == http.MethodDelete && strings.Contains(path, "/notes/"):
			w.WriteHeader(http.StatusNoContent)

		// GET .../discussions/{id} → get single discussion.
		case r.Method == http.MethodGet && strings.Contains(path, "/discussions/"):
			testutil.RespondJSON(w, http.StatusOK, discussionJSONCoverage)

		// GET .../discussions → list discussions.
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/discussions"):
			testutil.RespondJSON(w, http.StatusOK, "["+discussionJSONCoverage+"]")

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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newIssueDiscussionsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_issue_discussions", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_get_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": "abc123"}},
		{"gitlab_create_issue_discussion", map[string]any{"project_id": "42", "issue_iid": 10, "body": "New discussion"}},
		{"gitlab_add_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": "abc123", "body": "Reply"}},
		{"gitlab_update_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": "abc123", "note_id": 300, "body": "Updated"}},
		{"gitlab_delete_issue_discussion_note", map[string]any{"project_id": "42", "issue_iid": 10, "discussion_id": "abc123", "note_id": 300}},
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

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// snippetdiscussions_test.go contains unit tests for the snippet discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package snippetdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/snippets/5/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"snippet note","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}]}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{ProjectID: "1", SnippetID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 1 {
		t.Fatalf("got %d discussions, want 1", len(out.Discussions))
	}
	if out.Discussions[0].ID != "d1" {
		t.Errorf("got ID=%q, want d1", out.Discussions[0].ID)
	}
}

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{ProjectID: "1", SnippetID: 5})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/snippets/5/discussions/d1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"d1","individual_note":true,"notes":[{"id":10,"body":"test","author":{"username":"bob"},"created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "d1" {
		t.Errorf("got ID=%q, want d1", out.ID)
	}
}

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"d2","individual_note":false,"notes":[{"id":20,"body":"new","author":{"username":"carol"},"created_at":"2026-01-02T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", SnippetID: 5, Body: "new"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != "d2" {
		t.Errorf("got ID=%q, want d2", out.ID)
	}
}

// TestAddNote_Success verifies AddNote when success.
func TestAddNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":30,"body":"reply","author":{"username":"dave"},"created_at":"2026-01-03T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", Body: "reply"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 30 {
		t.Errorf("got ID=%d, want 30", out.ID)
	}
}

// TestUpdateNote_Success verifies UpdateNote when success.
func TestUpdateNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":30,"body":"updated","author":{"username":"dave"},"created_at":"2026-01-03T00:00:00Z","updated_at":"2026-01-04T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 30, Body: "updated"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Body != "updated" {
		t.Errorf("got body=%q, want updated", out.Body)
	}
}

// TestDeleteNote_Success verifies DeleteNote when success.
func TestDeleteNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)

	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 30})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteNote_APIError verifies DeleteNote when API error.
func TestDeleteNote_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	err := DeleteNote(t.Context(), client, DeleteNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 30})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// assertContains verifies that err is non-nil and its message contains substr.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestSnippetIDRequired_Validation ensures all handlers reject zero/negative snippet_id.
func TestSnippetIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when snippet_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, SnippetID: 0}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, SnippetID: 0, DiscussionID: "abc"})
			return e
		}},
		{"Create", func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, SnippetID: 0, Body: "x"})
			return e
		}},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, SnippetID: 0, DiscussionID: "abc", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 0, DiscussionID: "abc", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, SnippetID: 0, DiscussionID: "abc", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "snippet_id")
		})
	}
}

// TestNoteIDRequired_Validation ensures UpdateNote and DeleteNote reject zero/negative note_id.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when note_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "abc", NoteID: 0, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "abc", NoteID: -1})
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
		{"List", func() error { _, e := List(ctx, client, ListInput{SnippetID: 10}); return e }},
		{"Get", func() error { _, e := Get(ctx, client, GetInput{SnippetID: 10, DiscussionID: "abc"}); return e }},
		{"Create", func() error { _, e := Create(ctx, client, CreateInput{SnippetID: 10, Body: "x"}); return e }},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{SnippetID: 10, DiscussionID: "abc", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{SnippetID: 10, DiscussionID: "abc", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{SnippetID: 10, DiscussionID: "abc", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "project_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// covDiscussionJSON identifies the cov discussion JSON constant used by this package.
const covDiscussionJSON = `{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}]}`

// covNoteJSON identifies the cov note JSON constant used by this package.
const covNoteJSON = `{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`

// ---------------------------------------------------------------------------
// API error paths (use 400 to avoid go-retryablehttp retries)
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", SnippetID: 1, DiscussionID: "d1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", SnippetID: 1, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestAddNote_APIError verifies AddNote when API error.
func TestAddNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := AddNote(context.Background(), client, AddNoteInput{ProjectID: "1", SnippetID: 1, DiscussionID: "d1", Body: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateNote_APIError verifies UpdateNote when API error.
func TestUpdateNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := UpdateNote(context.Background(), client, UpdateNoteInput{ProjectID: "1", SnippetID: 1, DiscussionID: "d1", NoteID: 1, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// ---------------------------------------------------------------------------
// Converter edge cases
// ---------------------------------------------------------------------------.

// TestNoteToOutput_NilUpdatedAt verifies NoteToOutput when nil updated at.
func TestNoteToOutput_NilUpdatedAt(t *testing.T) {
	n := &gl.Note{
		ID:        42,
		Body:      "test",
		System:    true,
		Author:    gl.NoteAuthor{Username: "bob"},
		CreatedAt: new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		UpdatedAt: nil,
	}
	out := toolutil.DiscussionNoteOutputFromGitLab(n)
	if out.UpdatedAt != "" {
		t.Errorf("expected empty UpdatedAt, got %q", out.UpdatedAt)
	}
	if !out.System {
		t.Error("expected System=true")
	}
}

// TestNoteToOutput_EmptyAuthor verifies NoteToOutput when empty author.
func TestNoteToOutput_EmptyAuthor(t *testing.T) {
	n := &gl.Note{
		ID:        1,
		Body:      "test",
		CreatedAt: new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	out := toolutil.DiscussionNoteOutputFromGitLab(n)
	if out.Author != "" {
		t.Errorf("expected empty Author, got %q", out.Author)
	}
}

// TestNoteToOutput_ZeroCreatedAt verifies NoteToOutput when zero created at.
func TestNoteToOutput_ZeroCreatedAt(t *testing.T) {
	zero := time.Time{}
	n := &gl.Note{
		ID:        1,
		Body:      "test",
		CreatedAt: &zero,
	}
	out := toolutil.DiscussionNoteOutputFromGitLab(n)
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt for zero time, got %q", out.CreatedAt)
	}
}

// TestToOutput_NoNotes verifies ToOutput when no notes.
func TestToOutput_NoNotes(t *testing.T) {
	d := &gl.Discussion{
		ID:             "d1",
		IndividualNote: true,
		Notes:          nil,
	}
	out := toolutil.DiscussionOutputFromGitLab(d)
	if out.ID != "d1" {
		t.Errorf("expected d1, got %q", out.ID)
	}
	if len(out.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(out.Notes))
	}
}

// TestToListOutput_Empty verifies ToListOutput when empty.
func TestToListOutput_Empty(t *testing.T) {
	out := toListOutput(nil, nil)
	if len(out.Discussions) != 0 {
		t.Errorf("expected 0 discussions, got %d", len(out.Discussions))
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{
				ID: "d1",
				Notes: []NoteOutput{
					{ID: 1, Author: "alice", CreatedAt: "2026-01-01T00:00:00Z", Body: "note body"},
				},
			},
		},
	}
	s := FormatListMarkdownString(out)
	if !strings.Contains(s, "Snippet Discussions") {
		t.Error("expected header")
	}
	if !strings.Contains(s, "alice") {
		t.Error("expected author")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(s, "No snippet discussions found") {
		t.Error("expected empty message")
	}
}

// TestFormatMarkdown_WithNotes verifies FormatMarkdown when with notes.
func TestFormatMarkdown_WithNotes(t *testing.T) {
	out := Output{
		ID: "d1",
		Notes: []NoteOutput{
			{ID: 1, Author: "bob", CreatedAt: "2026-01-01T00:00:00Z", Body: "hello"},
		},
	}
	s := FormatMarkdownString(out)
	if !strings.Contains(s, "Discussion d1") {
		t.Error("expected discussion ID")
	}
	if !strings.Contains(s, "@bob") {
		t.Error("expected author")
	}
}

// TestFormatNoteMarkdown_AllFields verifies FormatNoteMarkdown when all fields.
func TestFormatNoteMarkdown_AllFields(t *testing.T) {
	out := NoteOutput{
		ID:        1,
		Author:    "carol",
		Body:      "test body",
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	s := FormatNoteMarkdownString(out)
	if !strings.Contains(s, "Note") {
		t.Error("expected Note header")
	}
	if !strings.Contains(s, "@carol") {
		t.Error("expected author")
	}
	if !strings.Contains(s, "Created") {
		t.Error("expected Created")
	}
}

// TestFormatNoteMarkdown_NoCreatedAt verifies FormatNoteMarkdown when no created at.
func TestFormatNoteMarkdown_NoCreatedAt(t *testing.T) {
	s := FormatNoteMarkdownString(NoteOutput{ID: 1, Author: "x", Body: "y"})
	if strings.Contains(s, "Created") {
		t.Error("should not include Created when empty")
	}
}

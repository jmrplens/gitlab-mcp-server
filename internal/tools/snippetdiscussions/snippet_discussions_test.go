// snippet_discussions_test.go contains unit tests for the snippet discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package snippetdiscussions

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestDiscussionIDRequired_Validation ensures discussion-scoped handlers reject an empty discussion_id.
func TestDiscussionIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, SnippetID: 10, DiscussionID: ""})
			return e
		}},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "discussion_id")
		})
	}
}

// TestContextCancelled_Validation ensures every handler returns early when the context is already cancelled.
func TestContextCancelled_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, SnippetID: 10}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, SnippetID: 10, DiscussionID: "d1"})
			return e
		}},
		{"Create", func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, SnippetID: 10, Body: "x"})
			return e
		}},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "d1", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "d1", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "d1", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("expected context error, got nil")
			}
		})
	}
}

// TestList_KeysetAndOrdering verifies that ordering and keyset pagination inputs are forwarded as query parameters.
func TestList_KeysetAndOrdering(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "created_at" {
			t.Errorf("order_by=%q, want created_at", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort=%q, want desc", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination=%q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "tok99" {
			t.Errorf("page_token=%q, want tok99", q.Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[`+covDiscussionJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{
		ProjectID:  "1",
		SnippetID:  5,
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok99",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Discussions) != 1 {
		t.Fatalf("got %d discussions, want 1", len(out.Discussions))
	}
}

// TestCreate_CreatedAt verifies the created_at backdate field is forwarded to the API.
func TestCreate_CreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "2025-01-01T00:00:00Z") {
			t.Errorf("created_at not forwarded; body=%s", body)
		}
		testutil.RespondJSON(w, http.StatusCreated, covDiscussionJSON)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", SnippetID: 5, Body: "x", CreatedAt: "2025-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestAddNote_CreatedAt verifies the created_at backdate field is forwarded on add-note.
func TestAddNote_CreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "2025-01-01T00:00:00Z") {
			t.Errorf("created_at not forwarded; body=%s", body)
		}
		testutil.RespondJSON(w, http.StatusCreated, covNoteJSON)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", Body: "x", CreatedAt: "2025-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdateNote_CreatedAt verifies the created_at override field is forwarded on update-note.
func TestUpdateNote_CreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "2025-01-01T00:00:00Z") {
			t.Errorf("created_at not forwarded; body=%s", body)
		}
		testutil.RespondJSON(w, http.StatusOK, covNoteJSON)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 1, Body: "x", CreatedAt: "2025-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestNoteIDRequired_Validation ensures UpdateNote and DeleteNote reject
// zero/negative note_id. Both handlers are driven at both sides of the
// guard: zero is what an omitted note_id decodes to, so a guard that only
// refused negatives would send a caller's omission to GitLab as note 0.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"UpdateNote/zero", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "abc", NoteID: 0, Body: "x"})
			return e
		}},
		{"UpdateNote/negative", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "abc", NoteID: -1, Body: "x"})
			return e
		}},
		{"DeleteNote/zero", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: pid, SnippetID: 10, DiscussionID: "abc", NoteID: 0})
		}},
		{"DeleteNote/negative", func() error {
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
	out := toolutil.DiscussionThreadNoteOutputFromGitLab(n, toolutil.NoteExtra{})
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
	out := toolutil.DiscussionThreadNoteOutputFromGitLab(n, toolutil.NoteExtra{})
	if out.Author == nil || out.Author.Username != "" {
		t.Errorf("expected an empty author object, got %+v", out.Author)
	}
}

// TestNoteToOutput_NoCreatedAt verifies that a note GitLab sends without a
// created_at, which the SDK decodes as a nil time, renders it empty.
func TestNoteToOutput_NoCreatedAt(t *testing.T) {
	n := &gl.Note{
		ID:   1,
		Body: "test",
	}
	out := toolutil.DiscussionThreadNoteOutputFromGitLab(n, toolutil.NoteExtra{})
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt for a nil time, got %q", out.CreatedAt)
	}
}

// TestToOutput_NoNotes verifies ToOutput when no notes.
func TestToOutput_NoNotes(t *testing.T) {
	d := &gl.Discussion{
		ID:             "d1",
		IndividualNote: true,
		Notes:          nil,
	}
	out := toolutil.DiscussionThreadOutputFromGitLab(d, toolutil.DiscussionExtra{})
	if out.ID != "d1" {
		t.Errorf("expected d1, got %q", out.ID)
	}
	if len(out.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(out.Notes))
	}
}

// TestList_Empty verifies an empty list answer converts to an output with no
// discussions rather than a nil-dereference, now that the list handler builds
// its output from the shared thread converter.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "42", SnippetID: 10})

	if err != nil || len(out.Discussions) != 0 {
		t.Errorf("List() = %+v, %v; want no discussions and no error", out, err)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage
// ---------------------------------------------------------------------------.

// The three guidance sections this package's shared renderers end with, so each
// expectation below can pin the whole rendered document.
//
// These three formatters are registered for [toolutil.DiscussionThreadOutput]
// and [toolutil.DiscussionThreadNoteOutput], which every REST discussion domain
// aliases, and the registry keeps the first registration of a type, so what a
// snippet discussion actually renders as is the commitdiscussions card. They
// are tested here for what they write, which is what this package would serve
// once each domain has a shape of its own.
const (
	listHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `snippet.discussion_get` to view full discussion details\n"
	threadHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `snippet.discussion_add_note` to reply to this discussion\n"
	noteHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `snippet.discussion_update_note` to edit this note\n"
)

// TestFormatListMarkdown_WithData pins the whole list document of a page of
// snippet discussion threads.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{
				ID: "d1",
				Notes: []*NoteOutput{
					{ID: 1, Author: &toolutil.NoteUserOutput{Username: "alice"}, CreatedAt: "2026-01-01T00:00:00Z", Body: "note body"},
				},
			},
		},
	}

	got := FormatListMarkdownString(out)

	want := "## Snippet Discussions (1)\n\n" +
		"### Discussion d1\n" +
		"- **@alice** (1 Jan 2026 00:00 UTC, note 1):\n" +
		"  > note body\n" +
		listHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a snippet with no
// discussions.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{})

	if want := "No snippet discussions found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkdown_WithNotes pins the whole card of a thread with one note.
func TestFormatMarkdown_WithNotes(t *testing.T) {
	out := Output{
		ID: "d1",
		Notes: []*NoteOutput{
			{ID: 1, Author: &toolutil.NoteUserOutput{Username: "bob"}, CreatedAt: "2026-01-01T00:00:00Z", Body: "hello"},
		},
	}

	got := FormatMarkdownString(out)

	want := "## Discussion d1\n\n" +
		"- **@bob** (1 Jan 2026 00:00 UTC, note 1):\n" +
		"  > hello\n" +
		threadHintsBlock
	if got != want {
		t.Errorf("thread card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdown_AllFields pins the whole note card.
func TestFormatNoteMarkdown_AllFields(t *testing.T) {
	out := NoteOutput{
		ID:         1,
		Author:     &toolutil.NoteUserOutput{Username: "carol"},
		Body:       "test body",
		CreatedAt:  "2026-01-01T00:00:00Z",
		Resolvable: true,
	}

	got := FormatNoteMarkdownString(out)

	want := "## Discussion Note #1\n\n" +
		"- **Author**: @carol\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Resolvable**: unresolved\n" +
		"- **Body**: test body\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdown_NoCreatedAt pins the card of a note with no time on
// it: the row is absent rather than empty, and a note that cannot be resolved
// shows no resolution state.
func TestFormatNoteMarkdown_NoCreatedAt(t *testing.T) {
	got := FormatNoteMarkdownString(NoteOutput{ID: 1, Author: &toolutil.NoteUserOutput{Username: "x"}, Body: "y"})

	want := "## Discussion Note #1\n\n" +
		"- **Author**: @x\n" +
		"- **Body**: y\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
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
			_, err := List(t.Context(), client, ListInput{ProjectID: "42", SnippetID: 10})
			return err
		}},
		{Name: "get", Call: func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectID: "42", SnippetID: 10, DiscussionID: "d1"})
			return err
		}},
		{Name: "create", Call: func() error {
			_, err := Create(t.Context(), client, CreateInput{ProjectID: "42", SnippetID: 10, Body: "x"})
			return err
		}},
		{Name: "add note", Call: func() error {
			_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: "42", SnippetID: 10, DiscussionID: "d1", Body: "x"})
			return err
		}},
		{Name: "update note", Call: func() error {
			_, err := UpdateNote(t.Context(), client, UpdateNoteInput{ProjectID: "42", SnippetID: 10, DiscussionID: "d1", NoteID: 1, Body: "x"})
			return err
		}},
	})
}

// TestList_Pagination_ComesFromTheResponseHeaders pins the whole pagination
// block against a page whose six figures all differ, so a block left at its
// zero value or filled from the wrong response cannot pass. It is a plain
// assignment, which neither gate scores, and it is the only thing that tells a
// caller there is a second page of threads to ask for.
func TestList_Pagination_ComesFromTheResponseHeaders(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covDiscussionJSON+`]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "7", Total: "31", TotalPages: "5", NextPage: "3", PrevPage: "1"})
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "1", SnippetID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := toolutil.PaginationOutput{Page: 2, PerPage: 7, TotalItems: 31, TotalPages: 5, NextPage: 3, PrevPage: 1, HasMore: true}
	if out.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestMutatingHandlers_TheRequestTheyBuild pins the method, the path and the
// body of every request the four mutating handlers send. All three are
// straight-line assignments that neither gate scores, and each has a failure a
// caller would meet: a path assembled from the wrong identifiers edits somebody
// else's thread, and a body that never leaves the process posts an empty note.
// Every identifier here differs from every other, so a swapped argument cannot
// read as a match.
func TestMutatingHandlers_TheRequestTheyBuild(t *testing.T) {
	ctx := t.Context()
	const (
		project    = "11"
		snippet    = 5
		discussion = "d9"
		note       = 77
	)

	tests := []struct {
		name       string
		wantMethod string
		wantPath   string
		wantBody   string
		answer     string // empty answers 204 with no body, as a delete does
		call       func(client *gitlabclient.Client) error
	}{
		{
			name: "Create", wantMethod: http.MethodPost,
			wantPath: "/api/v4/projects/11/snippets/5/discussions",
			wantBody: `"body":"first note"`, answer: covDiscussionJSON,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(ctx, client, CreateInput{ProjectID: project, SnippetID: snippet, Body: "first note"})
				return err
			},
		},
		{
			name: "AddNote", wantMethod: http.MethodPost,
			wantPath: "/api/v4/projects/11/snippets/5/discussions/d9/notes",
			wantBody: `"body":"a reply"`, answer: covNoteJSON,
			call: func(client *gitlabclient.Client) error {
				_, err := AddNote(ctx, client, AddNoteInput{ProjectID: project, SnippetID: snippet, DiscussionID: discussion, Body: "a reply"})
				return err
			},
		},
		{
			name: "UpdateNote", wantMethod: http.MethodPut,
			wantPath: "/api/v4/projects/11/snippets/5/discussions/d9/notes/77",
			wantBody: `"body":"edited"`, answer: covNoteJSON,
			call: func(client *gitlabclient.Client) error {
				_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: project, SnippetID: snippet, DiscussionID: discussion, NoteID: note, Body: "edited"})
				return err
			},
		},
		{
			name: "DeleteNote", wantMethod: http.MethodDelete,
			wantPath: "/api/v4/projects/11/snippets/5/discussions/d9/notes/77",
			call: func(client *gitlabclient.Client) error {
				return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: project, SnippetID: snippet, DiscussionID: discussion, NoteID: note})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen atomic.Int32
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen.Add(1)
				if r.Method != tt.wantMethod {
					t.Errorf("method = %s, want %s", r.Method, tt.wantMethod)
				}
				if r.URL.Path != tt.wantPath {
					t.Errorf("path = %s, want %s", r.URL.Path, tt.wantPath)
				}
				if body, _ := io.ReadAll(r.Body); tt.wantBody != "" && !strings.Contains(string(body), tt.wantBody) {
					t.Errorf("body = %s, want it to carry %s", body, tt.wantBody)
				}
				if tt.answer == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, tt.answer)
			}))

			if err := tt.call(client); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if got := seen.Load(); got != 1 {
				t.Fatalf("GitLab saw %d requests, want 1", got)
			}
		})
	}
}

// TestHandlers_TheHintIsAttachedToTheStatusItWasWrittenFor drives each handler
// at the status its hint is declared for and at one it is not. Both the status
// constant and the hint text are straight-line arguments, so nothing else here
// would notice a hint moved to the wrong status: a model told to "verify
// discussion_id" after a 403 goes looking for an id that was never wrong.
func TestHandlers_TheHintIsAttachedToTheStatusItWasWrittenFor(t *testing.T) {
	ctx := t.Context()
	const unrelatedStatus = http.StatusConflict

	tests := []struct {
		name   string
		status int
		hint   string
		call   func(client *gitlabclient.Client) error
	}{
		{
			name: "List", status: http.StatusNotFound,
			hint: "verify project_id with project.get and snippet_id with snippet.project_list",
			call: func(client *gitlabclient.Client) error {
				_, err := List(ctx, client, ListInput{ProjectID: "1", SnippetID: 5})
				return err
			},
		},
		{
			name: "Get", status: http.StatusNotFound,
			hint: "verify discussion_id with snippet.discussion_list (discussion IDs are 40-char hex strings)",
			call: func(client *gitlabclient.Client) error {
				_, err := Get(ctx, client, GetInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1"})
				return err
			},
		},
		{
			name: "Create", status: http.StatusBadRequest,
			hint: "body is required and cannot be empty; commenting requires Reporter role or being the snippet author",
			call: func(client *gitlabclient.Client) error {
				_, err := Create(ctx, client, CreateInput{ProjectID: "1", SnippetID: 5, Body: "x"})
				return err
			},
		},
		{
			name: "AddNote", status: http.StatusNotFound,
			hint: "verify discussion_id with snippet.discussion_list; the discussion must exist on this snippet",
			call: func(client *gitlabclient.Client) error {
				_, err := AddNote(ctx, client, AddNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", Body: "x"})
				return err
			},
		},
		{
			name: "UpdateNote", status: http.StatusForbidden,
			hint: "updating a note requires being the note author; system notes cannot be modified",
			call: func(client *gitlabclient.Client) error {
				_, err := UpdateNote(ctx, client, UpdateNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 1, Body: "x"})
				return err
			},
		},
		{
			name: "DeleteNote", status: http.StatusForbidden,
			hint: "deleting a note requires being the note author or Maintainer role; system notes cannot be deleted",
			call: func(client *gitlabclient.Client) error {
				return DeleteNote(ctx, client, DeleteNoteInput{ProjectID: "1", SnippetID: 5, DiscussionID: "d1", NoteID: 1})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.call(testutil.NewTestClient(t, refusingHandler(tt.status))), tt.hint)

			err := tt.call(testutil.NewTestClient(t, refusingHandler(unrelatedStatus)))
			if err == nil {
				t.Fatalf("expected an error from a %d", unrelatedStatus)
			}
			if strings.Contains(err.Error(), tt.hint) {
				t.Errorf("a %d carried the hint written for %d: %v", unrelatedStatus, tt.status, err)
			}
		})
	}
}

// refusingHandler answers every request with status, so a handler's error
// classification is decided by the status alone.
func refusingHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, `{"message":"refused"}`)
	})
}

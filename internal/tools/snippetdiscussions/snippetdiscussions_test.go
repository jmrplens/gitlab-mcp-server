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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/snippets/5/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"snippet note","author":{"username":"alice"},"created_at":"2024-01-01T00:00:00Z"}]}]`,
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

// TestList_APIError verifies the behavior of list a p i error.
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

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/snippets/5/discussions/d1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"d1","individual_note":true,"notes":[{"id":10,"body":"test","author":{"username":"bob"},"created_at":"2024-01-01T00:00:00Z"}]}`)
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

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":"d2","individual_note":false,"notes":[{"id":20,"body":"new","author":{"username":"carol"},"created_at":"2024-01-02T00:00:00Z"}]}`)
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

// TestAddNote_Success verifies the behavior of add note success.
func TestAddNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":30,"body":"reply","author":{"username":"dave"},"created_at":"2024-01-03T00:00:00Z"}`)
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

// TestUpdateNote_Success verifies the behavior of update note success.
func TestUpdateNote_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":30,"body":"updated","author":{"username":"dave"},"created_at":"2024-01-03T00:00:00Z","updated_at":"2024-01-04T00:00:00Z"}`)
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

// TestDeleteNote_Success verifies the behavior of delete note success.
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

// TestDeleteNote_APIError verifies the behavior of delete note a p i error.
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

const errExpectedErr = "expected error"

const errExpNonNilResult = "expected non-nil result"

const covDiscussionJSON = `{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01T00:00:00Z"}]}`
const covNoteJSON = `{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

// ---------------------------------------------------------------------------
// API error paths (use 400 to avoid go-retryablehttp retries)
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", SnippetID: 1, DiscussionID: "d1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", SnippetID: 1, Body: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestAddNote_APIError verifies the behavior of add note a p i error.
func TestAddNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := AddNote(context.Background(), client, AddNoteInput{ProjectID: "1", SnippetID: 1, DiscussionID: "d1", Body: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateNote_APIError verifies the behavior of update note a p i error.
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

// TestNoteToOutput_NilUpdatedAt verifies the behavior of note to output nil updated at.
func TestNoteToOutput_NilUpdatedAt(t *testing.T) {
	n := &gl.Note{
		ID:        42,
		Body:      "test",
		System:    true,
		Author:    gl.NoteAuthor{Username: "bob"},
		CreatedAt: new(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		UpdatedAt: nil,
	}
	out := noteToOutput(n)
	if out.UpdatedAt != "" {
		t.Errorf("expected empty UpdatedAt, got %q", out.UpdatedAt)
	}
	if !out.System {
		t.Error("expected System=true")
	}
}

// TestNoteToOutput_EmptyAuthor verifies the behavior of note to output empty author.
func TestNoteToOutput_EmptyAuthor(t *testing.T) {
	n := &gl.Note{
		ID:        1,
		Body:      "test",
		CreatedAt: new(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	out := noteToOutput(n)
	if out.Author != "" {
		t.Errorf("expected empty Author, got %q", out.Author)
	}
}

// TestNoteToOutput_ZeroCreatedAt verifies the behavior of note to output zero created at.
func TestNoteToOutput_ZeroCreatedAt(t *testing.T) {
	zero := time.Time{}
	n := &gl.Note{
		ID:        1,
		Body:      "test",
		CreatedAt: &zero,
	}
	out := noteToOutput(n)
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt for zero time, got %q", out.CreatedAt)
	}
}

// TestToOutput_NoNotes verifies the behavior of to output no notes.
func TestToOutput_NoNotes(t *testing.T) {
	d := &gl.Discussion{
		ID:             "d1",
		IndividualNote: true,
		Notes:          nil,
	}
	out := toOutput(d)
	if out.ID != "d1" {
		t.Errorf("expected d1, got %q", out.ID)
	}
	if len(out.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(out.Notes))
	}
}

// TestToListOutput_Empty verifies the behavior of to list output empty.
func TestToListOutput_Empty(t *testing.T) {
	out := toListOutput(nil, nil)
	if len(out.Discussions) != 0 {
		t.Errorf("expected 0 discussions, got %d", len(out.Discussions))
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{
				ID: "d1",
				Notes: []NoteOutput{
					{ID: 1, Author: "alice", CreatedAt: "2024-01-01T00:00:00Z", Body: "note body"},
				},
			},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	s := FormatListMarkdownString(out)
	if !strings.Contains(s, "Snippet Discussions") {
		t.Error("expected header")
	}
	if !strings.Contains(s, "alice") {
		t.Error("expected author")
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(s, "No snippet discussions found") {
		t.Error("expected empty message")
	}
}

// TestFormatMarkdown_WithNotes verifies the behavior of format markdown with notes.
func TestFormatMarkdown_WithNotes(t *testing.T) {
	out := Output{
		ID: "d1",
		Notes: []NoteOutput{
			{ID: 1, Author: "bob", CreatedAt: "2024-01-01T00:00:00Z", Body: "hello"},
		},
	}
	result := FormatMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	s := FormatMarkdownString(out)
	if !strings.Contains(s, "Discussion d1") {
		t.Error("expected discussion ID")
	}
	if !strings.Contains(s, "@bob") {
		t.Error("expected author")
	}
}

// TestFormatNoteMarkdown_AllFields verifies the behavior of format note markdown all fields.
func TestFormatNoteMarkdown_AllFields(t *testing.T) {
	out := NoteOutput{
		ID:        1,
		Author:    "carol",
		Body:      "test body",
		CreatedAt: "2024-01-01T00:00:00Z",
	}
	result := FormatNoteMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
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

// TestFormatNoteMarkdown_NoCreatedAt verifies the behavior of format note markdown no created at.
func TestFormatNoteMarkdown_NoCreatedAt(t *testing.T) {
	s := FormatNoteMarkdownString(NoteOutput{ID: 1, Author: "x", Body: "y"})
	if strings.Contains(s, "Created") {
		t.Error("should not include Created when empty")
	}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all 6 individual tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost:
			if strings.Contains(r.URL.Path, "/notes") {
				testutil.RespondJSON(w, http.StatusCreated, covNoteJSON)
			} else {
				testutil.RespondJSON(w, http.StatusCreated, covDiscussionJSON)
			}
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covNoteJSON)
		case strings.Contains(r.URL.Path, "/discussions/d1"):
			testutil.RespondJSON(w, http.StatusOK, covDiscussionJSON)
		default:
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[`+covDiscussionJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		}
	}))
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_snippet_discussions", map[string]any{"project_id": "1", "snippet_id": float64(5)}},
		{"gitlab_get_snippet_discussion", map[string]any{"project_id": "1", "snippet_id": float64(5), "discussion_id": "d1"}},
		{"gitlab_create_snippet_discussion", map[string]any{"project_id": "1", "snippet_id": float64(5), "body": "test"}},
		{"gitlab_add_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": float64(5), "discussion_id": "d1", "body": "reply"}},
		{"gitlab_update_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": float64(5), "discussion_id": "d1", "note_id": float64(1), "body": "updated"}},
		{"gitlab_delete_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": float64(5), "discussion_id": "d1", "note_id": float64(1)}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if result.IsError {
				t.Errorf("expected no error for %s", tc.name)
			}
		})
	}
}

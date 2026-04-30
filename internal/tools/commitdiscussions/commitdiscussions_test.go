// commitdiscussions_test.go contains unit tests for the commit discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package commitdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const errExpAPIFailure = "expected error for API failure, got nil"

const errExpCancelledNil = "expected error for canceled context, got nil"

const fmtUnexpErr = "unexpected error: %v"

const testDiscussionID = "d1"

const testCommitSHA = "abc123"

const testProjectID = "1"

const testAuthorAlice = "alice"

const testVersion = "0.0.1"

const (
	testPathDiscussions     = "/discussions"
	testPathDiscussionSlash = "/discussions/"
	testDate20260101        = "2026-01-01"
)

// TestList_Success verifies the behavior of list success.
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
	if out.Discussions[0].Notes[0].Author != testAuthorAlice {
		t.Errorf("got author=%q, want alice", out.Discussions[0].Notes[0].Author)
	}
}

// TestList_APIError verifies the behavior of list a p i error.
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

// TestGet_Success verifies the behavior of get success.
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

// TestCreate_Success verifies the behavior of create success.
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

// TestCreate_WithPosition verifies the behavior of create with position.
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

// TestAddNote_Success verifies the behavior of add note success.
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
	if out.Author != "eve" {
		t.Errorf("got author=%q, want eve", out.Author)
	}
}

// TestUpdateNote_Success verifies the behavior of update note success.
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

// TestDeleteNote_Success verifies the behavior of delete note success.
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

// TestDeleteNote_APIError verifies the behavior of delete note a p i error.
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

// TestUpdateNote_NoteIDValidation verifies the behavior of update note note i d validation.
func TestUpdateNote_NoteIDValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach API")
	}))
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

// TestDeleteNote_NoteIDValidation verifies the behavior of delete note note i d validation.
func TestDeleteNote_NoteIDValidation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach API")
	}))
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

// TestList_CancelledContext verifies the behavior of list cancelled context.
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

// TestGet_CancelledContext verifies the behavior of get cancelled context.
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

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
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

// TestAddNote_CancelledContext verifies the behavior of add note cancelled context.
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

// TestUpdateNote_CancelledContext verifies the behavior of update note cancelled context.
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

// TestDeleteNote_CancelledContext verifies the behavior of delete note cancelled context.
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

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, Body: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestAddNote_APIError verifies the behavior of add note a p i error.
func TestAddNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := AddNote(t.Context(), client, AddNoteInput{ProjectID: testProjectID, CommitSHA: testCommitSHA, DiscussionID: testDiscussionID, Body: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestUpdateNote_APIError verifies the behavior of update note a p i error.
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

// TestFormatListMarkdownString_WithData verifies the behavior of format list markdown string with data.
func TestFormatListMarkdownString_WithData(t *testing.T) {
	out := ListOutput{
		Discussions: []Output{
			{ID: testDiscussionID, Notes: []NoteOutput{{Author: testAuthorAlice, CreatedAt: testDate20260101, Body: "comment"}}},
			{ID: "d2", Notes: []NoteOutput{{Author: "bob", CreatedAt: "2026-01-02", Body: "reply"}}},
		},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "Commit Discussions (2)") {
		t.Errorf("expected header, got:\n%s", md)
	}
	if !strings.Contains(md, testDiscussionID) || !strings.Contains(md, "d2") {
		t.Error("expected discussion IDs")
	}
	if !strings.Contains(md, testAuthorAlice) {
		t.Error("expected author")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListOutput{Discussions: nil}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "No commit discussions found") {
		t.Error("expected empty message")
	}
}

// TestFormatMarkdownString_WithNotes verifies the behavior of format markdown string with notes.
func TestFormatMarkdownString_WithNotes(t *testing.T) {
	out := Output{
		ID:    testDiscussionID,
		Notes: []NoteOutput{{Author: "dev", CreatedAt: testDate20260101, Body: "LGTM"}},
	}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "Discussion "+testDiscussionID) {
		t.Error("expected header")
	}
	if !strings.Contains(md, "LGTM") {
		t.Error("expected note body")
	}
}

// TestFormatMarkdownString_Empty verifies the behavior of format markdown string empty.
func TestFormatMarkdownString_Empty(t *testing.T) {
	out := Output{ID: testDiscussionID, Notes: nil}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "Discussion "+testDiscussionID) {
		t.Error("expected header even with no notes")
	}
}

// TestFormatNoteMarkdownString verifies the behavior of format note markdown string.
func TestFormatNoteMarkdownString(t *testing.T) {
	n := NoteOutput{ID: 10, Author: "dev", Body: "Nice!", CreatedAt: testDate20260101}
	md := FormatNoteMarkdownString(n)
	if !strings.Contains(md, "Note") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "10") {
		t.Error("expected note ID")
	}
	if !strings.Contains(md, "Nice!") {
		t.Error("expected body")
	}
	if !strings.Contains(md, "1 Jan 2026") {
		t.Error("expected created date")
	}
}

// TestFormatNoteMarkdownString_NoDate verifies the behavior of format note markdown string no date.
func TestFormatNoteMarkdownString_NoDate(t *testing.T) {
	n := NoteOutput{ID: 11, Author: "bot", Body: "OK"}
	md := FormatNoteMarkdownString(n)
	if strings.Contains(md, "Created") {
		t.Error("should not show Created when empty")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------.

// callToolExpectSuccess is an internal helper for the commitdiscussions package.
func callToolExpectSuccess(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
			}
		}
	}
}

// newCommitDiscussionMockHandler is an internal helper for the commitdiscussions package.
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

// setupMCPSession is an internal helper for the commitdiscussions package.
func setupMCPSession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: testVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// ---------------------------------------------------------------------------
// RegisterTools Tests
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)
}

// callToolAndCheck is an internal helper for the commitdiscussions package.
func callToolAndCheck(t *testing.T, session *mcp.ClientSession, tools []struct {
	name string
	args map[string]any
}) {
	t.Helper()
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			callToolExpectSuccess(t, session, tt.name, tt.args)
		})
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	client := testutil.NewTestClient(t, newCommitDiscussionMockHandler(t))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)
	session := setupMCPSession(t, server)

	callToolAndCheck(t, session, []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_commit_discussions", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA}},
		{"gitlab_get_commit_discussion", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID}},
		{"gitlab_create_commit_discussion", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "body": "test"}},
		{"gitlab_add_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "body": "note"}},
		{"gitlab_update_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "note_id": float64(1), "body": "upd"}},
		{"gitlab_delete_commit_discussion_note", map[string]any{"project_id": testProjectID, "commit_sha": testCommitSHA, "discussion_id": testDiscussionID, "note_id": float64(1)}},
	})
}

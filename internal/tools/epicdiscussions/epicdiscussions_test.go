// epicdiscussions_test.go contains unit tests for the epic discussion MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package epicdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/1/epics/5/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":"d1","individual_note":false,"notes":[{"id":1,"body":"epic note","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}]}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{GroupID: "1", EpicID: 5})
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

	_, err := List(t.Context(), client, ListInput{GroupID: "1", EpicID: 5})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/1/epics/5/discussions/d1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":"d1","individual_note":true,"notes":[{"id":10,"body":"test","author":{"username":"bob"},"created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{GroupID: "1", EpicID: 5, DiscussionID: "d1"})
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
			`{"id":"d2","individual_note":false,"notes":[{"id":20,"body":"new epic thread","author":{"username":"carol"},"created_at":"2026-01-02T00:00:00Z"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{GroupID: "1", EpicID: 5, Body: "new epic thread"})
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
			`{"id":30,"body":"reply","author":{"username":"dave"},"created_at":"2026-01-03T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := AddNote(t.Context(), client, AddNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", Body: "reply"})
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
			`{"id":30,"body":"updated","author":{"username":"dave"},"created_at":"2026-01-03T00:00:00Z","updated_at":"2026-01-04T00:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := UpdateNote(t.Context(), client, UpdateNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", NoteID: 30, Body: "updated"})
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

	err := DeleteNote(t.Context(), client, DeleteNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", NoteID: 30})
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

	err := DeleteNote(t.Context(), client, DeleteNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", NoteID: 30})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// assertContains is an internal helper for the epicdiscussions package.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestEpicIDRequired_Validation ensures all handlers reject zero/negative epic_id.
func TestEpicIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when epic_id is invalid")
	}))
	ctx := context.Background()
	const gid = "my/group"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{GroupID: gid, EpicID: 0}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{GroupID: gid, EpicID: 0, DiscussionID: "abc"})
			return e
		}},
		{"Create", func() error { _, e := Create(ctx, client, CreateInput{GroupID: gid, EpicID: 0, Body: "x"}); return e }},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{GroupID: gid, EpicID: 0, DiscussionID: "abc", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{GroupID: gid, EpicID: 0, DiscussionID: "abc", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{GroupID: gid, EpicID: 0, DiscussionID: "abc", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "epic_id")
		})
	}
}

// TestNoteIDRequired_Validation ensures UpdateNote and DeleteNote reject zero/negative note_id.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when note_id is invalid")
	}))
	ctx := context.Background()
	const gid = "my/group"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{GroupID: gid, EpicID: 10, DiscussionID: "abc", NoteID: 0, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{GroupID: gid, EpicID: 10, DiscussionID: "abc", NoteID: -1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "note_id")
		})
	}
}

// TestGroupIDRequired_Validation ensures all handlers reject empty group_id.
func TestGroupIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when group_id is empty")
	}))
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{EpicID: 10}); return e }},
		{"Get", func() error { _, e := Get(ctx, client, GetInput{EpicID: 10, DiscussionID: "abc"}); return e }},
		{"Create", func() error { _, e := Create(ctx, client, CreateInput{EpicID: 10, Body: "x"}); return e }},
		{"AddNote", func() error {
			_, e := AddNote(ctx, client, AddNoteInput{EpicID: 10, DiscussionID: "abc", Body: "x"})
			return e
		}},
		{"UpdateNote", func() error {
			_, e := UpdateNote(ctx, client, UpdateNoteInput{EpicID: 10, DiscussionID: "abc", NoteID: 1, Body: "x"})
			return e
		}},
		{"DeleteNote", func() error {
			return DeleteNote(ctx, client, DeleteNoteInput{EpicID: 10, DiscussionID: "abc", NoteID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "group_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpNonNilResult = "expected non-nil result"

const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// List — API error (400)
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies the behavior of list a p i error400.
func TestList_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(t.Context(), client, ListInput{GroupID: "1", EpicID: 5})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Get — API error (400)
// ---------------------------------------------------------------------------.

// TestGet_APIError400 verifies the behavior of get a p i error400.
func TestGet_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(t.Context(), client, GetInput{GroupID: "1", EpicID: 5, DiscussionID: "d1"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Create — API error (400)
// ---------------------------------------------------------------------------.

// TestCreate_APIError400 verifies the behavior of create a p i error400.
func TestCreate_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Create(t.Context(), client, CreateInput{GroupID: "1", EpicID: 5, Body: "test"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// AddNote — API error (400)
// ---------------------------------------------------------------------------.

// TestAddNote_APIError400 verifies the behavior of add note a p i error400.
func TestAddNote_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := AddNote(t.Context(), client, AddNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", Body: "test"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// UpdateNote — API error (400)
// ---------------------------------------------------------------------------.

// TestUpdateNote_APIError400 verifies the behavior of update note a p i error400.
func TestUpdateNote_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateNote(t.Context(), client, UpdateNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", NoteID: 1, Body: "test"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// DeleteNote — API error (400)
// ---------------------------------------------------------------------------.

// TestDeleteNote_APIError400 verifies the behavior of delete note a p i error400.
func TestDeleteNote_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteNote(t.Context(), client, DeleteNoteInput{GroupID: "1", EpicID: 5, DiscussionID: "d1", NoteID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Formatters — empty + non-empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No epic discussions") {
		t.Errorf("expected 'No epic discussions' message, got %q", text)
	}
}

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Discussions: []Output{
			{
				ID:             "d1",
				IndividualNote: false,
				Notes: []NoteOutput{
					{ID: 1, Body: "Hello", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
				},
			},
		},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "d1") {
		t.Errorf("expected discussion ID in output, got %q", text)
	}
}

// TestFormatMarkdown_Empty verifies the behavior of format markdown empty.
func TestFormatMarkdown_Empty(t *testing.T) {
	result := FormatMarkdown(Output{ID: "d1"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "d1") {
		t.Errorf("expected discussion ID in output, got %q", text)
	}
}

// TestFormatMarkdown_WithNotes verifies the behavior of format markdown with notes.
func TestFormatMarkdown_WithNotes(t *testing.T) {
	result := FormatMarkdown(Output{
		ID: "d1",
		Notes: []NoteOutput{
			{ID: 1, Body: "note body", Author: "bob", CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "bob") {
		t.Errorf("expected author name in output, got %q", text)
	}
}

// TestFormatNoteMarkdown verifies the behavior of format note markdown.
func TestFormatNoteMarkdown(t *testing.T) {
	result := FormatNoteMarkdown(NoteOutput{ID: 1, Body: "test note", Author: "carol", CreatedAt: "2026-01-01T00:00:00Z"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "carol") {
		t.Errorf("expected author 'carol' in output, got %q", text)
	}
}

// TestFormatNoteMarkdown_NoCreatedAt verifies the behavior of format note markdown no created at.
func TestFormatNoteMarkdown_NoCreatedAt(t *testing.T) {
	result := FormatNoteMarkdown(NoteOutput{ID: 1, Body: "test", Author: "dave"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "Created") {
		t.Error("should not contain Created when no created_at")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools + RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newEpicDiscussionsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_epic_discussions", map[string]any{"group_id": "1", "epic_id": float64(5)}},
		{"get", "gitlab_get_epic_discussion", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1"}},
		{"create", "gitlab_create_epic_discussion", map[string]any{"group_id": "1", "epic_id": float64(5), "body": "new thread"}},
		{"add_note", "gitlab_add_epic_discussion_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "body": "reply"}},
		{"update_note", "gitlab_update_epic_discussion_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "note_id": float64(30), "body": "updated"}},
		{"delete_note", "gitlab_delete_epic_discussion_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "note_id": float64(30)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — meta tool
// ---------------------------------------------------------------------------.

// TestMCPRound_TripMetaTool validates m c p round trip meta tool across multiple scenarios using table-driven subtests.
func TestMCPRound_TripMetaTool(t *testing.T) {
	session := newEpicDiscussionsMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list", "list", map[string]any{"group_id": "1", "epic_id": float64(5)}},
		{"get", "get", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1"}},
		{"create", "create", map[string]any{"group_id": "1", "epic_id": float64(5), "body": "new thread"}},
		{"add_note", "add_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "body": "reply"}},
		{"update_note", "update_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "note_id": float64(30), "body": "updated"}},
		{"delete_note", "delete_note", map[string]any{"group_id": "1", "epic_id": float64(5), "discussion_id": "d1", "note_id": float64(30)}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_epic_discussion",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(gitlab_epic_discussion/%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(gitlab_epic_discussion/%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(gitlab_epic_discussion/%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers: MCP session factories
// ---------------------------------------------------------------------------.

// epicDiscussionsHandler is an internal helper for the epicdiscussions package.
func epicDiscussionsHandler() *http.ServeMux {
	handler := http.NewServeMux()

	noteJSON := `{"id":30,"body":"test","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"}`
	discussionJSON := `{"id":"d1","individual_note":false,"notes":[` + noteJSON + `]}`

	handler.HandleFunc("GET /api/v4/groups/1/epics/5/discussions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+discussionJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/1/epics/5/discussions/d1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, discussionJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/1/epics/5/discussions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, discussionJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/1/epics/5/discussions/d1/notes", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, noteJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/1/epics/5/discussions/d1/notes/30", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":30,"body":"updated","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`)
	})
	handler.HandleFunc("DELETE /api/v4/groups/1/epics/5/discussions/d1/notes/30", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return handler
}

// newEpicDiscussionsMCPSession is an internal helper for the epicdiscussions package.
func newEpicDiscussionsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, epicDiscussionsHandler())
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

// newEpicDiscussionsMetaMCPSession is an internal helper for the epicdiscussions package.
func newEpicDiscussionsMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, epicDiscussionsHandler())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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

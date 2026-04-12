package snippetnotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathSnippetNotes   = "/api/v4/projects/myproject/snippets/1/notes"
	pathSnippetNote100 = "/api/v4/projects/myproject/snippets/1/notes/100"

	noteJSON = `{
		"id": 100,
		"body": "Good snippet!",
		"author": {"username": "alice"},
		"system": false,
		"noteable_type": "Snippet",
		"noteable_id": 1,
		"created_at": "2024-03-10T09:00:00Z",
		"updated_at": "2024-03-10T09:00:00Z"
	}`

	noteSystemJSON = `{
		"id": 101,
		"body": "changed the title",
		"author": {"username": "admin"},
		"system": true,
		"noteable_type": "Snippet",
		"noteable_id": 1,
		"created_at": "2024-03-10T12:00:00Z",
		"updated_at": "2024-03-10T12:00:00Z"
	}`

	testProjectID = "myproject"
)

// List tests.

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSnippetNotes {
			testutil.RespondJSON(w, http.StatusOK, "["+noteJSON+","+noteSystemJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Notes) != 2 {
		t.Fatalf("len(Notes) = %d, want 2", len(out.Notes))
	}
	if out.Notes[0].ID != 100 {
		t.Errorf("Notes[0].ID = %d, want 100", out.Notes[0].ID)
	}
	if out.Notes[0].Body != "Good snippet!" {
		t.Errorf("Notes[0].Body = %q, want %q", out.Notes[0].Body, "Good snippet!")
	}
	if out.Notes[0].Author != "alice" {
		t.Errorf("Notes[0].Author = %q, want %q", out.Notes[0].Author, "alice")
	}
	if out.Notes[1].System != true {
		t.Error("Notes[1].System = false, want true")
	}
}

func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected error for missing project_id, got nil")
	}
}

func TestList_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("List() expected error for missing snippet_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected error for 500, got nil")
	}
}

func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page = %q, want %q", r.URL.Query().Get("page"), "2")
		}
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("per_page = %q, want %q", r.URL.Query().Get("per_page"), "5")
		}
		testutil.RespondJSON(w, http.StatusOK, "["+noteJSON+"]")
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		SnippetID:       1,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

func TestList_OrderBySort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("order_by") != "updated_at" {
			t.Errorf("order_by = %q, want %q", r.URL.Query().Get("order_by"), "updated_at")
		}
		if r.URL.Query().Get("sort") != "desc" {
			t.Errorf("sort = %q, want %q", r.URL.Query().Get("sort"), "desc")
		}
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		SnippetID: 1,
		OrderBy:   "updated_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// Get tests.

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSnippetNote100 {
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Author != "alice" {
		t.Errorf("out.Author = %q, want %q", out.Author, "alice")
	}
}

func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id, got nil")
	}
}

func TestGet_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected error for missing snippet_id, got nil")
	}
}

func TestGet_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing note_id, got nil")
	}
}

func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 999})
	if err == nil {
		t.Fatal("Get() expected error for 404, got nil")
	}
}

func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected context error, got nil")
	}
}

// Create tests.

func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathSnippetNotes {
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		SnippetID: 1,
		Body:      "Good snippet!",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
}

func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for missing project_id, got nil")
	}
}

func TestCreate_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for missing snippet_id, got nil")
	}
}

func TestCreate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Create() expected error for missing body, got nil")
	}
}

func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for 403, got nil")
	}
}

func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected context error, got nil")
	}
}

// Update tests.

func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathSnippetNote100 {
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: testProjectID,
		SnippetID: 1,
		NoteID:    100,
		Body:      "Updated snippet note",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
}

func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id, got nil")
	}
}

func TestUpdate_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing snippet_id, got nil")
	}
}

func TestUpdate_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing note_id, got nil")
	}
}

func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for 403, got nil")
	}
}

func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected context error, got nil")
	}
}

// Delete tests.

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathSnippetNote100 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for missing project_id, got nil")
	}
}

func TestDelete_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for missing snippet_id, got nil")
	}
}

func TestDelete_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing note_id, got nil")
	}
}

func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for 403, got nil")
	}
}

func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected context error, got nil")
	}
}

// Markdown tests.

func TestFormatOutputMarkdown_Basic(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     100,
		Body:   "Great snippet",
		Author: "alice",
		System: false,
	})
	if !contains(md, "## Snippet Note #100") {
		t.Error("missing header")
	}
	if !contains(md, "alice") {
		t.Error("missing author")
	}
}

func TestFormatOutputMarkdown_SystemNote(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     101,
		Body:   "changed the title",
		Author: "admin",
		System: true,
	})
	if !contains(md, "System note") {
		t.Error("missing system note indicator")
	}
}

func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !contains(md, "No snippet notes found") {
		t.Error("missing empty message")
	}
}

func TestFormatListMarkdown_WithNotes(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Notes: []Output{
			{ID: 100, Author: "alice", System: false},
			{ID: 101, Author: "admin", System: true},
		},
	})
	if !contains(md, "| 100 |") {
		t.Error("missing note 100 row")
	}
	if !contains(md, "| 101 |") {
		t.Error("missing note 101 row")
	}
}

// RegisterTools tests.

func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newSnippetNotesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_snippet_note_list", map[string]any{"project_id": testProjectID, "snippet_id": 1}},
		{"gitlab_snippet_note_get", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100}},
		{"gitlab_snippet_note_create", map[string]any{"project_id": testProjectID, "snippet_id": 1, "body": "test"}},
		{"gitlab_snippet_note_update", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100, "body": "updated"}},
		{"gitlab_snippet_note_delete", map[string]any{"project_id": testProjectID, "snippet_id": 1, "note_id": 100}},
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

func newSnippetNotesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathSnippetNotes:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathSnippetNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
		case r.Method == http.MethodPost && path == pathSnippetNotes:
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)
		case r.Method == http.MethodPut && path == pathSnippetNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSON)
		case r.Method == http.MethodDelete && path == pathSnippetNote100:
			w.WriteHeader(http.StatusNoContent)
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

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// toOutput coverage tests.

func TestToOutput_NilTimestamps(t *testing.T) {
	// Note with no created_at or updated_at (nil time pointers in the SDK).
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSnippetNote100 {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 100,
				"body": "note without timestamps",
				"author": {"username": ""},
				"system": false,
				"noteable_type": "Snippet",
				"noteable_id": 1
			}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", out.UpdatedAt)
	}
	if out.Author != "" {
		t.Errorf("Author = %q, want empty", out.Author)
	}
}

func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Notes) != 0 {
		t.Errorf("len(Notes) = %d, want 0", len(out.Notes))
	}
}

func TestFormatOutputMarkdown_WithUpdatedAt(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:        100,
		Body:      "test note",
		Author:    "bob",
		CreatedAt: "2024-03-10T09:00:00Z",
		UpdatedAt: "2024-03-10T10:00:00Z",
	})
	if !contains(md, "bob") {
		t.Error("missing author")
	}
	if !contains(md, "## Snippet Note #100") {
		t.Error("missing header")
	}
}

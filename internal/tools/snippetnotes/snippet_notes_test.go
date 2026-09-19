// snippet_notes_test.go contains unit tests for GitLab snippet note operations.
// Tests use httptest to mock the GitLab Snippet Notes API.
package snippetnotes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		"created_at": "2026-03-10T09:00:00Z",
		"updated_at": "2026-03-10T09:00:00Z"
	}`

	noteSystemJSON = `{
		"id": 101,
		"body": "changed the title",
		"author": {"username": "admin"},
		"system": true,
		"noteable_type": "Snippet",
		"noteable_id": 1,
		"created_at": "2026-03-10T12:00:00Z",
		"updated_at": "2026-03-10T12:00:00Z"
	}`

	testProjectID = "myproject"
)

// List tests.

// TestList_Success verifies that List lists a snippet note on a successful GitLab API response.
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
	if out.Notes[0].Author == nil || out.Notes[0].Author.Username != "alice" {
		t.Errorf("Notes[0].Author = %+v, want username alice", out.Notes[0].Author)
	}
	if out.Notes[1].System != true {
		t.Error("Notes[1].System = false, want true")
	}
}

// TestList_MissingProjectID verifies that List returns a validation error when project_id is missing.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected error for missing project_id, got nil")
	}
}

// TestList_MissingSnippetID verifies that List returns a validation error when snippet_id is missing.
func TestList_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("List() expected error for missing snippet_id, got nil")
	}
}

// TestList_CancelledContext verifies that List returns an error when the context is cancelled before the request completes.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestList_APIError verifies that List propagates errors returned by the GitLab API.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("List() expected error for 500, got nil")
	}
}

// TestList_Pagination verifies that List forwards pagination parameters (page, per_page) to the GitLab API.
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
		ProjectID: testProjectID,
		SnippetID: 1,
		Page:      2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// TestList_OrderBySort verifies that List forwards order_by and sort query parameters to the GitLab API.
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

// TestGet_Success verifies that Get retrieves a snippet note on a successful GitLab API response.
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
	if out.Author == nil || out.Author.Username != "alice" {
		t.Errorf("out.Author = %+v, want username alice", out.Author)
	}
	if out.NoteableType != "Snippet" {
		t.Errorf("out.NoteableType = %q, want Snippet", out.NoteableType)
	}
	if out.NoteableID != 1 {
		t.Errorf("out.NoteableID = %d, want 1", out.NoteableID)
	}
}

// TestGet_MissingProjectID verifies that Get returns a validation error when project_id is missing.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id, got nil")
	}
}

// TestGet_MissingSnippetID verifies that Get returns a validation error when snippet_id is missing.
func TestGet_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected error for missing snippet_id, got nil")
	}
}

// TestGet_MissingNoteID verifies that Get returns a validation error when note_id is missing.
func TestGet_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing note_id, got nil")
	}
}

// TestGet_APIError verifies that Get propagates errors returned by the GitLab API.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 999})
	if err == nil {
		t.Fatal("Get() expected error for 404, got nil")
	}
}

// TestGet_CancelledContext verifies that Get returns an error when the context is cancelled before the request completes.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Get() expected context error, got nil")
	}
}

// Create tests.

// TestCreate_Success verifies that Create creates a snippet note on a successful GitLab API response.
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

// TestCreate_MissingProjectID verifies that Create returns a validation error when project_id is missing.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for missing project_id, got nil")
	}
}

// TestCreate_MissingSnippetID verifies that Create returns a validation error when snippet_id is missing.
func TestCreate_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for missing snippet_id, got nil")
	}
}

// TestCreate_MissingBody verifies that Create returns a validation error when body is missing.
func TestCreate_MissingBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Create() expected error for missing body, got nil")
	}
}

// TestCreate_APIError verifies that Create propagates errors returned by the GitLab API.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected error for 403, got nil")
	}
}

// TestCreate_CancelledContext verifies that Create returns an error when the context is cancelled before the request completes.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: "hello"})
	if err == nil {
		t.Fatal("Create() expected context error, got nil")
	}
}

// Update tests.

// TestUpdate_Success verifies that Update updates a snippet note on a successful GitLab API response.
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

// TestUpdate_MissingProjectID verifies that Update returns a validation error when project_id is missing.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id, got nil")
	}
}

// TestUpdate_MissingSnippetID verifies that Update returns a validation error when snippet_id is missing.
func TestUpdate_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing snippet_id, got nil")
	}
}

// TestUpdate_MissingNoteID verifies that Update returns a validation error when note_id is missing.
func TestUpdate_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for missing note_id, got nil")
	}
}

// TestUpdate_APIError verifies that Update propagates errors returned by the GitLab API.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for 403, got nil")
	}
}

// TestUpdate_CancelledContext verifies that Update returns an error when the context is cancelled before the request completes.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected context error, got nil")
	}
}

// Delete tests.

// TestDelete_Success verifies that Delete deletes a snippet note on a successful GitLab API response.
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

// TestDelete_MissingProjectID verifies that Delete returns a validation error when project_id is missing.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for missing project_id, got nil")
	}
}

// TestDelete_MissingSnippetID verifies that Delete returns a validation error when snippet_id is missing.
func TestDelete_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for missing snippet_id, got nil")
	}
}

// TestDelete_MissingNoteID verifies that Delete returns a validation error when note_id is missing.
func TestDelete_MissingNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, SnippetID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing note_id, got nil")
	}
}

// TestDelete_APIError verifies that Delete propagates errors returned by the GitLab API.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for 403, got nil")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error when the context is cancelled before the request completes.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected context error, got nil")
	}
}

// Required-identifier tests.

// assertNames verifies that err is non-nil and names the parameter the caller
// left out, rather than being any error at all.
func assertNames(t *testing.T, err error, param string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %q, got nil", param)
	}
	if !strings.Contains(err.Error(), param) {
		t.Errorf("error %q does not name %q", err.Error(), param)
	}
}

// missingIDValues are the two values an absent identifier takes: the zero a
// caller really reaches by omitting the field, and a negative one.
var missingIDValues = []struct {
	name string
	id   int64
}{{"Zero", 0}, {"Negative", -1}}

// TestSnippetIDRequired_Validation holds every handler to refusing a missing or
// negative snippet_id itself, before a request leaves for GitLab.
//
// Zero is the value a caller really reaches, since an omitted snippet_id
// arrives here as the zero value, and it is the one the old assertions could
// not see: they only asked that some error came back, and a guard written
// "< 0" instead of "<= 0" still produced one: GitLab's 404 for snippet 0,
// which tells the caller nothing about the parameter it forgot. The mock
// refuses every request, so reaching the network is itself a failure.
func TestSnippetIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	handlers := []struct {
		name string
		call func(snippetID int64) error
	}{
		{"List", func(id int64) error {
			_, e := List(ctx, client, ListInput{ProjectID: testProjectID, SnippetID: id})
			return e
		}},
		{"Get", func(id int64) error {
			_, e := Get(ctx, client, GetInput{ProjectID: testProjectID, SnippetID: id, NoteID: 100})
			return e
		}},
		{"Create", func(id int64) error {
			_, e := Create(ctx, client, CreateInput{ProjectID: testProjectID, SnippetID: id, Body: "x"})
			return e
		}},
		{"Update", func(id int64) error {
			_, e := Update(ctx, client, UpdateInput{ProjectID: testProjectID, SnippetID: id, NoteID: 100, Body: "x"})
			return e
		}},
		{"Delete", func(id int64) error {
			return Delete(ctx, client, DeleteInput{ProjectID: testProjectID, SnippetID: id, NoteID: 100})
		}},
	}

	for _, h := range handlers {
		for _, id := range missingIDValues {
			t.Run(h.name+"_"+id.name, func(t *testing.T) {
				assertNames(t, h.call(id.id), "snippet_id")
			})
		}
	}
}

// TestNoteIDRequired_Validation holds the three handlers that address one note
// to refusing a missing or negative note_id before any request is made, for
// the reason TestSnippetIDRequired_Validation states: note IDs start at 1, so
// the zero value is what an omitted note_id looks like here.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	handlers := []struct {
		name string
		call func(noteID int64) error
	}{
		{"Get", func(id int64) error {
			_, e := Get(ctx, client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: id})
			return e
		}},
		{"Update", func(id int64) error {
			_, e := Update(ctx, client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: id, Body: "x"})
			return e
		}},
		{"Delete", func(id int64) error {
			return Delete(ctx, client, DeleteInput{ProjectID: testProjectID, SnippetID: 1, NoteID: id})
		}},
	}

	for _, h := range handlers {
		for _, id := range missingIDValues {
			t.Run(h.name+"_"+id.name, func(t *testing.T) {
				assertNames(t, h.call(id.id), "note_id")
			})
		}
	}
}

// Markdown tests.

// The two guidance sections a snippet note result ends with, so each
// expectation below can pin the whole rendered document.
const (
	noteHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'note_update' with note_id to edit this note\n" +
		"- Use action 'note_delete' with note_id to remove this note\n"
	listHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'note_get' with note_id to read a specific note\n" +
		"- Use action 'note_create' to add a new note to this snippet\n"
)

// TestFormatOutputMarkdown_Basic pins the whole card of a note somebody wrote.
func TestFormatOutputMarkdown_Basic(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:     100,
		Body:   "Great snippet",
		Author: &toolutil.NoteUserOutput{Username: "alice"},
		System: false,
	})

	want := "## Snippet Note #100\n\n" +
		"- **Author**: @alice\n" +
		"- **Body**: Great snippet\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_NilAuthor pins the card of a note GitLab answered
// with no author object: the author row is absent rather than a bare handle.
func TestFormatOutputMarkdown_NilAuthor(t *testing.T) {
	got := FormatOutputMarkdown(Output{ID: 100, Body: "no author"})

	want := "## Snippet Note #100\n\n" +
		"- **Body**: no author\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_SystemNote pins the card of a system note.
func TestFormatOutputMarkdown_SystemNote(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:     101,
		Body:   "changed the title",
		Author: &toolutil.NoteUserOutput{Username: "admin"},
		System: true,
	})

	want := "## Snippet Note #101\n\n" +
		"- **Author**: @admin\n" +
		"- **System note**\n" +
		"- **Body**: changed the title\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a snippet with no
// notes.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No snippet notes found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_WithNotes pins the whole list document: the table the
// shared note list renderer writes, with the system flag as a glyph.
func TestFormatListMarkdown_WithNotes(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Notes: []Output{
			{ID: 100, Author: &toolutil.NoteUserOutput{Username: "alice"}, System: false},
			{ID: 101, Author: &toolutil.NoteUserOutput{Username: "admin"}, System: true},
		},
	})

	want := "## Snippet Notes (2)\n\n" +
		"| ID | Author | Created | System |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 100 | alice |  | ❌ |\n" +
		"| 101 | admin |  | ✅ |\n" +
		listHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// toOutput coverage tests.

// TestToOutput_NilTimestamps verifies that toOutput handles edge cases in the
// GitLab response (nil timestamps or optional fields) without panicking.
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
	if out.Author == nil || out.Author.Username != "" {
		t.Errorf("Author = %+v, want non-nil empty username", out.Author)
	}
}

// TestToOutput_FullFields verifies that Get maps every additive gl.Note field,
// including the author object and the nested position / line_range sub-objects.
func TestToOutput_FullFields(t *testing.T) {
	const richNoteJSON = `{
		"id": 100,
		"type": "DiffNote",
		"body": "rich note",
		"attachment": "file.txt",
		"title": "t",
		"file_name": "snippet.rb",
		"author": {"id": 7, "username": "alice", "email": "a@x.io", "name": "Alice", "state": "active", "avatar_url": "http://a/x.png", "web_url": "http://a/alice"},
		"system": false,
		"internal": true,
		"resolvable": true,
		"resolved": true,
		"resolved_at": "2026-03-11T08:00:00Z",
		"resolved_by": {"id": 9, "username": "bob", "name": "Bob", "state": "active"},
		"commit_id": "abc123",
		"expires_at": "2027-01-01T00:00:00Z",
		"created_at": "2026-03-10T09:00:00Z",
		"updated_at": "2026-03-10T09:30:00Z",
		"noteable_type": "Snippet",
		"noteable_id": 1,
		"noteable_iid": 2,
		"project_id": 42,
		"confidential": true,
		"position": {
			"base_sha": "b", "start_sha": "s", "head_sha": "h",
			"position_type": "text", "new_path": "np", "new_line": 5,
			"old_path": "op", "old_line": 4,
			"line_range": {
				"start": {"line_code": "lc1", "type": "new", "old_line": 1, "new_line": 2},
				"end": {"line_code": "lc2", "type": "old", "old_line": 3, "new_line": 4}
			}
		}
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSnippetNote100 {
			testutil.RespondJSON(w, http.StatusOK, richNoteJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	assertFullNoteScalars(t, out)
	assertFullNotePosition(t, out.Position)
}

// assertFullNoteScalars checks the additive scalar fields and author object.
func assertFullNoteScalars(t *testing.T, out Output) {
	t.Helper()
	if out.Author == nil || out.Author.ID != 7 || out.Author.WebURL != "http://a/alice" {
		t.Errorf("Author = %+v, want fully populated", out.Author)
	}
	if !out.Internal || !out.Resolvable || !out.Confidential {
		t.Errorf("flags not mapped: internal=%v resolvable=%v confidential=%v", out.Internal, out.Resolvable, out.Confidential)
	}
	if out.CommitID != "abc123" {
		t.Errorf("commit_id not mapped: %q", out.CommitID)
	}
	if out.NoteableIID != 2 || out.ProjectID != 42 {
		t.Errorf("noteable_iid/project_id not mapped: %d %d", out.NoteableIID, out.ProjectID)
	}
	assertFullNoteResolution(t, out)
}

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler here: GitLab's answer
// decodes for the SDK and not for the fields this package reads beside it,
// which is a fault in the type naming them and is reported rather than
// swallowed. A string where `imported` is a bool is the shape.
func TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		note := `{"id":1,"body":"x","author":{"id":1,"username":"u"},"imported":"not-a-bool"}`
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/myproject/snippets/1/notes" {
			note = "[" + note + "]"
		}
		testutil.RespondJSON(w, http.StatusOK, note)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "create", Call: func() error {
			_, err := Create(t.Context(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: "x"})
			return err
		}},
		{Name: "list", Call: func() error {
			_, err := List(t.Context(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
			return err
		}},
		{Name: "get", Call: func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 1})
			return err
		}},
		{Name: "update", Call: func() error {
			_, err := Update(t.Context(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 1, Body: "x"})
			return err
		}},
	})
}

// assertFullNoteResolution checks the additive type / resolution sub-fields.
func assertFullNoteResolution(t *testing.T, out Output) {
	t.Helper()
	if out.Type != "DiffNote" {
		t.Errorf("Type = %q, want DiffNote", out.Type)
	}
	if !out.Resolved || out.ResolvedAt != "2026-03-11T08:00:00Z" {
		t.Errorf("resolved/resolved_at not mapped: resolved=%v resolved_at=%q", out.Resolved, out.ResolvedAt)
	}
	if out.ResolvedBy == nil || out.ResolvedBy.ID != 9 || out.ResolvedBy.Username != "bob" {
		t.Errorf("ResolvedBy = %+v, want populated", out.ResolvedBy)
	}
}

// TestToOutput_UnresolvedNote verifies version-tolerant degradation: when the
// GitLab response omits the resolved_* fields (older instances) the resolved_by
// object is nil and the timestamp/flag fields are zero-valued (the
// noteResolvedByOutput nil branch).
func TestToOutput_UnresolvedNote(t *testing.T) {
	const minimalNoteJSON = `{"id":100,"body":"plain"}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSnippetNote100 {
			testutil.RespondJSON(w, http.StatusOK, minimalNoteJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.Resolved || out.ResolvedAt != "" || out.ResolvedBy != nil || out.Type != "" {
		t.Errorf("unresolved note should have zero resolved_* fields: resolved=%v resolved_at=%q resolved_by=%+v type=%q",
			out.Resolved, out.ResolvedAt, out.ResolvedBy, out.Type)
	}
}

// assertFullNotePosition checks the nested position / line_range sub-objects.
func assertFullNotePosition(t *testing.T, pos *toolutil.NotePositionOutput) {
	t.Helper()
	if pos == nil {
		t.Fatal("Position = nil, want populated")
	}
	if pos.BaseSHA != "b" || pos.NewLine != 5 || pos.OldLine != 4 {
		t.Errorf("Position fields not mapped: %+v", pos)
	}
	if pos.LineRange == nil || pos.LineRange.Start == nil || pos.LineRange.End == nil {
		t.Fatalf("LineRange not mapped: %+v", pos.LineRange)
	}
	if pos.LineRange.Start.LineCode != "lc1" || pos.LineRange.End.NewLine != 4 {
		t.Errorf("LineRange endpoints not mapped: %+v", pos.LineRange)
	}
}

// TestToOutput_PositionNilLineRange verifies position mapping when line_range is
// absent and when its endpoints are null (covers the nil branches of
// lineRangeOutput and linePositionOutput).
func TestToOutput_PositionNilLineRange(t *testing.T) {
	cases := map[string]string{
		"no_line_range":  `{"id":100,"position":{"base_sha":"b"}}`,
		"null_endpoints": `{"id":100,"position":{"base_sha":"b","line_range":{"start":null,"end":null}}}`,
	}
	for name, noteBody := range cases {
		t.Run(name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == pathSnippetNote100 {
					testutil.RespondJSON(w, http.StatusOK, noteBody)
					return
				}
				http.NotFound(w, r)
			}))
			out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100})
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			if out.Position == nil || out.Position.BaseSHA != "b" {
				t.Fatalf("Position not mapped: %+v", out.Position)
			}
			if out.Position.LineRange != nil {
				t.Errorf("LineRange = %+v, want nil", out.Position.LineRange)
			}
		})
	}
}

// TestList_KeysetPagination verifies that List forwards keyset pagination
// parameters (pagination=keyset, page_token) to the GitLab API.
func TestList_KeysetPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", r.URL.Query().Get("pagination"))
		}
		if r.URL.Query().Get("page_token") != "tok123" {
			t.Errorf("page_token = %q, want tok123", r.URL.Query().Get("page_token"))
		}
		testutil.RespondJSON(w, http.StatusOK, "["+noteJSON+"]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:  testProjectID,
		SnippetID:  1,
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// TestCreate_CreatedAt verifies that Create forwards the created_at backdating
// timestamp to the GitLab API.
func TestCreate_CreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathSnippetNotes {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				http.Error(w, "read body", http.StatusInternalServerError)
				return
			}
			if !strings.Contains(string(raw), "2026-01-15T10:00:00Z") {
				t.Errorf("request body missing created_at: %s", raw)
			}
			testutil.RespondJSON(w, http.StatusCreated, noteJSON)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		SnippetID: 1,
		Body:      "hi",
		CreatedAt: "2026-01-15T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
}

// TestList_EmptyResult verifies that List returns an empty slice (not nil) when the GitLab API returns no items.
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

// TestFormatOutputMarkdown_WithUpdatedAt pins the whole card of a note that has
// been edited: the shared note card shows the time it was written, and the time
// of the edit is carried by the JSON rather than the card.
func TestFormatOutputMarkdown_WithUpdatedAt(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:        100,
		Body:      "test note",
		Author:    &toolutil.NoteUserOutput{Username: "bob"},
		CreatedAt: "2026-03-10T09:00:00Z",
		UpdatedAt: "2026-03-10T10:00:00Z",
	})

	want := "## Snippet Note #100\n\n" +
		"- **Author**: @bob\n" +
		"- **Created**: 10 Mar 2026 09:00 UTC\n" +
		"- **Body**: test note\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestList_PaginationBlock_MirrorsTheResponseHeaders pins the page block a
// caller reads against the headers GitLab answered with.
//
// Nothing asserted it before: the tests that send page and per_page check the
// query string and drop the response, so List could have published an empty
// block (no page, no total, no next page) and stayed green, leaving a model
// with a page of notes and no way to learn there are more. Every header here
// carries a different number so a block filled from the wrong one is visible.
func TestList_PaginationBlock_MirrorsTheResponseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers testutil.PaginationHeaders
		want    toolutil.PaginationOutput
	}{
		{
			name:    "MiddlePage",
			headers: testutil.PaginationHeaders{Page: "3", PerPage: "7", Total: "52", TotalPages: "8", NextPage: "4", PrevPage: "2"},
			want: toolutil.PaginationOutput{
				Page: 3, PerPage: 7, TotalItems: 52, TotalPages: 8, NextPage: 4, PrevPage: 2, HasMore: true,
			},
		},
		{
			// GitLab sends no X-Next-Page on the last page, which is the only
			// thing that makes has_more false.
			name:    "LastPage",
			headers: testutil.PaginationHeaders{Page: "8", PerPage: "7", Total: "52", TotalPages: "8", PrevPage: "6"},
			want: toolutil.PaginationOutput{
				Page: 8, PerPage: 7, TotalItems: 52, TotalPages: 8, PrevPage: 6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != pathSnippetNotes {
					http.NotFound(w, r)
					return
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSON+"]", tt.headers)
			}))
			out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, SnippetID: 1})
			if err != nil {
				t.Fatalf("List() error: %v", err)
			}
			if out.Pagination != tt.want {
				t.Errorf("Pagination = %+v, want %+v", out.Pagination, tt.want)
			}
		})
	}
}

// TestCreateAndUpdate_EscapedNewlines_ReachGitLabAsRealNewlines pins what the
// body handed to GitLab is, not just that one was sent.
//
// A model writing a multi-line comment routinely puts the two characters
// backslash-n in the JSON string rather than an escape, and both handlers run
// the body through NormalizeText so GitLab stores a real line break. Dropping
// that call from either handler left the whole suite green, which is how a
// note could ship with "\n" printed in its text.
func TestCreateAndUpdate_EscapedNewlines_ReachGitLabAsRealNewlines(t *testing.T) {
	const typed = `first line\nsecond line`
	const wantSent = "first line\nsecond line"

	tests := []struct {
		name   string
		method string
		path   string
		call   func(client *gitlabclient.Client) error
	}{
		{"Create", http.MethodPost, pathSnippetNotes, func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, SnippetID: 1, Body: typed})
			return err
		}},
		{"Update", http.MethodPut, pathSnippetNote100, func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, SnippetID: 1, NoteID: 100, Body: typed})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					http.NotFound(w, r)
					return
				}
				var sent struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Errorf("decode request body: %v", err)
					http.Error(w, "bad body", http.StatusBadRequest)
					return
				}
				if sent.Body != wantSent {
					t.Errorf("body sent = %q, want %q", sent.Body, wantSent)
				}
				testutil.RespondJSON(w, http.StatusOK, noteJSON)
			}))
			if err := tt.call(client); err != nil {
				t.Fatalf("%s() error: %v", tt.name, err)
			}
		})
	}
}

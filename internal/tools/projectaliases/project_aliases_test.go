// project_aliases_test.go contains unit tests for GitLab project alias
// operations. Tests use httptest to mock the GitLab Project Aliases API.
package projectaliases

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// --- List ---

// TestList_Success verifies that List returns project aliases correctly
// when the GitLab API responds with a valid JSON array.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/project_aliases")
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"project_id":100,"name":"alias-one"},
			{"id":2,"project_id":200,"name":"alias-two"}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(out.Aliases))
	}
	if out.Aliases[0].Name != "alias-one" {
		t.Errorf("first alias name = %q, want %q", out.Aliases[0].Name, "alias-one")
	}
	if out.Aliases[0].ID != 1 {
		t.Errorf("first alias ID = %d, want 1", out.Aliases[0].ID)
	}
	if out.Aliases[1].ProjectID != 200 {
		t.Errorf("second alias project_id = %d, want 200", out.Aliases[1].ProjectID)
	}
}

// TestList_Empty verifies that List returns an empty slice when no aliases exist.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/project_aliases")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Aliases) != 0 {
		t.Fatalf("expected 0 aliases, got %d", len(out.Aliases))
	}
}

// TestList_APIError verifies that List returns an error when the API responds
// with a non-success status code.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error on API failure, got nil")
	}
}

// TestList_ContextCancelled verifies that List returns an error when the
// context is cancelled before making the API call.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- Get ---

// TestGet_Success verifies that Get retrieves a single alias by name.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/project_aliases/my-alias")
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"project_id":100,"name":"my-alias"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{Name: "my-alias"})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.Name != "my-alias" {
		t.Errorf("name = %q, want %q", out.Name, "my-alias")
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.ProjectID != 100 {
		t.Errorf("project_id = %d, want 100", out.ProjectID)
	}
}

// TestGet_MissingName verifies that Get returns a validation error when
// the name field is empty.
func TestGet_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with empty name")
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// TestGet_NotFound verifies that Get returns an error when the alias does not exist.
func TestGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, GetInput{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for not found alias, got nil")
	}
}

// TestGet_ContextCancelled verifies that Get returns an error when the
// context is cancelled before the request.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{Name: "test"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- Create ---

// TestCreate_Success verifies that Create sends the correct request and
// returns the newly created alias.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/project_aliases")
		testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"project_id":42,"name":"new-alias"}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{Name: "new-alias", ProjectID: 42})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.Name != "new-alias" {
		t.Errorf("name = %q, want %q", out.Name, "new-alias")
	}
	if out.ID != 3 {
		t.Errorf("ID = %d, want 3", out.ID)
	}
	if out.ProjectID != 42 {
		t.Errorf("project_id = %d, want 42", out.ProjectID)
	}
}

// TestCreate_MissingName verifies that Create rejects input with empty name.
func TestCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with missing name")
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: 42})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// TestCreate_MissingProjectID verifies that Create rejects input with zero project_id.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with missing project_id")
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{Name: "test"})
	if err == nil {
		t.Fatal("expected error for missing project_id, got nil")
	}
}

// TestCreate_APIError verifies that Create returns an error when the API
// responds with a conflict (duplicate alias).
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))

	_, err := Create(context.Background(), client, CreateInput{Name: "dup", ProjectID: 1})
	if err == nil {
		t.Fatal("expected error on API failure, got nil")
	}
}

// TestCreate_ContextCancelled verifies that Create returns an error when the
// context is cancelled before making the request.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{Name: "test", ProjectID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- Delete ---

// TestDelete_Success verifies that Delete sends the correct DELETE request
// and returns no error on 204 No Content.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		testutil.AssertRequestPath(t, r, "/api/v4/project_aliases/old-alias")
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{Name: "old-alias"})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

// TestDelete_MissingName verifies that Delete rejects input with empty name.
func TestDelete_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with missing name")
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// TestDelete_NotFound verifies that Delete returns an error when the alias
// does not exist.
func TestDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	err := Delete(context.Background(), client, DeleteInput{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for not found alias, got nil")
	}
}

// TestDelete_ContextCancelled verifies that Delete returns an error when the
// context is cancelled before making the request.
func TestDelete_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{Name: "test"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- Markdown Formatters ---

// TestFormatOutputMarkdown verifies that FormatOutputMarkdown produces a
// Markdown table with the alias details and hint lines.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		ID:        7,
		ProjectID: 42,
		Name:      "my-alias",
	}

	md := FormatOutputMarkdown(out)

	checks := []string{
		"## Project Alias: my-alias",
		"| ID | 7 |",
		"| Name | `my-alias` |",
		"| Project ID | 42 |",
		"gitlab_delete_project_alias",
		"gitlab_list_project_aliases",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("FormatOutputMarkdown missing %q in:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_WithAliases verifies that FormatListMarkdown produces
// a Markdown table with rows for each alias.
func TestFormatListMarkdown_WithAliases(t *testing.T) {
	out := ListOutput{
		Aliases: []Output{
			{ID: 1, ProjectID: 100, Name: "alpha"},
			{ID: 2, ProjectID: 200, Name: "beta"},
		},
	}

	md := FormatListMarkdown(out)

	checks := []string{
		"## Project Aliases (2)",
		"| 1 | `alpha` | 100 |",
		"| 2 | `beta` | 200 |",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "No project aliases found") {
		t.Error("non-empty list should not contain empty message")
	}
}

// TestFormatListMarkdown_Empty verifies that FormatListMarkdown shows a
// "no aliases" message when the list is empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Aliases: []Output{}})

	if !strings.Contains(md, "## Project Aliases (0)") {
		t.Errorf("expected heading with count 0 in:\n%s", md)
	}
	if !strings.Contains(md, "No project aliases found") {
		t.Errorf("expected empty message in:\n%s", md)
	}
}

// TestFormatListMarkdown_NilAliases verifies that FormatListMarkdown handles
// a nil Aliases slice without panicking.
func TestFormatListMarkdown_NilAliases(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})

	if !strings.Contains(md, "## Project Aliases (0)") {
		t.Errorf("expected heading with count 0 in:\n%s", md)
	}
	if !strings.Contains(md, "No project aliases found") {
		t.Errorf("expected empty message in:\n%s", md)
	}
}

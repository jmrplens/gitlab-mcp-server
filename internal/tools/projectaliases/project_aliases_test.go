// project_aliases_test.go contains unit tests for GitLab project alias
// operations. Tests use httptest to mock the GitLab Project Aliases API.
package projectaliases

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// TestCreate_RequestBody_CarriesTheNameAndProjectIDTheCallerGave holds the POST
// body to the caller's own values. Nothing asserted them: the alias in the
// reply is a fixture this test writes, so a handler that sent an empty name or
// a zero project_id would create the wrong alias — or none — while every
// assertion about what came back still passed.
func TestCreate_RequestBody_CarriesTheNameAndProjectIDTheCallerGave(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
			testutil.RespondJSON(w, http.StatusCreated, `{}`)
			return
		}
		if got := body["name"]; got != "alias-to-create" {
			t.Errorf("name sent = %v, want %q", got, "alias-to-create")
		}
		if got := body["project_id"]; got != float64(77) {
			t.Errorf("project_id sent = %v, want 77", got)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"project_id":77,"name":"alias-to-create"}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{Name: "alias-to-create", ProjectID: 77})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
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

// --- Error hints ---

// TestProjectAliases_Failure_HintsOnTheStatusEachHandlerKeysOn drives every
// handler into the status it attaches its hint to, and List into one it does
// not. Each pairing is a literal argument, which both coverage gates are blind
// to, and only the four tests asserting `err != nil` stood over them: keyed to
// the wrong status the hint silently disappears, leaving an administrator told
// nothing but that the call failed.
func TestProjectAliases_Failure_HintsOnTheStatusEachHandlerKeysOn(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		call     func(context.Context, *gitlabclient.Client) error
		wantOp   string
		wantHint string
	}{
		{
			name:   "list forbidden names the admin requirement",
			status: http.StatusForbidden,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := List(ctx, c, ListInput{})
				return err
			},
			wantOp:   "list project aliases",
			wantHint: "project aliases require administrator access",
		},
		{
			name:   "list not found carries no hint",
			status: http.StatusNotFound,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := List(ctx, c, ListInput{})
				return err
			},
			wantOp: "list project aliases",
		},
		{
			name:   "get not found points back at the list action",
			status: http.StatusNotFound,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Get(ctx, c, GetInput{Name: "missing-alias"})
				return err
			},
			wantOp:   "get project alias",
			wantHint: "verify the alias name with gitlab_list_project_aliases",
		},
		{
			name:   "create bad request names the project_id and the collision",
			status: http.StatusBadRequest,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := Create(ctx, c, CreateInput{Name: "dup-alias", ProjectID: 9})
				return err
			},
			wantOp:   "create project alias",
			wantHint: "verify the project_id exists and alias name is unique",
		},
		{
			name:   "delete not found points back at the list action",
			status: http.StatusNotFound,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				return Delete(ctx, c, DeleteInput{Name: "missing-alias"})
			},
			wantOp:   "delete project alias",
			wantHint: "verify the alias name with gitlab_list_project_aliases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			err := tt.call(context.Background(), client)
			if err == nil {
				t.Fatalf("expected an error for status %d, got nil", tt.status)
			}
			if !strings.Contains(err.Error(), tt.wantOp+":") {
				t.Errorf("error %q does not name the operation %q", err, tt.wantOp)
			}
			switch tt.wantHint {
			case "":
				if strings.Contains(err.Error(), "Suggestion:") {
					t.Errorf("error %q carries a hint on a status the handler does not key on", err)
				}
			default:
				if !strings.Contains(err.Error(), "Suggestion: "+tt.wantHint) {
					t.Errorf("error %q does not suggest %q", err, tt.wantHint)
				}
			}
		})
	}
}

// --- Markdown Formatters ---

// TestFormatOutputMarkdown verifies the whole alias card: the heading, one list
// row per field with the name as a code span, and the hints last.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		ID:        7,
		ProjectID: 42,
		Name:      "my-alias",
	}

	md := FormatOutputMarkdown(out)

	want := "## Project Alias: my-alias\n\n" +
		"- **ID**: 7\n" +
		"- **Name**: `my-alias`\n" +
		"- **Project ID**: 42\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project_alias.delete' to remove this alias\n" +
		"- Use action 'project_alias.list' to view all aliases\n"
	if md != want {
		t.Errorf("FormatOutputMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatOutputMarkdown_NameHoldingAPipe verifies that a name carrying the
// cell separator stays inside its code span: a span escapes the pipe with a
// backslash, which is the one escape GFM honors there, and writes no entity,
// since an entity renders literally inside a span.
func TestFormatOutputMarkdown_NameHoldingAPipe(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 7, ProjectID: 42, Name: "a|b"})

	want := "## Project Alias: a|b\n\n" +
		"- **ID**: 7\n" +
		"- **Name**: `a|b`\n" +
		"- **Project ID**: 42\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project_alias.delete' to remove this alias\n" +
		"- Use action 'project_alias.list' to view all aliases\n"
	if md != want {
		t.Errorf("FormatOutputMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatOutputMarkdown_NameHoldingATag verifies where the containment of a
// hostile name is: the heading escapes the angle bracket, and the row shows the
// value inside a code span, which a client renders as the text it is. The span
// is why the value carries no entity there — an entity renders literally inside
// one, which is what the row used to show.
func TestFormatOutputMarkdown_NameHoldingATag(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 7, ProjectID: 42, Name: `<a href="http://attacker.invalid">x</a>`})

	want := "## Project Alias: &lt;a href=\"http://attacker.invalid\">x&lt;/a>\n\n" +
		"- **ID**: 7\n" +
		"- **Name**: `<a href=\"http://attacker.invalid\">x</a>`\n" +
		"- **Project ID**: 42\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project_alias.delete' to remove this alias\n" +
		"- Use action 'project_alias.list' to view all aliases\n"
	if md != want {
		t.Errorf("FormatOutputMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatListMarkdown_WithAliases verifies the whole list: the heading
// counting what the response can vouch for, the table, and the hints after it
// rather than above the header, where they used to leave the table unrendered.
func TestFormatListMarkdown_WithAliases(t *testing.T) {
	out := ListOutput{
		Aliases: []Output{
			{ID: 1, ProjectID: 100, Name: "alpha"},
			{ID: 2, ProjectID: 200, Name: "beta"},
		},
	}

	md := FormatListMarkdown(out)

	want := "## Project Aliases (2)\n\n" +
		"| ID | Name | Project ID |\n| --- | --- | --- |\n" +
		"| 1 | `alpha` | 100 |\n" +
		"| 2 | `beta` | 200 |\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project_alias.get' to see one alias\n"
	if md != want {
		t.Errorf("FormatListMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatListMarkdown_Empty verifies that an empty list is the one sentence
// and nothing else: a heading counting zero above it said the same thing twice.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Aliases: []Output{}})

	if want := "No project aliases found.\n"; md != want {
		t.Errorf("FormatListMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatListMarkdown_NilAliases verifies that FormatListMarkdown handles
// a nil Aliases slice the same way as an empty one.
func TestFormatListMarkdown_NilAliases(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})

	if want := "No project aliases found.\n"; md != want {
		t.Errorf("FormatListMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// snippets_test.go contains unit tests for the snippet MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package snippets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// snippetJSON identifies the snippet JSON constant used by this package.
const snippetJSON = `{"id":42,"title":"Test Snippet","file_name":"test.go","description":"A test","visibility":"private","author":{"id":1,"username":"admin","name":"Admin","email":"admin@example.com","state":"active"},"project_id":0,"web_url":"https://gitlab.example.com/snippets/42","raw_url":"https://gitlab.example.com/snippets/42/raw","files":[{"path":"test.go","raw_url":"https://gitlab.example.com/snippets/42/raw/main/test.go"}]}`

// snippetListJSON identifies the snippet list JSON constant used by this package.
const snippetListJSON = `[` + snippetJSON + `]`

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(out.Snippets))
	}
	if out.Snippets[0].Title != "Test Snippet" {
		t.Errorf("expected title 'Test Snippet', got %s", out.Snippets[0].Title)
	}
}

// ---------------------------------------------------------------------------
// ListAll
// ---------------------------------------------------------------------------.

// TestListAll_Success verifies ListAll when success.
func TestListAll_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/all", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(out.Snippets))
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Get(context.Background(), client, GetInput{SnippetID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestGet_MissingSnippetID verifies Get when missing snippet ID.
func TestGet_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Content
// ---------------------------------------------------------------------------.

// TestContent_Success verifies Content when success.
func TestContent_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42/raw", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("package main\nfunc main() {}"))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Content(context.Background(), client, ContentInput{SnippetID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(out.Content, "package main") {
		t.Errorf("expected content to contain 'package main', got: %s", out.Content)
	}
}

// TestContent_MissingSnippetID verifies Content when missing snippet ID.
func TestContent_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Content(context.Background(), client, ContentInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// FileContent
// ---------------------------------------------------------------------------.

// TestFileContent_Success verifies FileContent answers with the bytes GitLab
// served and echoes each of the three identifiers the caller named back on its
// own key. The ref and the file name are echoed rather than read from a
// response, so a handler that put one under the other's key would publish a
// wrong answer with no branch for either gate to see; the fixture therefore
// gives no two of them the same value.
func TestFileContent_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42/files/a-ref/b-file.go/raw", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content here"))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := FileContent(context.Background(), client, FileContentInput{
		SnippetID: 42, Ref: "a-ref", FileName: "b-file.go",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := FileContentOutput{SnippetID: 42, Ref: "a-ref", FileName: "b-file.go", Content: "file content here"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("file content = %+v, want %+v", out, want)
	}
}

// TestFileContent_MissingParams verifies FileContent when missing params.
func TestFileContent_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := FileContent(context.Background(), client, FileContentInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}

	_, err = FileContent(context.Background(), client, FileContentInput{SnippetID: 42})
	if err == nil || !strings.Contains(err.Error(), "ref is required") {
		t.Fatalf("expected ref required error, got %v", err)
	}

	_, err = FileContent(context.Background(), client, FileContentInput{SnippetID: 42, Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), "file_name is required") {
		t.Fatalf("expected file_name required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------.

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		if !strings.Contains(string(body), "package main") {
			t.Errorf("request body = %q, want snippet content", string(body))
			http.Error(w, "request body, want snippet content", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Create(context.Background(), client, CreateInput{
		Title:       "Test Snippet",
		FileName:    "test.go",
		ContentBody: "package main",
		Visibility:  "private",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestCreate_MissingTitle verifies Create when missing title.
func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// TestCreate_MissingContent verifies Create when missing content.
func TestCreate_MissingContent(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{Title: "Test Snippet", FileName: "test.go"})
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("expected content required error, got %v", err)
	}
}

// TestValidateCreateSnippetContent_MultiFileErrors verifies per-file validation.
func TestValidateCreateSnippetContent_MultiFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		files   []CreateFileInput
		wantErr string
	}{
		{name: "missing path", files: []CreateFileInput{{Content: "package main"}}, wantErr: "files[0].file_path"},
		{name: "missing content", files: []CreateFileInput{{FilePath: "main.go"}}, wantErr: "files[0].content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateSnippetContent("", "", tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateCreateSnippetContent() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------.

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Update(context.Background(), client, UpdateInput{
		SnippetID: 42,
		Title:     "Updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestUpdate_MissingSnippetID verifies Update when missing snippet ID.
func TestUpdate_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := Delete(context.Background(), client, DeleteInput{SnippetID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_MissingSnippetID verifies Delete when missing snippet ID.
func TestDelete_MissingSnippetID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Explore
// ---------------------------------------------------------------------------.

// TestExplore_Success verifies Explore when success.
func TestExplore_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/public", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Explore(context.Background(), client, ExploreInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(out.Snippets))
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatMarkdown verifies the whole card of a personal snippet.
func TestFormatMarkdown(t *testing.T) {
	const webURL = "https://example.com/snippets/42"
	got := FormatMarkdown(Output{
		ID: 42, Title: "Test", Visibility: "private",
		Author: &SnippetAuthorOutput{Name: "Admin", Username: "admin"},
		WebURL: webURL,
	})
	want := "## Snippet #42: Test\n\n" +
		"- **ID**: 42\n" +
		"- **Title**: Test\n" +
		"- **Visibility**: private\n" +
		"- **Author**: Admin (@admin)\n" +
		"- **URL**: [" + webURL + "](" + webURL + ")\n" +
		snippetCardHints(false, true)
	if got != want {
		t.Errorf("personal snippet card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown verifies the whole render of a page of personal
// snippets: the heading, the table and the personal actions.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Snippets: []Output{{ID: 1, Title: "S1", Visibility: "public", Author: &SnippetAuthorOutput{Username: "u1"}}},
	})
	want := "## Snippets (1)\n\n" +
		"| ID | Title | Visibility | Author | Files |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | S1 | public | @u1 | 0 |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'get' with snippet_id for full details\n" +
		"- Use action 'create' to add a new snippet\n"
	if got != want {
		t.Errorf("personal snippet list:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContentMarkdown verifies the whole render of a snippet's content:
// the heading, the content in a fence and the two next steps.
func TestFormatContentMarkdown(t *testing.T) {
	got := FormatContentMarkdown(ContentOutput{SnippetID: 42, Content: "hello world"})
	want := "## Snippet #42 Content\n\n" +
		"```\nhello world\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'file_content' to get content of a specific file\n" +
		"- " + hintUpdateSnippet + "\n"
	if got != want {
		t.Errorf("snippet content:\n got %q\nwant %q", got, want)
	}
}

// TestFormatSnippetNotFound verifies not-found result formatting.
func TestFormatSnippetNotFound(t *testing.T) {
	result := formatSnippetNotFound(snippetNotFoundOutput{Identifier: "42"})
	if result == nil || !result.IsError {
		t.Fatalf("formatSnippetNotFound() = %+v, want error result", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(content.Text, "Snippet") || !strings.Contains(content.Text, "42") {
		t.Fatalf("content = %q, want snippet identifier", content.Text)
	}
}

// TestAddSnippetCreateFileRequirement_EdgeCases verifies schema mutation guards.
func TestAddSnippetCreateFileRequirement_EdgeCases(t *testing.T) {
	addSnippetCreateFileRequirement(nil)

	schema := &jsonschema.Schema{}
	addSnippetCreateFileRequirement(schema)
	if schema.Properties == nil {
		t.Fatal("expected properties map to be initialized")
	}
	if len(schema.AnyOf) != 2 {
		t.Fatalf("AnyOf length = %d, want 2", len(schema.AnyOf))
	}

	filesSchema := &jsonschema.Schema{}
	schema = &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"files": filesSchema}}
	addSnippetCreateFileRequirement(schema)
	if filesSchema.MinItems == nil || *filesSchema.MinItems != 1 {
		t.Fatalf("files MinItems = %v, want 1", filesSchema.MinItems)
	}
}

// TestCreateInputSchemaMaps verifies snippet create schema maps expose the file requirement.
func TestCreateInputSchemaMaps(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"personal": CreateInputSchemaMap(),
		"project":  ProjectCreateInputSchemaMap(),
	} {
		t.Run(name, func(t *testing.T) {
			if len(schema) == 0 {
				t.Fatal("expected non-empty schema map")
			}
			if _, ok := schema["anyOf"]; !ok {
				t.Fatalf("schema missing anyOf: %#v", schema)
			}
		})
	}
}

// TestSnippetCreateInputSchemaPanicsForUnsupportedType verifies schema-generation failures are surfaced.
func TestSnippetCreateInputSchemaPanicsForUnsupportedType(t *testing.T) {
	type unsupportedSchemaInput struct {
		Callback func() `json:"callback"`
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unsupported schema type")
		}
	}()
	_ = snippetCreateInputSchema[unsupportedSchemaInput]()
}

// TestSnippetCreateInputSchemaMap_DeadBranches documents why the two
// json.Marshal and json.Unmarshal panic branches in
// snippetCreateInputSchemaMap are unreachable in practice. The function
// always receives a *jsonschema.Schema returned by jsonschema.For[T](nil),
// which is built reflectively from a Go type. The library guarantees:
//   - The generated schema passes basicChecks() (it deduplicates
//     PropertyOrder internally and never sets both Type/Types, Defs/Definitions,
//     or Items/ItemsArray simultaneously).
//   - The Schema.MarshalJSON implementation only uses standard JSON-friendly
//     Go types (strings, numbers, bools, maps, slices, *Schema), so
//     json.Marshal cannot return an error.
//   - The Schema.UnmarshalJSON implementation accepts any valid JSON object,
//     so json.Unmarshal of the marshaled bytes into map[string]any cannot fail.
//
// We assert the live happy path here as a regression guard, and the test
// also documents the rationale so future maintainers do not "fix" the
// dead branches by removing them.
func TestSnippetCreateInputSchemaMap_DeadBranches(t *testing.T) {
	// Personal and project schemas must round-trip cleanly through the
	// marshal/unmarshal dance implemented in snippetCreateInputSchemaMap.
	for name, schema := range map[string]map[string]any{
		"personal": CreateInputSchemaMap(),
		"project":  ProjectCreateInputSchemaMap(),
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(schema)
			if err != nil {
				t.Fatalf("re-marshal failed: %v", err)
			}
			var roundTrip map[string]any
			if unmarshalErr := json.Unmarshal(raw, &roundTrip); unmarshalErr != nil {
				t.Fatalf("re-unmarshal failed: %v", unmarshalErr)
			}
			if _, ok := roundTrip["anyOf"]; !ok {
				t.Fatalf("round-trip lost anyOf key: %#v", roundTrip)
			}
		})
	}
}

// TestFormatFileContentMarkdown verifies the whole render of one snippet
// file's content.
func TestFormatFileContentMarkdown(t *testing.T) {
	got := FormatFileContentMarkdown(FileContentOutput{SnippetID: 42, Ref: "main", FileName: "test.go", Content: "package main"})
	want := "## Snippet #42 File: test.go (ref: main)\n\n" +
		"```\npackage main\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'content' to get the full snippet content\n" +
		"- " + hintUpdateSnippet + "\n"
	if got != want {
		t.Errorf("snippet file content:\n got %q\nwant %q", got, want)
	}
}

// TestResolveProjectLabel_Fallback validates that resolveProjectLabel returns
// the numeric project ID when extractProjectPath fails to parse the WebURL.
func TestResolveProjectLabel_Fallback(t *testing.T) {
	out := Output{ProjectID: 99, WebURL: "not-a-url"}
	got := resolveProjectLabel(out)
	if got != "99" {
		t.Errorf("resolveProjectLabel = %q, want %q", got, "99")
	}
}

// TestActionSpecs_Get404 validates not-found and error behavior for snippet get routes.
func TestActionSpecs_Get404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	client := testutil.NewTestClient(t, mux)
	byTool := snippetSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name         string
		args         map[string]any
		expectResult bool
	}{
		{"gitlab_snippet_get", map[string]any{"snippet_id": 1}, true},
		{"gitlab_project_snippet_get", map[string]any{"project_id": "p", "snippet_id": 1}, false},
	}
	for _, tc := range tools {
		t.Run(tc.name+"_404", func(t *testing.T) {
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if tc.expectResult {
				if err != nil {
					t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
				}
				if _, ok := result.(snippetNotFoundOutput); !ok {
					t.Fatalf("result type = %T, want snippetNotFoundOutput", result)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error from %s", tc.name)
			}
		})
	}
}

// TestActionSpecs_GetForbidden verifies the personal snippet get reports a
// refusal as an error rather than as a snippet that is not there: only a 404
// means the snippet does not exist, and a 403 means it does and is not yours.
func TestActionSpecs_GetForbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	})
	byTool := snippetSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mux)))

	result, err := byTool["gitlab_snippet_get"].Route.Handler(t.Context(), map[string]any{"snippet_id": 1})
	if err == nil {
		t.Fatalf("Route.Handler = %+v, want the refusal reported as an error", result)
	}
	if _, ok := result.(snippetNotFoundOutput); ok {
		t.Error("a refused snippet was reported as one that does not exist")
	}
}

// TestResolveProjectLabel_ZeroProjectID verifies that resolveProjectLabel
// returns an empty string when the snippet has no associated project
// (ProjectID == 0, indicating a personal snippet). This targets the early
// return branch at the top of resolveProjectLabel.
func TestResolveProjectLabel_ZeroProjectID(t *testing.T) {
	got := resolveProjectLabel(Output{ProjectID: 0, WebURL: "https://gitlab.example.com/snippets/42"})
	if got != "" {
		t.Errorf("resolveProjectLabel(ProjectID=0) = %q, want empty string", got)
	}
}

// TestActionSpecs_SnippetGetRoute verifies the canonical personal snippet get route output.
func TestActionSpecs_SnippetGetRoute(t *testing.T) {
	const respJSON = `{"id":33,"title":"hello","file_name":"hello.txt","description":"","visibility":"public","author":{"id":1,"username":"u","name":"u"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/snippets/33") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := snippetSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_snippet_get"].Route.Handler(t.Context(), map[string]any{"snippet_id": 33})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 33 || out.Title != "hello" {
		t.Fatalf("snippet output = %#v, want ID 33 title hello", out)
	}
}

// snippetSpecsByTool supports snippet specs by tool assertions in snippets tests.
func snippetSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// snippetWithStorageJSON is a snippet payload that exercises the full author
// object (including created_at) and the repository_storage field added by the
// 1:1 audit.
const snippetWithStorageJSON = `{"id":7,"title":"Audit","file_name":"a.go","description":"","visibility":"public",` +
	`"author":{"id":3,"username":"dev","email":"dev@example.com","name":"Dev","state":"active","created_at":"2024-01-02T03:04:05Z"},` +
	`"project_id":0,"web_url":"https://gitlab.example.com/snippets/7","raw_url":"https://gitlab.example.com/snippets/7/raw",` +
	`"repository_storage":"default","files":[{"path":"a.go","raw_url":"https://gitlab.example.com/snippets/7/raw/main/a.go"}]}`

// TestConvertSnippet_FullAuthorAndStorage verifies that convertSnippet surfaces
// the full nested author object (with created_at) and repository_storage, per
// the 1:1 audit policy.
func TestConvertSnippet_FullAuthorAndStorage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/7", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, snippetWithStorageJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Get(context.Background(), client, GetInput{SnippetID: 7})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.RepositoryStorage != "default" {
		t.Errorf("RepositoryStorage = %q, want default", out.RepositoryStorage)
	}
	if out.Author == nil {
		t.Fatal("Author is nil, want full author object")
	}
	if out.Author.Email != "dev@example.com" || out.Author.State != "active" {
		t.Errorf("Author = %+v, want email/state populated", out.Author)
	}
	if out.Author.CreatedAt == nil || !out.Author.CreatedAt.Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("Author.CreatedAt = %v, want 2024-01-02T03:04:05Z", out.Author.CreatedAt)
	}
}

// TestList_KeysetAndOrdering verifies List forwards order_by, sort, pagination,
// and page_token onto the GitLab query, covering the keyset wiring.
func TestList_KeysetAndOrdering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := List(context.Background(), client, ListInput{
		OrderBy:    "created_at",
		Sort:       "desc",
		PerPage:    50,
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	for key, want := range map[string]string{
		"order_by": "created_at", "sort": "desc", "pagination": "keyset", "page_token": "tok", "per_page": "50",
	} {
		t.Run(key, func(t *testing.T) {
			if got.Get(key) != want {
				t.Errorf("query %s = %q, want %q", key, got.Get(key), want)
			}
		})
	}
}

// TestExplore_Ordering verifies Explore forwards order_by and sort.
func TestExplore_Ordering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/public", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	if _, err := Explore(context.Background(), client, ExploreInput{OrderBy: "updated_at", Sort: "asc"}); err != nil {
		t.Fatalf("Explore returned error: %v", err)
	}
	if got.Get("order_by") != "updated_at" || got.Get("sort") != "asc" {
		t.Errorf("explore order_by/sort = %q/%q, want updated_at/asc", got.Get("order_by"), got.Get("sort"))
	}
}

// TestListAll_RepositoryStorageAndOrdering verifies ListAll forwards the
// repository_storage filter alongside order_by and sort.
func TestListAll_RepositoryStorageAndOrdering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/all", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListAll(context.Background(), client, ListAllInput{
		RepositoryStorage: "nfs-01",
		OrderBy:           "id",
		Sort:              "asc",
	})
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}
	if got.Get("repository_storage") != "nfs-01" {
		t.Errorf("repository_storage = %q, want nfs-01", got.Get("repository_storage"))
	}
	if got.Get("order_by") != "id" || got.Get("sort") != "asc" {
		t.Errorf("order_by/sort = %q/%q, want id/asc", got.Get("order_by"), got.Get("sort"))
	}
}

// TestApplyOrderSort_NilOpts verifies applyOrderSort is a safe no-op when the
// options pointer is nil.
func TestApplyOrderSort_NilOpts(t *testing.T) {
	applyOrderSort(nil, "created_at", "desc") // must not panic
}

// TestAuthorUsername_Nil verifies authorUsername returns an empty string for a
// nil author object (minimal payloads).
func TestAuthorUsername_Nil(t *testing.T) {
	if got := authorUsername(nil); got != "" {
		t.Errorf("authorUsername(nil) = %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// Option builders: what a request carries and what it leaves out
// ---------------------------------------------------------------------------.

// TestSnippetVisibility_DefaultsToPrivate verifies a snippet created without a
// visibility is private, and that a visibility the caller named is kept.
func TestSnippetVisibility_DefaultsToPrivate(t *testing.T) {
	for name, want := range map[string]string{"": "private", "public": "public", "internal": "internal"} {
		t.Run("visibility "+name, func(t *testing.T) {
			if got := snippetVisibility(name); got == nil || string(*got) != want {
				t.Errorf("snippetVisibility(%q) = %v, want %q", name, got, want)
			}
		})
	}
}

// TestBuildUpdateOpts_SendsOnlyWhatTheCallerGave verifies the personal snippet
// update sends every field the caller named and nothing else, so an omitted
// field keeps its current value rather than being cleared.
func TestBuildUpdateOpts_SendsOnlyWhatTheCallerGave(t *testing.T) {
	opts := buildUpdateOpts(UpdateInput{
		SnippetID: 42, Title: "Renamed", FileName: "new.go", Description: "desc",
		ContentBody: "package main", Visibility: "public",
		Files: []UpdateFileInput{{Action: "update", FilePath: "new.go", Content: "x", PreviousPath: "old.go"}},
	})
	want := map[string]string{
		"title": "Renamed", "file_name": "new.go", "description": "desc", "content": "package main",
	}
	for field, got := range map[string]*string{
		"title":       opts.Title,
		"file_name":   opts.FileName,
		"description": opts.Description,
		"content":     opts.Content,
	} {
		t.Run(field, func(t *testing.T) { assertStringPtr(t, field, got, want[field]) })
	}
	if opts.Visibility == nil || string(*opts.Visibility) != "public" {
		t.Errorf("Visibility = %v, want public", opts.Visibility)
	}
	if opts.Files == nil || len(*opts.Files) != 1 {
		t.Fatalf("Files = %v, want the one file operation", opts.Files)
	}
	file := (*opts.Files)[0]
	assertStringPtr(t, "files[0].content", file.Content, "x")
	assertStringPtr(t, "files[0].previous_path", file.PreviousPath, "old.go")
}

// assertStringPtr reports an optional request field that is missing or is not
// the value the caller gave.
func assertStringPtr(t *testing.T, field string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %q", field, got, want)
	}
}

// TestBuildUpdateOpts_WithoutAnyFieldSendsNothing verifies an update naming no
// field at all sends none, so every value the snippet has keeps its place.
func TestBuildUpdateOpts_WithoutAnyFieldSendsNothing(t *testing.T) {
	opts := buildUpdateOpts(UpdateInput{SnippetID: 42})
	if opts.Title != nil || opts.FileName != nil || opts.Description != nil ||
		opts.Content != nil || opts.Visibility != nil || opts.Files != nil {
		t.Errorf("update options = %+v, want every field left alone", opts)
	}
}

// TestBuildUpdateFileOpts_WithoutContentOrPreviousPath verifies a file
// operation that names neither sends neither, which is what a delete is.
func TestBuildUpdateFileOpts_WithoutContentOrPreviousPath(t *testing.T) {
	files := buildUpdateFileOpts([]UpdateFileInput{{Action: "delete", FilePath: "gone.go"}})
	if files == nil || len(*files) != 1 {
		t.Fatalf("files = %v, want the one operation", files)
	}
	if (*files)[0].Content != nil || (*files)[0].PreviousPath != nil {
		t.Errorf("file operation = %+v, want neither content nor a previous path", (*files)[0])
	}
}

// TestListAll_CreatedFilters verifies the admin listing sends the two date
// filters when they parse and leaves them out when they do not, since a
// half-understood filter would silently narrow the answer.
func TestListAll_CreatedFilters(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input ListAllInput
		want  []string
		omit  []string
	}{
		{
			name: "dates that parse",
			input: ListAllInput{
				CreatedAfter: "2026-01-01T00:00:00Z", CreatedBefore: "2026-02-01T00:00:00Z",
				RepositoryStorage: "nfs-01",
			},
			want: []string{"created_after=2026-01-01", "created_before=2026-02-01", "repository_storage=nfs-01"},
		},
		{
			name:  "dates that do not parse",
			input: ListAllInput{CreatedAfter: "yesterday", CreatedBefore: "tomorrow"},
			omit:  []string{"created_after", "created_before", "repository_storage"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var query string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.RawQuery
				testutil.RespondJSON(w, http.StatusOK, `[`+snippetSentJSON+`]`)
			}))
			if _, err := ListAll(context.Background(), client, testCase.input); err != nil {
				t.Fatalf("ListAll: %v", err)
			}
			for _, want := range testCase.want {
				if !strings.Contains(query, want) {
					t.Errorf("query %q missing %q", query, want)
				}
			}
			for _, omit := range testCase.omit {
				if strings.Contains(query, omit) {
					t.Errorf("query %q carries %q it could not read", query, omit)
				}
			}
		})
	}
}

// TestCreate_SendsOnlyWhatTheCallerGave verifies the personal snippet create
// sends the single-file fields and the description only when the caller named
// them, and always names a visibility.
func TestCreate_SendsOnlyWhatTheCallerGave(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input CreateInput
		want  []string
		omit  []string
	}{
		{
			name: "single file with a description",
			input: CreateInput{
				Title: "Test Snippet", FileName: "test.go", Description: "desc",
				ContentBody: "package main", Visibility: "public",
			},
			want: []string{`"title":"Test Snippet"`, `"file_name":"test.go"`, `"description":"desc"`, `"content":"package main"`, `"visibility":"public"`},
		},
		{
			name: "files array only",
			input: CreateInput{
				Title: "Test Snippet",
				Files: []CreateFileInput{{FilePath: "a.go", Content: "package a"}},
			},
			want: []string{`"title":"Test Snippet"`, `"files"`, `"file_path":"a.go"`, `"visibility":"private"`},
			// The content of the one file is inside files[], and the
			// single-file keys beside it are not sent at all.
			omit: []string{`"file_name"`, `"description"`},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var body string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					t.Errorf("read request body: %v", readErr)
				}
				body = string(raw)
				testutil.RespondJSON(w, http.StatusCreated, snippetSentJSON)
			}))
			if _, err := Create(context.Background(), client, testCase.input); err != nil {
				t.Fatalf("Create: %v", err)
			}
			for _, want := range testCase.want {
				if !strings.Contains(body, want) {
					t.Errorf("create body %q missing %q", body, want)
				}
			}
			for _, omit := range testCase.omit {
				if strings.Contains(body, omit) {
					t.Errorf("create body %q carries %q it was not given", body, omit)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fields GitLab sends beside the ones the SDK's own Snippet models
// ---------------------------------------------------------------------------.

// snippetSentJSON is one snippet as GitLab renders it once its repository
// exists, carrying the keys the SDK's own Snippet leaves out: the two clone
// URLs and where an imported snippet came from.
const snippetSentJSON = `{"id":42,"title":"Test Snippet","file_name":"test.go","visibility":"private",` +
	`"web_url":"https://gitlab.example.com/snippets/42",` +
	`"ssh_url_to_repo":"git@gitlab.example.com:snippets/42.git",` +
	`"http_url_to_repo":"https://gitlab.example.com/snippets/42.git",` +
	`"imported":true,"imported_from":"github"}`

// snippetWithoutRepoJSON is a snippet whose repository does not exist yet and
// which was written here rather than imported.
const snippetWithoutRepoJSON = `{"id":42,"title":"Test Snippet","file_name":"test.go",` +
	`"visibility":"private","web_url":"https://gitlab.example.com/snippets/42",` +
	`"imported":false,"imported_from":null}`

// snippetsClient answers every snippet endpoint with body, which the caller
// writes as an array for a list handler and as an object for the rest.
func snippetsClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// errNoSnippet reports a list handler that answered with no snippet.
var errNoSnippet = errors.New("the handler published no snippet")

// snippetCalls are every handler that answers with a snippet, personal and
// project-scoped alike, since one shape serves both.
var snippetCalls = []struct {
	name string
	list bool
	call func(client *gitlabclient.Client) (Output, error)
}{
	{name: "list", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		return firstSnippet(List(context.Background(), client, ListInput{}))
	}},
	{name: "list_all", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		return firstSnippet(ListAll(context.Background(), client, ListAllInput{}))
	}},
	{name: "explore", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		return firstSnippet(Explore(context.Background(), client, ExploreInput{}))
	}},
	{name: "get", call: func(client *gitlabclient.Client) (Output, error) {
		return Get(context.Background(), client, GetInput{SnippetID: 42})
	}},
	{name: "create", call: func(client *gitlabclient.Client) (Output, error) {
		return Create(context.Background(), client, CreateInput{
			Title: "Test Snippet", FileName: "test.go", ContentBody: "package main",
		})
	}},
	{name: "update", call: func(client *gitlabclient.Client) (Output, error) {
		return Update(context.Background(), client, UpdateInput{SnippetID: 42, Title: "Renamed"})
	}},
	{name: "project_list", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		return firstSnippet(ProjectList(context.Background(), client, ProjectListInput{ProjectID: "42"}))
	}},
	{name: "project_get", call: func(client *gitlabclient.Client) (Output, error) {
		return ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "42", SnippetID: 42})
	}},
	{name: "project_create", call: func(client *gitlabclient.Client) (Output, error) {
		return ProjectCreate(context.Background(), client, ProjectCreateInput{
			ProjectID: "42", Title: "Test Snippet", FileName: "test.go", ContentBody: "package main",
		})
	}},
	{name: "project_update", call: func(client *gitlabclient.Client) (Output, error) {
		return ProjectUpdate(context.Background(), client, ProjectUpdateInput{
			ProjectID: "42", SnippetID: 42, Title: "Renamed",
		})
	}},
}

// firstSnippet reduces a list answer to its single snippet, reporting a list
// that carried none rather than panicking on the index.
func firstSnippet(out ListOutput, err error) (Output, error) {
	if err != nil {
		return Output{}, err
	}
	if len(out.Snippets) != 1 {
		return Output{}, errNoSnippet
	}
	return out.Snippets[0], nil
}

// snippetBodyFor wraps the object body in an array for a list endpoint.
func snippetBodyFor(list bool, body string) string {
	if list {
		return "[" + body + "]"
	}
	return body
}

// TestSnippets_PublishTheFieldsGitLabSendsBesideTheSDKs verifies every handler
// answering with a snippet publishes the two clone URLs and the import origin,
// read off the captured response.
func TestSnippets_PublishTheFieldsGitLabSendsBesideTheSDKs(t *testing.T) {
	for _, snippetCall := range snippetCalls {
		t.Run(snippetCall.name, func(t *testing.T) {
			out, err := snippetCall.call(snippetsClient(t, snippetBodyFor(snippetCall.list, snippetSentJSON)))
			if err != nil {
				t.Fatalf("%s: %v", snippetCall.name, err)
			}
			if out.SSHURLToRepo != "git@gitlab.example.com:snippets/42.git" {
				t.Errorf("ssh_url_to_repo = %q, want the clone URL GitLab sent", out.SSHURLToRepo)
			}
			if out.HTTPURLToRepo != "https://gitlab.example.com/snippets/42.git" {
				t.Errorf("http_url_to_repo = %q, want the clone URL GitLab sent", out.HTTPURLToRepo)
			}
			if !out.Imported {
				t.Error("imported = false, want true")
			}
			if out.ImportedFrom != "github" {
				t.Errorf("imported_from = %q, want github", out.ImportedFrom)
			}
		})
	}
}

// TestSnippets_OmitTheCloneURLsGitLabDidNotSend verifies a snippet whose
// repository does not exist publishes neither clone URL, and one written here
// says it was not imported.
func TestSnippets_OmitTheCloneURLsGitLabDidNotSend(t *testing.T) {
	for _, snippetCall := range snippetCalls {
		t.Run(snippetCall.name, func(t *testing.T) {
			out, err := snippetCall.call(snippetsClient(t, snippetBodyFor(snippetCall.list, snippetWithoutRepoJSON)))
			if err != nil {
				t.Fatalf("%s: %v", snippetCall.name, err)
			}
			if out.SSHURLToRepo != "" || out.HTTPURLToRepo != "" {
				t.Errorf("clone URLs = %q / %q, want none", out.SSHURLToRepo, out.HTTPURLToRepo)
			}
			if out.Imported {
				t.Error("imported = true, want false for a snippet written here")
			}
			if out.ImportedFrom != "" {
				t.Errorf("imported_from = %q, want none", out.ImportedFrom)
			}
		})
	}
}

// TestSnippets_UnreadableFields verifies every handler answering with a
// snippet reports a decode failure rather than a snippet missing what GitLab
// sent. client-go models the imported flag on its own Snippet as of v3.12.0,
// so the SDK's decoder is what refuses it now that the captured read is
// retired.
func TestSnippets_UnreadableFields(t *testing.T) {
	const poisoned = `{"id":42,"title":"Test Snippet","imported":"yes"}`
	cases := make([]testutil.CapturedCase, 0, len(snippetCalls))
	for _, snippetCall := range snippetCalls {
		cases = append(cases, testutil.CapturedCase{Name: snippetCall.name, Call: func() error {
			_, err := snippetCall.call(snippetsClient(t, snippetBodyFor(snippetCall.list, poisoned)))
			return err
		}})
	}
	testutil.AssertUnreadableBodyRefused(t, cases)
}

// TestFormatMarkdown_FilesAndHintsFollowTheSnippet verifies the file table is
// written only for a snippet that has files, and that a project snippet is
// given the project actions while a personal one is given the personal ones.
func TestFormatMarkdown_FilesAndHintsFollowTheSnippet(t *testing.T) {
	const rawURL = "https://gitlab.example.com/raw/test.go"
	withFiles := FormatMarkdown(Output{
		ID: 42, Title: "Test Snippet", ProjectID: 7,
		Files: []FileOutput{{Path: "test.go", RawURL: rawURL}},
	})
	wantFiles := "## Snippet #42: Test Snippet\n\n" +
		"- **ID**: 42\n" +
		"- **Title**: Test Snippet\n" +
		"- **Project ID**: 7\n" +
		"\n### Files\n\n| Path | Raw URL |\n| --- | --- |\n| test.go | [" + rawURL + "](" + rawURL + ") |\n" +
		snippetCardHints(true, true)
	if withFiles != wantFiles {
		t.Errorf("project snippet with files:\n got %q\nwant %q", withFiles, wantFiles)
	}

	personal := FormatMarkdown(Output{ID: 42, Title: "Test Snippet"})
	wantPersonal := "## Snippet #42: Test Snippet\n\n" +
		"- **ID**: 42\n" +
		"- **Title**: Test Snippet\n" +
		snippetCardHints(false, false)
	if personal != wantPersonal {
		t.Errorf("personal snippet without files:\n got %q\nwant %q", personal, wantPersonal)
	}
}

// TestApplySnippetMeta_WithoutAnEntryKeepsThePlaceholders verifies the
// decorator leaves the generic usage, the tool-name alias and the empty
// description in place for an entry that names none of them. No action reaches
// that fallback today, so the applier is called directly.
func TestApplySnippetMeta_WithoutAnEntryKeepsThePlaceholders(t *testing.T) {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{"gitlab_snippet_get"},
		Usage:   "Use to execute snippets domain action.",
	}
	applySnippetMeta(&options, snippetActionMetaEntry{})
	if options.Usage != "Use to execute snippets domain action." {
		t.Errorf("Usage = %q, want the placeholder", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_snippet_get" {
		t.Errorf("Aliases = %v, want the tool name alone", options.Aliases)
	}
	if options.RelatedActions != nil || options.IndividualTool.Description != "" {
		t.Errorf("options carry %v / %q, want neither", options.RelatedActions, options.IndividualTool.Description)
	}
}

// TestFormatMarkdown_SentFields verifies the snippet Markdown names the clone
// URLs and the import origin read off the captured answer, and leaves each of
// them out of a snippet that carries none.
func TestFormatMarkdown_SentFields(t *testing.T) {
	t.Run("shows what GitLab sent", func(t *testing.T) {
		got := FormatMarkdown(Output{
			ID: 42, Title: "Test Snippet", Visibility: "private",
			SSHURLToRepo:  "git@gitlab.example.com:snippets/42.git",
			HTTPURLToRepo: "https://gitlab.example.com/snippets/42.git",
			Imported:      true, ImportedFrom: "github",
		})
		want := "## Snippet #42: Test Snippet\n\n" +
			"- **ID**: 42\n" +
			"- **Title**: Test Snippet\n" +
			"- **Visibility**: private\n" +
			"- **SSH URL to Repo**: `git@gitlab.example.com:snippets/42.git`\n" +
			"- **HTTP URL to Repo**: `https://gitlab.example.com/snippets/42.git`\n" +
			"- **Imported**\n" +
			"- **Imported From**: github\n" +
			snippetCardHints(false, false)
		if got != want {
			t.Errorf("imported snippet card:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("omits what it did not", func(t *testing.T) {
		got := FormatMarkdown(Output{ID: 42, Title: "Test Snippet", Visibility: "private"})
		want := "## Snippet #42: Test Snippet\n\n" +
			"- **ID**: 42\n" +
			"- **Title**: Test Snippet\n" +
			"- **Visibility**: private\n" +
			snippetCardHints(false, false)
		if got != want {
			t.Errorf("bare snippet card:\n got %q\nwant %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// The whole conversion, key by key
// ---------------------------------------------------------------------------.

// snippetEveryFieldJSON is one snippet in which no two values agree, neither
// between the snippet's own keys nor between the author's, so no output below
// can be produced by a converter that reads a neighboring key.
const snippetEveryFieldJSON = `{"id":11,"title":"t-title","file_name":"n-file.go",` +
	`"description":"d-description","visibility":"internal",` +
	`"author":{"id":22,"username":"u-username","email":"e-mail@example.test",` +
	`"name":"n-name","state":"s-state","created_at":"2021-01-01T01:01:01Z"},` +
	`"project_id":33,"web_url":"https://example.test/w-web",` +
	`"raw_url":"https://example.test/r-raw","repository_storage":"s-storage",` +
	`"files":[{"path":"p-path.go","raw_url":"https://example.test/f-fileraw"}],` +
	`"ssh_url_to_repo":"git@example.test:s-ssh.git",` +
	`"http_url_to_repo":"https://example.test/h-http.git",` +
	`"imported":true,"imported_from":"i-origin",` +
	`"created_at":"2022-02-02T02:02:02Z","updated_at":"2023-03-03T03:03:03Z"}`

// TestSnippets_EveryFieldComesFromItsOwnKey verifies that every handler
// answering with a snippet fills each published field from the key GitLab
// spells it under, the author object and the file rows included.
//
// A converter that assigns its neighbor, or stops assigning a field at all,
// has no branch to flip, so neither coverage gate can see it. Measured here
// before this test existed, ten of the seventeen assignments could be dropped
// and three pairs swapped with the whole suite still green.
func TestSnippets_EveryFieldComesFromItsOwnKey(t *testing.T) {
	authorCreated := time.Date(2021, 1, 1, 1, 1, 1, 0, time.UTC)
	created := time.Date(2022, 2, 2, 2, 2, 2, 0, time.UTC)
	updated := time.Date(2023, 3, 3, 3, 3, 3, 0, time.UTC)
	want := Output{
		ID: 11, Title: "t-title", FileName: "n-file.go",
		Description: "d-description", Visibility: "internal",
		Author: &SnippetAuthorOutput{
			ID: 22, Username: "u-username", Email: "e-mail@example.test",
			Name: "n-name", State: "s-state", CreatedAt: &authorCreated,
		},
		ProjectID:         33,
		WebURL:            "https://example.test/w-web",
		RawURL:            "https://example.test/r-raw",
		RepositoryStorage: "s-storage",
		Files:             []FileOutput{{Path: "p-path.go", RawURL: "https://example.test/f-fileraw"}},
		SSHURLToRepo:      "git@example.test:s-ssh.git",
		HTTPURLToRepo:     "https://example.test/h-http.git",
		Imported:          true, ImportedFrom: "i-origin",
		CreatedAt: &created, UpdatedAt: &updated,
	}
	for _, snippetCall := range snippetCalls {
		t.Run(snippetCall.name, func(t *testing.T) {
			got, err := snippetCall.call(snippetsClient(t, snippetBodyFor(snippetCall.list, snippetEveryFieldJSON)))
			if err != nil {
				t.Fatalf("%s: %v", snippetCall.name, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("snippet:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// snippetListCalls are the four handlers that answer with a page of snippets.
var snippetListCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (ListOutput, error)
}{
	{name: "list", call: func(client *gitlabclient.Client) (ListOutput, error) {
		return List(context.Background(), client, ListInput{})
	}},
	{name: "list_all", call: func(client *gitlabclient.Client) (ListOutput, error) {
		return ListAll(context.Background(), client, ListAllInput{})
	}},
	{name: "explore", call: func(client *gitlabclient.Client) (ListOutput, error) {
		return Explore(context.Background(), client, ExploreInput{})
	}},
	{name: "project_list", call: func(client *gitlabclient.Client) (ListOutput, error) {
		return ProjectList(context.Background(), client, ProjectListInput{ProjectID: "42"})
	}},
}

// TestSnippets_PaginationComesFromTheResponseHeaders verifies every snippet
// listing publishes the page GitLab answered with, each field from its own
// header and no two of them equal.
//
// The block is one straight assignment from the response, so a listing that
// filled none of it would leave both gates green while telling a caller there
// is no second page to ask for.
func TestSnippets_PaginationComesFromTheResponseHeaders(t *testing.T) {
	want := toolutil.PaginationOutput{
		Page: 3, PerPage: 25, TotalItems: 97, TotalPages: 4,
		NextPage: 5, PrevPage: 2, HasMore: true,
	}
	for _, listCall := range snippetListCalls {
		t.Run(listCall.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON, testutil.PaginationHeaders{
					Page: "3", PerPage: "25", Total: "97", TotalPages: "4", NextPage: "5", PrevPage: "2",
				})
			}))
			out, err := listCall.call(client)
			if err != nil {
				t.Fatalf("%s: %v", listCall.name, err)
			}
			if out.Pagination != want {
				t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
			}
		})
	}
}

// TestFormatMarkdown_AuthorRendersWhicheverHalfGitLabSent verifies the author
// row carries the display name and the handle when GitLab sent both, whichever
// one it sent alone, and no row at all when it sent neither or no author.
// Only the "both" arm was exercised before, which left the other two arms of
// the switch unreached by every test in the package.
func TestFormatMarkdown_AuthorRendersWhicheverHalfGitLabSent(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		author *SnippetAuthorOutput
		want   string
	}{
		{name: "name and handle", author: &SnippetAuthorOutput{Name: "A Name", Username: "handle"}, want: "- **Author**: A Name (@handle)\n"},
		{name: "handle alone", author: &SnippetAuthorOutput{Username: "handle"}, want: "- **Author**: @handle\n"},
		{name: "name alone", author: &SnippetAuthorOutput{Name: "A Name"}, want: "- **Author**: A Name\n"},
		{name: "neither", author: &SnippetAuthorOutput{ID: 7}, want: ""},
		{name: "no author", author: nil, want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := FormatMarkdown(Output{ID: 1, Title: "T", Author: testCase.author})
			if testCase.want == "" {
				if strings.Contains(got, "**Author**") {
					t.Errorf("card %q carries an author row, want none", got)
				}
				return
			}
			if !strings.Contains(got, testCase.want) {
				t.Errorf("card %q missing %q", got, testCase.want)
			}
		})
	}
}

// TestFormatMarkdown_LinkInstructionFollowsTheFilesRawURLs verifies a card
// whose only file carries no raw URL is not told to preserve links, and that
// one file with a URL among several without is enough for the instruction.
// The loop over the files used to be entered with a URL on the first entry
// every time, so the empty case was never taken.
func TestFormatMarkdown_LinkInstructionFollowsTheFilesRawURLs(t *testing.T) {
	noLinks := FormatMarkdown(Output{ID: 1, Title: "T", Files: []FileOutput{{Path: "a.go"}, {Path: "b.go"}}})
	if strings.Contains(noLinks, toolutil.HintPreserveLinks) {
		t.Errorf("card with no raw URL %q asks for links to be preserved", noLinks)
	}
	oneLink := FormatMarkdown(Output{ID: 1, Title: "T", Files: []FileOutput{{Path: "a.go"}, {Path: "b.go", RawURL: "https://example.test/raw"}}})
	if !strings.Contains(oneLink, toolutil.HintPreserveLinks) {
		t.Errorf("card whose second file has a raw URL %q does not ask for links to be preserved", oneLink)
	}
}

// snippets_test.go contains unit tests for the snippet MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package snippets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	client := testutil.NewTestClient(t, http.NewServeMux())
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
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Content(context.Background(), client, ContentInput{})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// FileContent
// ---------------------------------------------------------------------------.

// TestFileContent_Success verifies FileContent when success.
func TestFileContent_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/42/files/main/test.go/raw", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content here"))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := FileContent(context.Background(), client, FileContentInput{
		SnippetID: 42, Ref: "main", FileName: "test.go",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Content != "file content here" {
		t.Errorf("unexpected content: %s", out.Content)
	}
}

// TestFileContent_MissingParams verifies FileContent when missing params.
func TestFileContent_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

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
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), "package main") {
			t.Fatalf("request body = %q, want snippet content", string(body))
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
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// TestCreate_MissingContent verifies Create when missing content.
func TestCreate_MissingContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{Title: "Test Snippet", FileName: "test.go"})
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("expected content required error, got %v", err)
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
	client := testutil.NewTestClient(t, http.NewServeMux())
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
	client := testutil.NewTestClient(t, http.NewServeMux())
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

// TestFormatMarkdown verifies FormatMarkdown.
func TestFormatMarkdown(t *testing.T) {
	out := Output{
		ID: 42, Title: "Test", Visibility: "private",
		Author: AuthorOutput{Name: "Admin", Username: "admin"},
		WebURL: "https://example.com/snippets/42",
	}
	md := FormatMarkdown(out)
	if !strings.Contains(md, "Test") || !strings.Contains(md, "@admin") {
		t.Errorf("unexpected markdown: %s", md)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Snippets: []Output{{ID: 1, Title: "S1", Visibility: "public", Author: AuthorOutput{Username: "u1"}}},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "S1") {
		t.Errorf("unexpected markdown: %s", md)
	}
}

// TestFormatContentMarkdown verifies FormatContentMarkdown.
func TestFormatContentMarkdown(t *testing.T) {
	out := ContentOutput{SnippetID: 42, Content: "hello world"}
	md := FormatContentMarkdown(out)
	if !strings.Contains(md, "hello world") {
		t.Errorf("unexpected markdown: %s", md)
	}
}

// TestFormatFileContentMarkdown verifies FormatFileContentMarkdown.
func TestFormatFileContentMarkdown(t *testing.T) {
	out := FileContentOutput{SnippetID: 42, Ref: "main", FileName: "test.go", Content: "package main"}
	md := FormatFileContentMarkdown(out)
	if !strings.Contains(md, "test.go") || !strings.Contains(md, "package main") {
		t.Errorf("unexpected markdown: %s", md)
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

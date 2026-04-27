// snippets_test.go contains unit tests for the snippet MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package snippets

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const snippetJSON = `{"id":42,"title":"Test Snippet","file_name":"test.go","description":"A test","visibility":"private","author":{"id":1,"username":"admin","name":"Admin","email":"admin@example.com","state":"active"},"project_id":0,"web_url":"https://gitlab.example.com/snippets/42","raw_url":"https://gitlab.example.com/snippets/42/raw","files":[{"path":"test.go","raw_url":"https://gitlab.example.com/snippets/42/raw/main/test.go"}]}`

const snippetListJSON = `[` + snippetJSON + `]`

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// TestList_Success verifies the behavior of list success.
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

// TestListAll_Success verifies the behavior of list all success.
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

// TestGet_Success verifies the behavior of get success.
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

// TestGet_MissingSnippetID verifies the behavior of get missing snippet i d.
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

// TestContent_Success verifies the behavior of content success.
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

// TestContent_MissingSnippetID verifies the behavior of content missing snippet i d.
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

// TestFileContent_Success verifies the behavior of file content success.
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

// TestFileContent_MissingParams verifies the behavior of file content missing params.
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

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Create(context.Background(), client, CreateInput{
		Title:      "Test Snippet",
		FileName:   "test.go",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestCreate_MissingTitle verifies the behavior of create missing title.
func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------.

// TestUpdate_Success verifies the behavior of update success.
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

// TestUpdate_MissingSnippetID verifies the behavior of update missing snippet i d.
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

// TestDelete_Success verifies the behavior of delete success.
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

// TestDelete_MissingSnippetID verifies the behavior of delete missing snippet i d.
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

// TestExplore_Success verifies the behavior of explore success.
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

// TestFormatMarkdown verifies the behavior of format markdown.
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

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Snippets: []Output{{ID: 1, Title: "S1", Visibility: "public", Author: AuthorOutput{Username: "u1"}}},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "S1") {
		t.Errorf("unexpected markdown: %s", md)
	}
}

// TestFormatContentMarkdown verifies the behavior of format content markdown.
func TestFormatContentMarkdown(t *testing.T) {
	out := ContentOutput{SnippetID: 42, Content: "hello world"}
	md := FormatContentMarkdown(out)
	if !strings.Contains(md, "hello world") {
		t.Errorf("unexpected markdown: %s", md)
	}
}

// TestFormatFileContentMarkdown verifies the behavior of format file content markdown.
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

// TestMCPRoundTrip_Get404 validates that the snippet get tool returns NotFound
// for 404 responses (register.go 404 paths).
func TestMCPRoundTrip_Get404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_snippet_get", map[string]any{"snippet_id": 1}},
		{"gitlab_project_snippet_get", map[string]any{"project_id": "p", "snippet_id": 1}},
	}
	for _, tc := range tools {
		t.Run(tc.name+"_404", func(t *testing.T) {
			res, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if callErr != nil {
				t.Fatalf("CallTool: %v", callErr)
			}
			if !res.IsError {
				t.Error("expected IsError=true for 404")
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

// TestSnippetGet_EmbedsCanonicalResource asserts gitlab_snippet_get
// attaches an EmbeddedResource block with URI gitlab://snippet/{id}.
func TestSnippetGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":33,"title":"hello","file_name":"hello.txt","description":"","visibility":"public","author":{"id":1,"username":"u","name":"u"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/snippets/33") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"snippet_id": 33}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_snippet_get", args, "gitlab://snippet/33", toolutil.EnableEmbeddedResources)
}

// project_snippets_test.go contains unit tests for the snippet MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package snippets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// errSnippetIDRequired identifies the err snippet ID required constant used by this package.
const errSnippetIDRequired = "snippet_id is required"

// errProjectIDRequired identifies the err project ID required constant used by this package.
const errProjectIDRequired = "project_id is required"

const (
	// fmtExpProjIDReqErr identifies the fmt exp proj ID req err constant used by this package.
	fmtExpProjIDReqErr = "expected project_id required error, got %v"
	// fmtExpSnippetIDReqErr identifies the fmt exp snippet ID req err constant used by this package.
	fmtExpSnippetIDReqErr = "expected snippet_id required error, got %v"
	// pathSnippet42 identifies the path snippet 42 constant used by this package.
	pathSnippet42 = "/api/v4/projects/10/snippets/42"
	// msgMethodNotAllowed identifies the msg method not allowed constant used by this package.
	msgMethodNotAllowed = "method not allowed"
	// fmtExpID42 identifies the fmt exp ID 42 constant used by this package.
	fmtExpID42 = "expected ID 42, got %d"
)

// ---------------------------------------------------------------------------
// ProjectList
// ---------------------------------------------------------------------------.

// TestProjectList_Success verifies ProjectList when success.
func TestProjectList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/snippets", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: toolutil.StringOrInt("10")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(out.Snippets))
	}
}

// TestProjectList_MissingProjectID verifies ProjectList when missing project ID.
func TestProjectList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectList(context.Background(), client, ProjectListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}
}

// ---------------------------------------------------------------------------
// ProjectGet
// ---------------------------------------------------------------------------.

// TestProjectGet_Success verifies ProjectGet when success.
func TestProjectGet_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathSnippet42, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ProjectGet(context.Background(), client, ProjectGetInput{
		ProjectID: toolutil.StringOrInt("10"), SnippetID: 42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf(fmtExpID42, out.ID)
	}
}

// TestProjectGet_MissingParams verifies ProjectGet when missing params.
func TestProjectGet_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectGet(context.Background(), client, ProjectGetInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}

	_, err = ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errSnippetIDRequired) {
		t.Fatalf(fmtExpSnippetIDReqErr, err)
	}
}

// ---------------------------------------------------------------------------
// ProjectContent
// ---------------------------------------------------------------------------.

// TestProjectContent_Success verifies ProjectContent when success.
func TestProjectContent_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/snippets/42/raw", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("project snippet content"))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ProjectContent(context.Background(), client, ProjectContentInput{
		ProjectID: toolutil.StringOrInt("10"), SnippetID: 42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Content != "project snippet content" {
		t.Errorf("unexpected content: %s", out.Content)
	}
}

// TestProjectContent_MissingParams verifies ProjectContent when missing params.
func TestProjectContent_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectContent(context.Background(), client, ProjectContentInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}

	_, err = ProjectContent(context.Background(), client, ProjectContentInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errSnippetIDRequired) {
		t.Fatalf(fmtExpSnippetIDReqErr, err)
	}
}

// ---------------------------------------------------------------------------
// ProjectCreate
// ---------------------------------------------------------------------------.

// TestProjectCreate_Success verifies ProjectCreate when success.
func TestProjectCreate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   toolutil.StringOrInt("10"),
		Title:       "Test Snippet",
		FileName:    "test.go",
		ContentBody: "package main",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf(fmtExpID42, out.ID)
	}
}

// TestProjectCreate_MissingParams verifies ProjectCreate when missing params.
func TestProjectCreate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectCreate(context.Background(), client, ProjectCreateInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}

	_, err = ProjectCreate(context.Background(), client, ProjectCreateInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// TestProjectCreate_MissingContent verifies that project create validates file
// content before calling GitLab.
func TestProjectCreate_MissingContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID: toolutil.StringOrInt("10"),
		Title:     "Test Snippet",
		FileName:  "test.go",
	})
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("expected content required error, got %v", err)
	}
}

// TestProjectCreate_MissingFileName verifies that project create validates the
// single-file file name before calling GitLab.
func TestProjectCreate_MissingFileName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   toolutil.StringOrInt("10"),
		Title:       "Test Snippet",
		ContentBody: "package main",
	})
	if err == nil || !strings.Contains(err.Error(), "file_name is required") {
		t.Fatalf("expected file name required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ProjectUpdate
// ---------------------------------------------------------------------------.

// TestProjectUpdate_Success verifies ProjectUpdate when success.
func TestProjectUpdate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathSnippet42, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, snippetJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{
		ProjectID: toolutil.StringOrInt("10"),
		SnippetID: 42,
		Title:     "Updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf(fmtExpID42, out.ID)
	}
}

// TestProjectUpdate_MissingParams verifies ProjectUpdate when missing params.
func TestProjectUpdate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}

	_, err = ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), errSnippetIDRequired) {
		t.Fatalf(fmtExpSnippetIDReqErr, err)
	}
}

// ---------------------------------------------------------------------------
// ProjectDelete
// ---------------------------------------------------------------------------.

// TestProjectDelete_Success verifies ProjectDelete when success.
func TestProjectDelete_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathSnippet42, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := ProjectDelete(context.Background(), client, ProjectDeleteInput{
		ProjectID: toolutil.StringOrInt("10"), SnippetID: 42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestProjectDelete_MissingParams verifies ProjectDelete when missing params.
func TestProjectDelete_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := ProjectDelete(context.Background(), client, ProjectDeleteInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjIDReqErr, err)
	}

	err = ProjectDelete(context.Background(), client, ProjectDeleteInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "snippet_id is required") {
		t.Fatalf("expected snippet_id required error, got %v", err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// covSnippetJSON identifies the cov snippet JSON constant used by this package.
const covSnippetJSON = `{"id":1,"title":"Hello","file_name":"hello.rb","description":"test","visibility":"private","author":{"id":10,"username":"user","name":"User","email":"u@e.com","state":"active"},"web_url":"https://x","raw_url":"https://r","files":[{"path":"hello.rb","raw_url":"https://f"}],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`

// covListSnippetJSON identifies the cov list snippet JSON constant used by this package.
const covListSnippetJSON = `[` + covSnippetJSON + `]`

const (
	// errExpected identifies the err expected constant used by this package.
	errExpected = "expected error"
	// fmtUnexpectedErr identifies the fmt unexpected err constant used by this package.
	fmtUnexpectedErr = "unexpected error: %v"
	// fmtIDEquals identifies the fmt ID equals constant used by this package.
	fmtIDEquals = "ID = %d"
	// testWebURL identifies the test web URL constant used by this package.
	testWebURL = "https://x"
	// labelProjectID identifies the label project ID constant used by this package.
	labelProjectID = "Project ID"
)

// snippetFixtureNoFiles stores the package-level snippet fixture no files state.
var snippetFixtureNoFiles = gl.Snippet{
	ID:         1,
	Title:      "Test",
	Visibility: "private",
	Author:     gl.SnippetAuthor{ID: 10, Username: "u", Name: "U"},
}

// ---------------------------------------------------------------------------
// API error paths
// ---------------------------------------------------------------------------.

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestListAll_APIError verifies ListAll when API error.
func TestListAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ListAll(context.Background(), client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestListAll_WithDateFilters verifies ListAll when with date filters.
func TestListAll_WithDateFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covListSnippetJSON)
	}))
	out, err := ListAll(context.Background(), client, ListAllInput{
		CreatedAfter:  "2026-01-01T00:00:00Z",
		CreatedBefore: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("got %d snippets", len(out.Snippets))
	}
}

// TestListAll_InvalidDateFilters verifies ListAll when invalid date filters.
func TestListAll_InvalidDateFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListAll(context.Background(), client, ListAllInput{
		CreatedAfter:  "bad-date",
		CreatedBefore: "bad-date",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Get(context.Background(), client, GetInput{SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestContent_APIError verifies Content when API error.
func TestContent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Content(context.Background(), client, ContentInput{SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestFileContent_APIError verifies FileContent when API error.
func TestFileContent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := FileContent(context.Background(), client, FileContentInput{SnippetID: 1, Ref: "main", FileName: "f"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Create(context.Background(), client, CreateInput{Title: "t", FileName: "f.txt", ContentBody: "hello"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Update(context.Background(), client, UpdateInput{SnippetID: 1, Title: "t"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := Delete(context.Background(), client, DeleteInput{SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestExplore_APIError verifies Explore when API error.
func TestExplore_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Explore(context.Background(), client, ExploreInput{})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// Create/Update with full options
// ---------------------------------------------------------------------------.

// TestCreate_WithAllOptions verifies Create when with all options.
func TestCreate_WithAllOptions(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusCreated, covSnippetJSON)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		Title:       "Test",
		FileName:    "test.rb",
		Description: "desc",
		ContentBody: "puts 'hi'",
		Visibility:  "private",
		Files:       []CreateFileInput{{FilePath: "test.rb", Content: "puts 'hi'"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDEquals, out.ID)
	}
	for _, want := range []string{"title", "description", "visibility", "files"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestUpdate_WithAllOptions verifies Update when with all options.
func TestUpdate_WithAllOptions(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, covSnippetJSON)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		SnippetID:   1,
		Title:       "New",
		FileName:    "new.rb",
		Description: "new desc",
		ContentBody: "puts 'new'",
		Visibility:  "public",
		Files:       []UpdateFileInput{{Action: "update", FilePath: "new.rb", Content: "puts 'x'", PreviousPath: "old.rb"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDEquals, out.ID)
	}
	for _, want := range []string{"title", "description", "visibility", "files"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Project snippet errors
// ---------------------------------------------------------------------------.

// TestProjectList_APIError verifies ProjectList when API error.
func TestProjectList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestProjectGet_APIError verifies ProjectGet when API error.
func TestProjectGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "42", SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestProjectContent_APIError verifies ProjectContent when API error.
func TestProjectContent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ProjectContent(context.Background(), client, ProjectContentInput{ProjectID: "42", SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestProjectCreate_AllOptions verifies ProjectCreate when all options.
func TestProjectCreate_AllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covSnippetJSON)
	}))
	out, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   "42",
		Title:       "Test",
		Description: "desc",
		Visibility:  "internal",
		FileName:    "a.rb",
		ContentBody: "x",
		Files:       []CreateFileInput{{FilePath: "a.rb", Content: "x"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDEquals, out.ID)
	}
}

// TestProjectCreate_APIError verifies ProjectCreate when API error.
func TestProjectCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ProjectCreate(context.Background(), client, ProjectCreateInput{ProjectID: "42", Title: "t", FileName: "f.txt", ContentBody: "hello"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestProjectUpdate_AllOptions verifies ProjectUpdate when all options.
func TestProjectUpdate_AllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSnippetJSON)
	}))
	out, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{
		ProjectID:   "42",
		SnippetID:   1,
		Title:       "New",
		Description: "d",
		Visibility:  "public",
		FileName:    "b.rb",
		ContentBody: "y",
		Files:       []UpdateFileInput{{Action: "update", FilePath: "b.rb", Content: "y", PreviousPath: "a.rb"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDEquals, out.ID)
	}
}

// TestProjectUpdate_APIError verifies ProjectUpdate when API error.
func TestProjectUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: "42", SnippetID: 1, Title: "t"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestProjectDelete_APIError verifies ProjectDelete when API error.
func TestProjectDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := ProjectDelete(context.Background(), client, ProjectDeleteInput{ProjectID: "42", SnippetID: 1})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// Formatter coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies FormatMarkdown when all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	s := FormatMarkdown(Output{
		ID: 1, Title: "T", FileName: "f.rb", Description: "desc",
		Visibility: "private", ProjectID: 42, WebURL: testWebURL,
		Author: AuthorOutput{Name: "User", Username: "user"},
		Files:  []FileOutput{{Path: "f.rb", RawURL: "https://r"}},
	})
	if !strings.Contains(s, "File Name") {
		t.Error("expected File Name")
	}
	if !strings.Contains(s, "Description") {
		t.Error("expected Description")
	}
	if !strings.Contains(s, labelProjectID) {
		t.Error("expected Project ID")
	}
	if !strings.Contains(s, "Files") {
		t.Error("expected Files section")
	}
}

// TestFormatMarkdown_Minimal verifies FormatMarkdown when minimal.
func TestFormatMarkdown_Minimal(t *testing.T) {
	s := FormatMarkdown(Output{ID: 1, Title: "T", Visibility: "private", Author: AuthorOutput{Name: "U", Username: "u"}})
	if strings.Contains(s, "File Name") {
		t.Error("should not include File Name when empty")
	}
	if strings.Contains(s, "Description") {
		t.Error("should not include Description when empty")
	}
	if strings.Contains(s, labelProjectID) {
		t.Error("should not include Project ID when 0")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	s := FormatListMarkdown(ListOutput{})
	if !strings.Contains(s, "No snippets found") {
		t.Error("expected empty message")
	}
}

// ---------------------------------------------------------------------------
// extractProjectPath
// ---------------------------------------------------------------------------.

// TestExtractProjectPath covers ExtractProjectPath with table-driven subtests.
func TestExtractProjectPath(t *testing.T) {
	tests := []struct {
		name   string
		webURL string
		want   string
	}{
		{"project snippet", "https://gitlab.example.com/my-group/my-project/-/snippets/42", "my-group/my-project"},
		{"nested group", "https://gitlab.example.com/org/team/repo/-/snippets/1", "org/team/repo"},
		{"personal snippet dash prefix", "https://gitlab.example.com/-/snippets/42", ""},
		{"personal snippet no dash", "https://gitlab.example.com/snippets/42", ""},
		{"short URL", testWebURL, ""},
		{"empty string", "", ""},
		{"no scheme returns empty", "gitlab.example.com/group/project/-/snippets/1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractProjectPath(tt.webURL)
			if got != tt.want {
				t.Errorf("extractProjectPath(%q) = %q, want %q", tt.webURL, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown with project path
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_WithProjectPath verifies FormatMarkdown when with project path.
func TestFormatMarkdown_WithProjectPath(t *testing.T) {
	s := FormatMarkdown(Output{
		ID: 5, Title: "Project Snippet", Visibility: "internal",
		ProjectID: 42,
		WebURL:    "https://gitlab.example.com/my-group/my-project/-/snippets/5",
		Author:    AuthorOutput{Name: "Dev", Username: "dev"},
	})
	if !strings.Contains(s, "| Project | my-group/my-project |") {
		t.Errorf("expected project path row, got:\n%s", s)
	}
	if strings.Contains(s, labelProjectID) {
		t.Error("should not show numeric Project ID when path is extractable")
	}
}

// TestFormatMarkdown_FallbackProjectID verifies FormatMarkdown when fallback project ID.
func TestFormatMarkdown_FallbackProjectID(t *testing.T) {
	s := FormatMarkdown(Output{
		ID: 5, Title: "Snippet", Visibility: "private",
		ProjectID: 99, WebURL: testWebURL,
		Author: AuthorOutput{Name: "U", Username: "u"},
	})
	if !strings.Contains(s, "| Project ID | 99 |") {
		t.Errorf("expected numeric Project ID fallback, got:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown with project column
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithProjectColumn verifies FormatListMarkdown when with project column.
func TestFormatListMarkdown_WithProjectColumn(t *testing.T) {
	out := ListOutput{
		Snippets: []Output{
			{
				ID: 1, Title: "PS1", Visibility: "public",
				ProjectID: 10,
				WebURL:    "https://gitlab.example.com/team/app/-/snippets/1",
				Author:    AuthorOutput{Username: "u1"},
			},
			{
				ID: 2, Title: "PS2", Visibility: "private",
				ProjectID: 20,
				WebURL:    "https://short-url",
				Author:    AuthorOutput{Username: "u2"},
			},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "| Project |") {
		t.Errorf("expected Project column header, got:\n%s", md)
	}
	if !strings.Contains(md, "team/app") {
		t.Errorf("expected project path in row, got:\n%s", md)
	}
	if !strings.Contains(md, "| 20 |") {
		t.Errorf("expected numeric project ID fallback for short URL, got:\n%s", md)
	}
}

// TestFormatListMarkdown_NoProjectColumn verifies FormatListMarkdown when no project column.
func TestFormatListMarkdown_NoProjectColumn(t *testing.T) {
	out := ListOutput{
		Snippets: []Output{
			{ID: 1, Title: "Personal", Visibility: "private", Author: AuthorOutput{Username: "u1"}},
		},
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "| Project") {
		t.Errorf("should not include Project column for personal snippets, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// convertSnippet with nil files
// ---------------------------------------------------------------------------.

// TestConvertSnippet_NilFiles verifies ConvertSnippet when nil files.
func TestConvertSnippet_NilFiles(t *testing.T) {
	s := convertSnippet(&snippetFixtureNoFiles)
	if len(s.Files) != 0 {
		t.Errorf("expected no files, got %d", len(s.Files))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for snippet actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := snippetSpecsByTool(t, specs)

	if len(specs) != 15 {
		t.Fatalf("len(ActionSpecs) = %d, want 15", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "snippets" {
			t.Fatalf("OwnerPackage for %s = %q, want snippets", spec.Name, spec.OwnerPackage)
		}
	}
}

// TestActionSpecs_CallAllRoutes validates snippet routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, covSnippetJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covSnippetJSON)
		case strings.Contains(r.URL.Path, "/raw"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("raw content"))
		case strings.Contains(r.URL.Path, "/files/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("file content"))
		case strings.HasSuffix(r.URL.Path, "/snippets/1"):
			testutil.RespondJSON(w, http.StatusOK, covSnippetJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, covListSnippetJSON)
		}
	}))
	byTool := snippetSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_snippet_list", map[string]any{}},
		{"gitlab_snippet_list_all", map[string]any{}},
		{"gitlab_snippet_get", map[string]any{"snippet_id": float64(1)}},
		{"gitlab_snippet_content", map[string]any{"snippet_id": float64(1)}},
		{"gitlab_snippet_file_content", map[string]any{"snippet_id": float64(1), "ref": "main", "file_name": "f"}},
		{"gitlab_snippet_create", map[string]any{"title": "t", "file_name": "f.txt", "content": "hello"}},
		{"gitlab_snippet_update", map[string]any{"snippet_id": float64(1), "title": "t"}},
		{"gitlab_snippet_delete", map[string]any{"snippet_id": float64(1)}},
		{"gitlab_snippet_explore", map[string]any{}},
		{"gitlab_project_snippet_list", map[string]any{"project_id": "42"}},
		{"gitlab_project_snippet_get", map[string]any{"project_id": "42", "snippet_id": float64(1)}},
		{"gitlab_project_snippet_content", map[string]any{"project_id": "42", "snippet_id": float64(1)}},
		{"gitlab_project_snippet_create", map[string]any{"project_id": "42", "title": "t", "file_name": "f.txt", "content": "hello"}},
		{"gitlab_project_snippet_update", map[string]any{"project_id": "42", "snippet_id": float64(1), "title": "t"}},
		{"gitlab_project_snippet_delete", map[string]any{"project_id": "42", "snippet_id": float64(1)}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestProjectCreate_SingleFileFallback validates the single-file fallback path
// in ProjectCreate when len(input.Files)==0 but FileName/ContentBody are set.
func TestProjectCreate_SingleFileFallback(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, snippetJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   "42",
		Title:       "test",
		FileName:    "main.go",
		ContentBody: "package main",
		Visibility:  "public",
	})
	if err != nil {
		t.Fatalf("ProjectCreate: %v", err)
	}
	if out.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

// TestProjectUpdate_SingleFileFallback validates the single-file fallback path
// in ProjectUpdate when len(input.Files)==0 but FileName/ContentBody are set.
func TestProjectUpdate_SingleFileFallback(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, snippetJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{
		ProjectID:   "42",
		SnippetID:   1,
		FileName:    "main.go",
		ContentBody: "package main\nfunc main() {}",
	})
	if err != nil {
		t.Fatalf("ProjectUpdate: %v", err)
	}
	if out.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

// TestActionSpecs_ProjectSnippetGetRoute verifies the canonical project snippet get route output.
func TestActionSpecs_ProjectSnippetGetRoute(t *testing.T) {
	const respJSON = `{"id":7,"title":"hi","file_name":"f.txt","description":"","visibility":"private","author":{"id":1,"username":"u","name":"u"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/snippets/7") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := snippetSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_project_snippet_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "snippet_id": 7})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 7 || out.Title != "hi" {
		t.Fatalf("project snippet output = %#v, want ID 7 title hi", out)
	}
}

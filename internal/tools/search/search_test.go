// search_test.go contains unit tests for GitLab search operations
// (code search and merge request search). Tests use httptest to mock
// the GitLab Search API and verify both success and error paths.
package search

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpCancelledNil = "expected error for canceled context, got nil"
	errExpEmptyQuery   = "expected error for empty query, got nil"
	errExpAPIFailure   = "expected error for API failure, got nil"
	fmtLenBlobsWant1   = "len(Blobs) = %d, want 1"
	pathSearchProject  = "/api/v4/projects/42/-/search"
	pathSearchGroup    = "/api/v4/groups/7/-/search"
	pathSearchGlobal   = "/api/v4/search"
	queryScope         = "scope"
	scopeBlobs         = "blobs"
	testMRFixBugTitle  = "Fix bug"
	testProjectSlug    = "my-project"
	fmtTitleWant       = "Title = %q, want %q"
)

var defaultPagination = testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}

// TestSearchCode_ProjectScope verifies the behavior of search code project scope.
func TestSearchCode_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject {
			testutil.AssertQueryParam(t, r, queryScope, scopeBlobs)
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"main","data":"func main()","path":"cmd/main.go",
				"filename":"main.go","ref":"main","startline":1,"project_id":42
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "func main"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
	if out.Blobs[0].Filename != "main.go" {
		t.Errorf("Filename = %q, want %q", out.Blobs[0].Filename, "main.go")
	}
}

// TestSearchCode_GlobalScope verifies the behavior of search code global scope.
func TestSearchCode_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == scopeBlobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"main","data":"func main()","path":"main.go",
				"filename":"main.go","ref":"main","startline":1,"project_id":1
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{Query: "func main"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// TestSearchCode_GroupScope verifies the behavior of search code group scope.
func TestSearchCode_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGroup && r.URL.Query().Get(queryScope) == scopeBlobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"util","data":"func helper()","path":"util.go",
				"filename":"util.go","ref":"main","startline":5,"project_id":99
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{GroupID: "7", Query: "helper"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// TestSearchCode_EmptyQuery verifies the behavior of search code empty query.
func TestSearchCode_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Code(context.Background(), client, CodeInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCode_APIError verifies the behavior of search code a p i error.
func TestSearchCode_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "test"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestSearchMerge_RequestsProjectScope verifies the behavior of search merge requests project scope.
func TestSearchMerge_RequestsProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":1,"iid":10,"title":"Fix bug","state":"merged",
				"source_branch":"fix/bug","target_branch":"main",
				"web_url":"https://gitlab.example.com/-/merge_requests/10",
				"author":{"username":"dev1"}
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MergeRequests(context.Background(), client, MergeRequestsInput{ProjectID: "42", Query: testMRFixBugTitle})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != testMRFixBugTitle {
		t.Errorf(fmtTitleWant, out.MergeRequests[0].Title, testMRFixBugTitle)
	}
}

// TestSearchMerge_RequestsGlobalScope verifies the behavior of search merge requests global scope.
func TestSearchMerge_RequestsGlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":2,"iid":20,"title":"Feature","state":"opened",
				"source_branch":"feat","target_branch":"main",
				"web_url":"https://gitlab.example.com/-/merge_requests/20",
				"author":{"username":"dev2"}
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MergeRequests(context.Background(), client, MergeRequestsInput{Query: "Feature"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
}

// TestSearchMergeRequests_EmptyQuery verifies the behavior of search merge requests empty query.
func TestSearchMergeRequests_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := MergeRequests(context.Background(), client, MergeRequestsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCode_CancelledContext verifies the behavior of search code cancelled context.
func TestSearchCode_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Code(ctx, client, CodeInput{ProjectID: "42", Query: "test"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSearchIssuesGlobal_Success verifies the behavior of search issues global success.
func TestSearchIssuesGlobal_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":101,"iid":5,"title":"Fix critical bug","state":"opened",
				"labels":["bug"],"web_url":"https://gitlab.example.com/project/-/issues/5",
				"author":{"username":"dev1"},"project_id":42
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Issues(context.Background(), client, IssuesInput{Query: "bug"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
	}
	if out.Issues[0].Title != "Fix critical bug" {
		t.Errorf(fmtTitleWant, out.Issues[0].Title, "Fix critical bug")
	}
}

// TestSearchIssuesByProject_Success verifies the behavior of search issues by project success.
func TestSearchIssuesByProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":101,"iid":5,"title":"Fix critical bug","state":"opened",
				"labels":["bug"],"web_url":"https://gitlab.example.com/project/-/issues/5",
				"author":{"username":"dev1"},"project_id":42
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Issues(context.Background(), client, IssuesInput{ProjectID: "42", Query: "bug"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
	}
}

// TestSearchIssues_EmptyQuery verifies the behavior of search issues empty query.
func TestSearchIssues_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Issues(context.Background(), client, IssuesInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchIssues_APIError verifies the behavior of search issues a p i error.
func TestSearchIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Issues(context.Background(), client, IssuesInput{Query: "bug"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestSearchIssues_CancelledContext verifies the behavior of search issues cancelled context.
func TestSearchIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Issues(ctx, client, IssuesInput{Query: "bug"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Commits
// ---------------------------------------------------------------------------.

// TestSearchCommits_GlobalScope verifies the behavior of search commits global scope.
func TestSearchCommits_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "commits" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":"abc123","short_id":"abc","title":"Initial commit",
				"author_name":"Dev","author_email":"dev@example.com",
				"committer_name":"Dev","committer_email":"dev@example.com",
				"web_url":"https://gitlab.example.com/commit/abc123"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Commits(context.Background(), client, CommitsInput{Query: "Initial"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	if out.Commits[0].Title != "Initial commit" {
		t.Errorf(fmtTitleWant, out.Commits[0].Title, "Initial commit")
	}
}

// TestSearchCommits_ProjectScope verifies the behavior of search commits project scope.
func TestSearchCommits_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "commits" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":"def456","short_id":"def","title":"Fix it",
				"author_name":"Dev","author_email":"dev@example.com",
				"committer_name":"Dev","committer_email":"dev@example.com",
				"web_url":"https://gitlab.example.com/commit/def456"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Commits(context.Background(), client, CommitsInput{ProjectID: "42", Query: "Fix"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
}

// TestSearchCommits_EmptyQuery verifies the behavior of search commits empty query.
func TestSearchCommits_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Commits(context.Background(), client, CommitsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCommits_CancelledContext verifies the behavior of search commits cancelled context.
func TestSearchCommits_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Commits(ctx, client, CommitsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Milestones
// ---------------------------------------------------------------------------.

// TestSearchMilestones_GlobalScope verifies the behavior of search milestones global scope.
func TestSearchMilestones_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "milestones" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":10,"iid":1,"title":"v1.0","state":"active",
				"web_url":"https://gitlab.example.com/-/milestones/1"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Milestones(context.Background(), client, MilestonesInput{Query: "v1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
	if out.Milestones[0].Title != "v1.0" {
		t.Errorf(fmtTitleWant, out.Milestones[0].Title, "v1.0")
	}
}

// TestSearchMilestones_ProjectScope verifies the behavior of search milestones project scope.
func TestSearchMilestones_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "milestones" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":11,"iid":2,"title":"v2.0","state":"active",
				"web_url":"https://gitlab.example.com/-/milestones/2","project_id":42
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Milestones(context.Background(), client, MilestonesInput{ProjectID: "42", Query: "v2"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestSearchMilestones_EmptyQuery verifies the behavior of search milestones empty query.
func TestSearchMilestones_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Milestones(context.Background(), client, MilestonesInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// ---------------------------------------------------------------------------
// Notes (project-scoped only)
// ---------------------------------------------------------------------------.

// TestSearchNotes_ProjectScope verifies the behavior of search notes project scope.
func TestSearchNotes_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "notes" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":55,"body":"Looks good to me","author":{"username":"reviewer"},
				"noteable_type":"Issue","noteable_id":101,"noteable_iid":5,
				"system":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Notes(context.Background(), client, NotesInput{ProjectID: "42", Query: "good"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Notes) != 1 {
		t.Fatalf("len(Notes) = %d, want 1", len(out.Notes))
	}
	if out.Notes[0].Author != "reviewer" {
		t.Errorf("Author = %q, want %q", out.Notes[0].Author, "reviewer")
	}
}

// TestSearchNotes_MissingProjectID verifies the behavior of search notes missing project i d.
func TestSearchNotes_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Notes(context.Background(), client, NotesInput{Query: "test"})
	if err == nil {
		t.Fatal("expected error for missing project_id, got nil")
	}
}

// TestSearchNotes_EmptyQuery verifies the behavior of search notes empty query.
func TestSearchNotes_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Notes(context.Background(), client, NotesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------.

// TestSearchProjects_GlobalScope verifies the behavior of search projects global scope.
func TestSearchProjects_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "projects" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":42,"name":"my-project","path":"my-project",
				"path_with_namespace":"user/my-project","visibility":"private",
				"default_branch":"main","web_url":"https://gitlab.example.com/user/my-project"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Projects(context.Background(), client, ProjectsInput{Query: testProjectSlug})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(out.Projects))
	}
	if out.Projects[0].Name != "my-project" {
		t.Errorf("Name = %q, want %q", out.Projects[0].Name, testProjectSlug)
	}
}

// TestSearchProjects_GroupScope verifies the behavior of search projects group scope.
func TestSearchProjects_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGroup && r.URL.Query().Get(queryScope) == "projects" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":99,"name":"group-proj","path":"group-proj",
				"path_with_namespace":"g/group-proj","visibility":"internal",
				"default_branch":"main","web_url":"https://gitlab.example.com/g/group-proj"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Projects(context.Background(), client, ProjectsInput{GroupID: "7", Query: "group"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(out.Projects))
	}
}

// TestSearchProjects_EmptyQuery verifies the behavior of search projects empty query.
func TestSearchProjects_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Projects(context.Background(), client, ProjectsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// ---------------------------------------------------------------------------
// Snippets (global only)
// ---------------------------------------------------------------------------.

// TestSearchSnippets_GlobalScope verifies the behavior of search snippets global scope.
func TestSearchSnippets_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "snippet_titles" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":301,"title":"My snippet","file_name":"notes.md",
				"description":"A note","visibility":"private",
				"author":{"username":"dev1"},
				"web_url":"https://gitlab.example.com/-/snippets/301",
				"raw_url":"https://gitlab.example.com/-/snippets/301/raw",
				"created_at":"2026-06-01T12:00:00Z","updated_at":"2026-06-01T12:00:00Z"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Snippets(context.Background(), client, SnippetsInput{Query: "snippet"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("len(Snippets) = %d, want 1", len(out.Snippets))
	}
	if out.Snippets[0].Title != "My snippet" {
		t.Errorf(fmtTitleWant, out.Snippets[0].Title, "My snippet")
	}
}

// TestSearchSnippets_EmptyQuery verifies the behavior of search snippets empty query.
func TestSearchSnippets_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Snippets(context.Background(), client, SnippetsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------.

// TestSearchUsers_GlobalScope verifies the behavior of search users global scope.
func TestSearchUsers_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "users" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":1,"username":"admin","name":"Admin User","state":"active",
				"avatar_url":"https://gitlab.example.com/avatar","web_url":"https://gitlab.example.com/admin"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Users(context.Background(), client, UsersInput{Query: "admin"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
	if out.Users[0].Username != "admin" {
		t.Errorf("Username = %q, want %q", out.Users[0].Username, "admin")
	}
}

// TestSearchUsers_ProjectScope verifies the behavior of search users project scope.
func TestSearchUsers_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "users" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":2,"username":"dev1","name":"Developer","state":"active",
				"avatar_url":"","web_url":"https://gitlab.example.com/dev1"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Users(context.Background(), client, UsersInput{ProjectID: "42", Query: "dev"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
}

// TestSearchUsers_EmptyQuery verifies the behavior of search users empty query.
func TestSearchUsers_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Users(context.Background(), client, UsersInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// ---------------------------------------------------------------------------
// Wiki Blobs
// ---------------------------------------------------------------------------.

// TestSearchWiki_GlobalScope verifies the behavior of search wiki global scope.
func TestSearchWiki_GlobalScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "wiki_blobs" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"slug":"home","title":"Home","content":"Welcome to the wiki","format":"markdown"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wiki(context.Background(), client, WikiInput{Query: "wiki"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WikiBlobs) != 1 {
		t.Fatalf("len(WikiBlobs) = %d, want 1", len(out.WikiBlobs))
	}
	if out.WikiBlobs[0].Title != "Home" {
		t.Errorf(fmtTitleWant, out.WikiBlobs[0].Title, "Home")
	}
}

// TestSearchWiki_ProjectScope verifies the behavior of search wiki project scope.
func TestSearchWiki_ProjectScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == "wiki_blobs" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"slug":"setup","title":"Setup","content":"How to set up","format":"markdown"
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wiki(context.Background(), client, WikiInput{ProjectID: "42", Query: "setup"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WikiBlobs) != 1 {
		t.Fatalf("len(WikiBlobs) = %d, want 1", len(out.WikiBlobs))
	}
}

// TestSearchWiki_EmptyQuery verifies the behavior of search wiki empty query.
func TestSearchWiki_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Wiki(context.Background(), client, WikiInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchWiki_CancelledContext verifies the behavior of search wiki cancelled context.
func TestSearchWiki_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Wiki(ctx, client, WikiInput{Query: "test"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	errExpected    = "expected error"
	fmtUnexpErr    = "unexpected error: %v"
	errExpectedHdr = "expected header with count"
	fmtLenWant1    = "len=%d, want 1"
)

// ---------------------------------------------------------------------------
// searchOpts helper
// ---------------------------------------------------------------------------.

// TestSearchOpts_Defaults verifies the behavior of search opts defaults.
func TestSearchOpts_Defaults(t *testing.T) {
	opts := searchOpts(0, 0, "")
	if opts.Ref != nil {
		t.Errorf("expected nil Ref")
	}
	if opts.Page != 0 {
		t.Errorf("expected Page 0, got %d", opts.Page)
	}
}

// TestSearchOpts_AllParams verifies the behavior of search opts all params.
func TestSearchOpts_AllParams(t *testing.T) {
	opts := searchOpts(3, 50, "develop")
	if opts.Ref == nil || *opts.Ref != "develop" {
		t.Error("expected Ref=develop")
	}
	if opts.Page != 3 {
		t.Errorf("expected Page=3, got %d", opts.Page)
	}
	if opts.PerPage != 50 {
		t.Errorf("expected PerPage=50, got %d", opts.PerPage)
	}
}

// ---------------------------------------------------------------------------
// Group-scope tests (missing from search_test.go)
// ---------------------------------------------------------------------------.

// TestSearchMerge_RequestsGroupScope verifies the behavior of search merge requests group scope.
func TestSearchMerge_RequestsGroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":5,"iid":15,"title":"Group MR","state":"opened","source_branch":"f","target_branch":"main","author":{"username":"u"}}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := MergeRequests(context.Background(), client, MergeRequestsInput{GroupID: "7", Query: "Group"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf(fmtLenWant1, len(out.MergeRequests))
	}
}

// TestSearchIssues_GroupScope verifies the behavior of search issues group scope.
func TestSearchIssues_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"iid":1,"title":"Group Issue","state":"opened","author":{"username":"u"}}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Issues(context.Background(), client, IssuesInput{GroupID: "7", Query: "grp"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtLenWant1, len(out.Issues))
	}
}

// TestSearchCommits_GroupScope verifies the behavior of search commits group scope.
func TestSearchCommits_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "commits" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":"aaa","short_id":"aaa","title":"Grp commit","author_name":"A","author_email":"a@a.com","committer_name":"A","committer_email":"a@a.com"}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Commits(context.Background(), client, CommitsInput{GroupID: "7", Query: "grp"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf(fmtLenWant1, len(out.Commits))
	}
}

// TestSearchMilestones_GroupScope verifies the behavior of search milestones group scope.
func TestSearchMilestones_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "milestones" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":20,"iid":2,"title":"v3.0","state":"active"}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Milestones(context.Background(), client, MilestonesInput{GroupID: "7", Query: "v3"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf(fmtLenWant1, len(out.Milestones))
	}
}

// TestSearchUsers_GroupScope verifies the behavior of search users group scope.
func TestSearchUsers_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "users" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":3,"username":"grpuser","name":"G","state":"active","avatar_url":"","web_url":"https://x/grpuser"}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Users(context.Background(), client, UsersInput{GroupID: "7", Query: "grp"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf(fmtLenWant1, len(out.Users))
	}
}

// TestSearchWiki_GroupScope verifies the behavior of search wiki group scope.
func TestSearchWiki_GroupScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGroup && r.URL.Query().Get("scope") == "wiki_blobs" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"slug":"home","title":"Home","content":"wiki","format":"markdown"}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Wiki(context.Background(), client, WikiInput{GroupID: "7", Query: "wiki"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WikiBlobs) != 1 {
		t.Fatalf(fmtLenWant1, len(out.WikiBlobs))
	}
}

// ---------------------------------------------------------------------------
// API error tests
// ---------------------------------------------------------------------------.

// TestSearchMergeRequests_APIError verifies the behavior of search merge requests a p i error.
func TestSearchMergeRequests_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := MergeRequests(context.Background(), client, MergeRequestsInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchCommits_APIError verifies the behavior of search commits a p i error.
func TestSearchCommits_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Commits(context.Background(), client, CommitsInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchMilestones_APIError verifies the behavior of search milestones a p i error.
func TestSearchMilestones_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Milestones(context.Background(), client, MilestonesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchNotes_APIError verifies the behavior of search notes a p i error.
func TestSearchNotes_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Notes(context.Background(), client, NotesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchProjects_APIError verifies the behavior of search projects a p i error.
func TestSearchProjects_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Projects(context.Background(), client, ProjectsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchSnippets_APIError verifies the behavior of search snippets a p i error.
func TestSearchSnippets_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Snippets(context.Background(), client, SnippetsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchUsers_APIError verifies the behavior of search users a p i error.
func TestSearchUsers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Users(context.Background(), client, UsersInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchWiki_APIError verifies the behavior of search wiki a p i error.
func TestSearchWiki_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Wiki(context.Background(), client, WikiInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests
// ---------------------------------------------------------------------------.

// TestSearchMergeRequests_CancelledCtx verifies the behavior of search merge requests cancelled ctx.
func TestSearchMergeRequests_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := MergeRequests(ctx, client, MergeRequestsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchMilestones_CancelledCtx verifies the behavior of search milestones cancelled ctx.
func TestSearchMilestones_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Milestones(ctx, client, MilestonesInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchNotes_CancelledCtx verifies the behavior of search notes cancelled ctx.
func TestSearchNotes_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Notes(ctx, client, NotesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchProjects_CancelledCtx verifies the behavior of search projects cancelled ctx.
func TestSearchProjects_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Projects(ctx, client, ProjectsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchSnippets_CancelledCtx verifies the behavior of search snippets cancelled ctx.
func TestSearchSnippets_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Snippets(ctx, client, SnippetsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchUsers_CancelledCtx verifies the behavior of search users cancelled ctx.
func TestSearchUsers_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Users(ctx, client, UsersInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// Code search with Ref parameter
// ---------------------------------------------------------------------------.

// TestSearchCode_WithRef verifies the behavior of search code with ref.
func TestSearchCode_WithRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "ref", "develop")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"basename":"f","data":"d","path":"f.go","filename":"f.go","ref":"develop","startline":1,"project_id":42}]`, defaultPagination)
	}))
	out, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "test", Ref: "develop"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Blobs[0].Ref != "develop" {
		t.Errorf("Ref=%q, want develop", out.Blobs[0].Ref)
	}
}

// ---------------------------------------------------------------------------
// Notes with nil fields
// ---------------------------------------------------------------------------.

// TestSearchNotes_NilAuthorAndDates verifies the behavior of search notes nil author and dates.
func TestSearchNotes_NilAuthorAndDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchProject && r.URL.Query().Get("scope") == "notes" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"body":"note","noteable_type":"Issue","noteable_id":10,"system":false}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Notes(context.Background(), client, NotesInput{ProjectID: "42", Query: "note"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Notes[0].Author != "" {
		t.Errorf("expected empty Author, got %q", out.Notes[0].Author)
	}
	if out.Notes[0].CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.Notes[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Snippets with nil fields and ProjectID
// ---------------------------------------------------------------------------.

// TestSearchSnippets_NilAuthorAndDates verifies the behavior of search snippets nil author and dates.
func TestSearchSnippets_NilAuthorAndDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get("scope") == "snippet_titles" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"title":"S","file_name":"f.md","description":"d","visibility":"private","web_url":"u","raw_url":"r","project_id":99}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Snippets(context.Background(), client, SnippetsInput{Query: "S"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Snippets[0].ProjectID != 99 {
		t.Errorf("ProjectID=%d, want 99", out.Snippets[0].ProjectID)
	}
	if out.Snippets[0].Author != "" {
		t.Errorf("expected empty Author")
	}
}

// ---------------------------------------------------------------------------
// Markdown formatter tests
// ---------------------------------------------------------------------------.

// TestFormatCodeMarkdown_Empty verifies the behavior of format code markdown empty.
func TestFormatCodeMarkdown_Empty(t *testing.T) {
	s := FormatCodeMarkdown(CodeOutput{})
	if !strings.Contains(s, "No code search results found") {
		t.Errorf("expected 'No code search results found', got %q", s)
	}
}

// TestFormatCodeMarkdown_WithResults verifies the behavior of format code markdown with results.
func TestFormatCodeMarkdown_WithResults(t *testing.T) {
	s := FormatCodeMarkdown(CodeOutput{
		Blobs:      []BlobOutput{{Filename: "main.go", Path: "cmd/main.go", Ref: "main", Startline: 10}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(s, "Code Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// TestFormatMRsMarkdown_Empty verifies the behavior of format m rs markdown empty.
func TestFormatMRsMarkdown_Empty(t *testing.T) {
	s := FormatMRsMarkdown(MergeRequestsOutput{})
	if !strings.Contains(s, "No merge requests found") {
		t.Errorf("expected 'No merge requests found', got %q", s)
	}
}

// TestFormatMRsMarkdown_WithResults verifies the behavior of format m rs markdown with results.
func TestFormatMRsMarkdown_WithResults(t *testing.T) {
	s := FormatMRsMarkdown(MergeRequestsOutput{
		MergeRequests: []mergerequests.Output{{IID: 5, Title: "Fix", State: "merged", SourceBranch: "fix", TargetBranch: "main"}},
		Pagination:    toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "!5") {
		t.Error("expected !5 in output")
	}
	if !strings.Contains(s, "MR Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// markdownForResult dispatch
// ---------------------------------------------------------------------------.

// TestMarkdownForResult_CodeOutput verifies the behavior of markdown for result code output.
func TestMarkdownForResult_CodeOutput(t *testing.T) {
	result := markdownForResult(CodeOutput{})
	if result == nil {
		t.Error("expected non-nil result for CodeOutput")
	}
}

// TestMarkdownForResult_MROutput verifies the behavior of markdown for result m r output.
func TestMarkdownForResult_MROutput(t *testing.T) {
	result := markdownForResult(MergeRequestsOutput{})
	if result == nil {
		t.Error("expected non-nil result for MergeRequestsOutput")
	}
}

// TestMarkdownForResult_Unknown verifies the behavior of markdown for result unknown.
func TestMarkdownForResult_Unknown(t *testing.T) {
	result := markdownForResult("unknown")
	if result != nil {
		t.Error("expected nil for unknown type")
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Issues
// ---------------------------------------------------------------------------.

// TestFormatIssuesMarkdown_Empty verifies the behavior of format issues markdown empty.
func TestFormatIssuesMarkdown_Empty(t *testing.T) {
	s := FormatIssuesMarkdown(IssuesOutput{})
	if !strings.Contains(s, "No issues found") {
		t.Errorf("expected 'No issues found', got %q", s)
	}
}

// TestFormatIssuesMarkdown_WithResults verifies the behavior of format issues markdown with results.
func TestFormatIssuesMarkdown_WithResults(t *testing.T) {
	s := FormatIssuesMarkdown(IssuesOutput{
		Issues:     []issues.Output{{IID: 3, Title: "Fix login", State: "opened", Author: "dev1", Labels: []string{"bug", "critical"}}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "#3") {
		t.Error("expected #3 in output")
	}
	if !strings.Contains(s, "Issue Search Results (1)") {
		t.Error(errExpectedHdr)
	}
	if !strings.Contains(s, "bug, critical") {
		t.Error("expected labels in output")
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Commits
// ---------------------------------------------------------------------------.

// TestFormatCommitsMarkdown_Empty verifies the behavior of format commits markdown empty.
func TestFormatCommitsMarkdown_Empty(t *testing.T) {
	s := FormatCommitsMarkdown(CommitsOutput{})
	if !strings.Contains(s, "No commits found") {
		t.Errorf("expected 'No commits found', got %q", s)
	}
}

// TestFormatCommitsMarkdown_WithResults verifies the behavior of format commits markdown with results.
func TestFormatCommitsMarkdown_WithResults(t *testing.T) {
	s := FormatCommitsMarkdown(CommitsOutput{
		Commits:    []commits.Output{{ShortID: "abc123", Title: "Initial commit", AuthorName: "Dev", CommittedDate: "2026-01-01"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "abc123") {
		t.Error("expected short ID in output")
	}
	if !strings.Contains(s, "Commit Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Milestones
// ---------------------------------------------------------------------------.

// TestFormatMilestonesMarkdown_Empty verifies the behavior of format milestones markdown empty.
func TestFormatMilestonesMarkdown_Empty(t *testing.T) {
	s := FormatMilestonesMarkdown(MilestonesOutput{})
	if !strings.Contains(s, "No milestones found") {
		t.Errorf("expected 'No milestones found', got %q", s)
	}
}

// TestFormatMilestonesMarkdown_WithResults verifies the behavior of format milestones markdown with results.
func TestFormatMilestonesMarkdown_WithResults(t *testing.T) {
	s := FormatMilestonesMarkdown(MilestonesOutput{
		Milestones: []milestones.Output{{IID: 1, Title: "v1.0", State: "active", DueDate: "2026-06-01"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "v1.0") {
		t.Error("expected milestone title in output")
	}
	if !strings.Contains(s, "Milestone Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// TestFormatMilestonesMarkdown_NoDueDate verifies the behavior of format milestones markdown no due date.
func TestFormatMilestonesMarkdown_NoDueDate(t *testing.T) {
	s := FormatMilestonesMarkdown(MilestonesOutput{
		Milestones: []milestones.Output{{IID: 2, Title: "v2.0", State: "active"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "\u2014") {
		t.Error("expected em-dash for missing due date")
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Notes
// ---------------------------------------------------------------------------.

// TestFormatNotesMarkdown_Empty verifies the behavior of format notes markdown empty.
func TestFormatNotesMarkdown_Empty(t *testing.T) {
	s := FormatNotesMarkdown(NotesOutput{})
	if !strings.Contains(s, "No note search results found") {
		t.Errorf("expected 'No note search results found', got %q", s)
	}
}

// TestFormatNotesMarkdown_WithResults verifies the behavior of format notes markdown with results.
func TestFormatNotesMarkdown_WithResults(t *testing.T) {
	s := FormatNotesMarkdown(NotesOutput{
		Notes:      []NoteOutput{{Author: "reviewer", NoteableType: "Issue", NoteableIID: 5, Body: "Looks good"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "reviewer") {
		t.Error("expected author in output")
	}
	if !strings.Contains(s, "#5") {
		t.Error("expected issue ref in output")
	}
	if !strings.Contains(s, "Note Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Projects
// ---------------------------------------------------------------------------.

// TestFormatProjectsMarkdown_Empty verifies the behavior of format projects markdown empty.
func TestFormatProjectsMarkdown_Empty(t *testing.T) {
	s := FormatProjectsMarkdown(ProjectsOutput{})
	if !strings.Contains(s, "No projects found") {
		t.Errorf("expected 'No projects found', got %q", s)
	}
}

// TestFormatProjectsMarkdown_WithResults verifies the behavior of format projects markdown with results.
func TestFormatProjectsMarkdown_WithResults(t *testing.T) {
	s := FormatProjectsMarkdown(ProjectsOutput{
		Projects:   []projects.Output{{Name: "my-project", PathWithNamespace: "user/my-project", Visibility: "private", DefaultBranch: "main"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "user/my-project") {
		t.Error("expected project path in output")
	}
	if !strings.Contains(s, "Project Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Snippets
// ---------------------------------------------------------------------------.

// TestFormatSnippetsMarkdown_Empty verifies the behavior of format snippets markdown empty.
func TestFormatSnippetsMarkdown_Empty(t *testing.T) {
	s := FormatSnippetsMarkdown(SnippetsOutput{})
	if !strings.Contains(s, "No snippets found") {
		t.Errorf("expected 'No snippets found', got %q", s)
	}
}

// TestFormatSnippetsMarkdown_WithResults verifies the behavior of format snippets markdown with results.
func TestFormatSnippetsMarkdown_WithResults(t *testing.T) {
	s := FormatSnippetsMarkdown(SnippetsOutput{
		Snippets:   []SnippetOutput{{Title: "My snippet", FileName: "notes.md", Visibility: "private", Author: "dev1"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "My snippet") {
		t.Error("expected snippet title in output")
	}
	if !strings.Contains(s, "Snippet Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Users
// ---------------------------------------------------------------------------.

// TestFormatUsersMarkdown_Empty verifies the behavior of format users markdown empty.
func TestFormatUsersMarkdown_Empty(t *testing.T) {
	s := FormatUsersMarkdown(UsersOutput{})
	if !strings.Contains(s, "No users found") {
		t.Errorf("expected 'No users found', got %q", s)
	}
}

// TestFormatUsersMarkdown_WithResults verifies the behavior of format users markdown with results.
func TestFormatUsersMarkdown_WithResults(t *testing.T) {
	s := FormatUsersMarkdown(UsersOutput{
		Users:      []UserOutput{{Username: "admin", Name: "Admin User", State: "active"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "@admin") {
		t.Error("expected @admin in output")
	}
	if !strings.Contains(s, "User Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Wiki
// ---------------------------------------------------------------------------.

// TestFormatWikiMarkdown_Empty verifies the behavior of format wiki markdown empty.
func TestFormatWikiMarkdown_Empty(t *testing.T) {
	s := FormatWikiMarkdown(WikiOutput{})
	if !strings.Contains(s, "No wiki pages found") {
		t.Errorf("expected 'No wiki pages found', got %q", s)
	}
}

// TestFormatWikiMarkdown_WithResults verifies the behavior of format wiki markdown with results.
func TestFormatWikiMarkdown_WithResults(t *testing.T) {
	s := FormatWikiMarkdown(WikiOutput{
		WikiBlobs:  []WikiBlobOutput{{Title: "Home", Slug: "home", Format: "markdown"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if !strings.Contains(s, "Home") {
		t.Error("expected wiki title in output")
	}
	if !strings.Contains(s, "Wiki Search Results (1)") {
		t.Error(errExpectedHdr)
	}
}

// ---------------------------------------------------------------------------
// Helper tests
// ---------------------------------------------------------------------------.

// TestTruncateBody validates truncate body across multiple scenarios using table-driven subtests.
func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world", 5, "hello\u2026"},
		{"newlines", "line1\nline2", 20, "line1 line2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("truncateBody(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// TestNoteableRef validates noteable ref across multiple scenarios using table-driven subtests.
func TestNoteableRef(t *testing.T) {
	tests := []struct {
		nType string
		iid   int64
		want  string
	}{
		{"Issue", 5, "#5"},
		{"MergeRequest", 10, "!10"},
		{"Commit", 0, "Commit"},
		{"Snippet", 3, "Snippet #3"},
	}
	for _, tt := range tests {
		t.Run(tt.nType, func(t *testing.T) {
			got := noteableRef(tt.nType, tt.iid)
			if got != tt.want {
				t.Errorf("noteableRef(%q, %d) = %q, want %q", tt.nType, tt.iid, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// markdownForResult dispatch — new output types
// ---------------------------------------------------------------------------.

// TestMarkdownForResult_IssuesOutput verifies the behavior of markdown for result issues output.
func TestMarkdownForResult_IssuesOutput(t *testing.T) {
	result := markdownForResult(IssuesOutput{})
	if result == nil {
		t.Error("expected non-nil result for IssuesOutput")
	}
}

// TestMarkdownForResult_CommitsOutput verifies the behavior of markdown for result commits output.
func TestMarkdownForResult_CommitsOutput(t *testing.T) {
	result := markdownForResult(CommitsOutput{})
	if result == nil {
		t.Error("expected non-nil result for CommitsOutput")
	}
}

// TestMarkdownForResult_MilestonesOutput verifies the behavior of markdown for result milestones output.
func TestMarkdownForResult_MilestonesOutput(t *testing.T) {
	result := markdownForResult(MilestonesOutput{})
	if result == nil {
		t.Error("expected non-nil result for MilestonesOutput")
	}
}

// TestMarkdownForResult_NotesOutput verifies the behavior of markdown for result notes output.
func TestMarkdownForResult_NotesOutput(t *testing.T) {
	result := markdownForResult(NotesOutput{})
	if result == nil {
		t.Error("expected non-nil result for NotesOutput")
	}
}

// TestMarkdownForResult_ProjectsOutput verifies the behavior of markdown for result projects output.
func TestMarkdownForResult_ProjectsOutput(t *testing.T) {
	result := markdownForResult(ProjectsOutput{})
	if result == nil {
		t.Error("expected non-nil result for ProjectsOutput")
	}
}

// TestMarkdownForResult_SnippetsOutput verifies the behavior of markdown for result snippets output.
func TestMarkdownForResult_SnippetsOutput(t *testing.T) {
	result := markdownForResult(SnippetsOutput{})
	if result == nil {
		t.Error("expected non-nil result for SnippetsOutput")
	}
}

// TestMarkdownForResult_UsersOutput verifies the behavior of markdown for result users output.
func TestMarkdownForResult_UsersOutput(t *testing.T) {
	result := markdownForResult(UsersOutput{})
	if result == nil {
		t.Error("expected non-nil result for UsersOutput")
	}
}

// TestMarkdownForResult_WikiOutput verifies the behavior of markdown for result wiki output.
func TestMarkdownForResult_WikiOutput(t *testing.T) {
	result := markdownForResult(WikiOutput{})
	if result == nil {
		t.Error("expected non-nil result for WikiOutput")
	}
}

// ---------------------------------------------------------------------------
// Registration tests
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllSearchTools validates m c p round trip all search tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllSearchTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		switch scope {
		case "blobs":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"basename":"f","data":"d","path":"f.go","filename":"f.go","ref":"main","startline":1,"project_id":1}]`, defaultPagination)
		case "merge_requests":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"f","target_branch":"main","author":{"username":"u"}}]`, defaultPagination)
		case "issues":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"iid":1,"title":"Issue","state":"opened","author":{"username":"u"}}]`, defaultPagination)
		case "commits":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":"abc","short_id":"abc","title":"Commit","author_name":"A","author_email":"a@a.com","committer_name":"A","committer_email":"a@a.com"}]`, defaultPagination)
		case "milestones":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"iid":1,"title":"v1","state":"active"}]`, defaultPagination)
		case "notes":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"body":"note","noteable_type":"Issue","noteable_id":10}]`, defaultPagination)
		case "projects":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"proj","path":"proj","path_with_namespace":"u/proj","visibility":"private","default_branch":"main","web_url":"https://x"}]`, defaultPagination)
		case "snippet_titles":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"title":"Snip","file_name":"f.md","visibility":"private","web_url":"u","raw_url":"r"}]`, defaultPagination)
		case "users":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"username":"u","name":"U","state":"active","web_url":"https://x"}]`, defaultPagination)
		case "wiki_blobs":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"slug":"home","title":"Home","content":"c","format":"markdown"}]`, defaultPagination)
		default:
			http.NotFound(w, r)
		}
	}))
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_search_code", map[string]any{"query": "test", "project_id": "42"}},
		{"gitlab_search_merge_requests", map[string]any{"query": "test"}},
		{"gitlab_search_issues", map[string]any{"query": "test"}},
		{"gitlab_search_commits", map[string]any{"query": "test"}},
		{"gitlab_search_milestones", map[string]any{"query": "test"}},
		{"gitlab_search_notes", map[string]any{"query": "test", "project_id": "42"}},
		{"gitlab_search_projects", map[string]any{"query": "test"}},
		{"gitlab_search_snippets", map[string]any{"query": "test"}},
		{"gitlab_search_users", map[string]any{"query": "test"}},
		{"gitlab_search_wiki", map[string]any{"query": "test"}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if result.IsError {
				t.Errorf("expected no error for %s", tc.name)
			}
		})
	}
}

// TestMCPRound_TripMetaTool verifies the behavior of m c p round trip meta tool.
func TestMCPRound_TripMetaTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"basename":"f","data":"d","path":"f.go","filename":"f.go","ref":"main","startline":1,"project_id":1}]`, defaultPagination)
	}))
	RegisterMeta(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_search",
		Arguments: map[string]any{
			"action": "code",
			"params": map[string]any{"query": "test", "project_id": "42"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Error("expected no error")
	}
}

// ---------------------------------------------------------------------------
// Pagination adjustment — Search API missing totals
// ---------------------------------------------------------------------------.

// noPagination simulates the GitLab Search API which returns X-Page and
// X-Per-Page but NOT X-Total or X-Total-Pages.
var noPagination = testutil.PaginationHeaders{Page: "1", PerPage: "20"}

// TestSearchProjects_PaginationAdjusted verifies that when the GitLab Search
// API does not return X-Total/X-Total-Pages headers, the handler infers
// correct TotalItems and TotalPages from the actual result count.
func TestSearchProjects_PaginationAdjusted(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get("scope") == "projects" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"name":"P1","path":"p1","path_with_namespace":"u/p1","visibility":"private","default_branch":"main","web_url":"https://x/p1"},
				{"id":2,"name":"P2","path":"p2","path_with_namespace":"u/p2","visibility":"public","default_branch":"main","web_url":"https://x/p2"},
				{"id":3,"name":"P3","path":"p3","path_with_namespace":"u/p3","visibility":"internal","default_branch":"develop","web_url":"https://x/p3"}
			]`, noPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Projects(context.Background(), client, ProjectsInput{Query: "P"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 3 {
		t.Fatalf("len(Projects) = %d, want 3", len(out.Projects))
	}
	if out.Pagination.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 1 {
		t.Errorf("TotalPages = %d, want 1", out.Pagination.TotalPages)
	}

	md := FormatProjectsMarkdown(out)
	if !strings.Contains(md, "Project Search Results (3)") {
		t.Errorf("expected header with count 3, got %q", md)
	}
	if strings.Contains(md, "Page 1 of 0") {
		t.Error("pagination footer should not show 'Page 1 of 0'")
	}
}

// ---------------------------------------------------------------------------
// Special character query edge cases
// ---------------------------------------------------------------------------.

// TestSearchCode_QueryWithDoubleQuotes verifies that queries containing double
// quotes (exact match syntax) are passed through correctly.
func TestSearchCode_QueryWithDoubleQuotes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == scopeBlobs {
			got := r.URL.Query().Get("search")
			if got != `"func main"` {
				t.Errorf("search query = %q, want %q", got, `"func main"`)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"main","data":"func main()","path":"main.go",
				"filename":"main.go","ref":"main","startline":1,"project_id":1
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{Query: `"func main"`})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// TestSearchCode_QueryWithSpecialSymbols verifies that queries with symbols
// like @, #, and & are handled without error.
func TestSearchCode_QueryWithSpecialSymbols(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == scopeBlobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{Query: "user@example.com #tag &ref"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 0 {
		t.Errorf("len(Blobs) = %d, want 0", len(out.Blobs))
	}
}

// TestSearchCode_QueryWithParenthesesAndBrackets verifies that queries with
// parentheses and brackets are handled correctly.
func TestSearchCode_QueryWithParenthesesAndBrackets(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchProject && r.URL.Query().Get(queryScope) == scopeBlobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"util","data":"map[string]int{}","path":"util.go",
				"filename":"util.go","ref":"main","startline":1,"project_id":42
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "map[string]int{}"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// TestSearchIssues_QueryWithUnicode verifies that issue search handles
// Unicode characters in the query string.
func TestSearchIssues_QueryWithUnicode(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == "issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":1,"iid":5,"title":"\u00e9l\u00e8ve probl\u00e8me","state":"opened",
				"web_url":"https://gitlab.example.com/issues/5",
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",
				"labels":[],"assignees":[],"author":{"username":"alice"}
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Issues(context.Background(), client, IssuesInput{Query: "\u00e9l\u00e8ve"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
	}
}

// TestSearchCode_PaginationAdjusted verifies pagination adjustment for code
// search when the API does not return total headers.
func TestSearchCode_PaginationAdjusted(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSearchGlobal && r.URL.Query().Get("scope") == scopeBlobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"basename":"f","data":"d","path":"f.go","filename":"f.go","ref":"main","startline":1,"project_id":1}
			]`, noPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{Query: "test"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 1 {
		t.Errorf("TotalPages = %d, want 1", out.Pagination.TotalPages)
	}
}

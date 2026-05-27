// search_test.go contains unit tests for GitLab search operations
// (code search and merge request search). Tests use httptest to mock
// the GitLab Search API and verify both success and error paths.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
	errExpCancelledNil = "expected error for canceled context, got nil"
	// errExpEmptyQuery identifies the err exp empty query constant used by this package.
	errExpEmptyQuery = "expected error for empty query, got nil"
	// errExpAPIFailure identifies the err exp API failure constant used by this package.
	errExpAPIFailure = "expected error for API failure, got nil"
	// fmtLenBlobsWant1 identifies the fmt len blobs want 1 constant used by this package.
	fmtLenBlobsWant1 = "len(Blobs) = %d, want 1"
	// pathSearchProject identifies the path search project constant used by this package.
	pathSearchProject = "/api/v4/projects/42/-/search"
	// pathSearchGroup identifies the path search group constant used by this package.
	pathSearchGroup = "/api/v4/groups/7/-/search"
	// pathSearchGlobal identifies the path search global constant used by this package.
	pathSearchGlobal = "/api/v4/search"
	// queryScope identifies the query scope constant used by this package.
	queryScope = "scope"
	// scopeBlobs identifies the scope blobs constant used by this package.
	scopeBlobs = "blobs"
	// testMRFixBugTitle identifies the test MR fix bug title constant used by this package.
	testMRFixBugTitle = "Fix bug"
	// testProjectSlug identifies the test project slug constant used by this package.
	testProjectSlug = "my-project"
	// fmtTitleWant identifies the fmt title want constant used by this package.
	fmtTitleWant = "Title = %q, want %q"
)

// defaultPagination stores the package-level default pagination state.
var defaultPagination = testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}

// unsupportedSearchSchemaInput defines parameters for the unsupported search schema operation.
type unsupportedSearchSchemaInput struct {
	Values map[int]string `json:"values"`
}

// TestSearchCode_ProjectScope verifies SearchCode when project scope.
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

// TestSearchCode_SearchType verifies that the optional search_type parameter
// is forwarded to GitLab Search API requests.
func TestSearchCode_SearchType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathSearchGlobal && r.URL.Query().Get(queryScope) == scopeBlobs {
			testutil.AssertQueryParam(t, r, "search_type", "zoekt")
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"basename":"main","data":"func main()","path":"main.go",
				"filename":"main.go","ref":"main","startline":1,"project_id":1
			}]`, defaultPagination)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Code(context.Background(), client, CodeInput{Query: "func main", TypeInput: TypeInput{SearchType: "zoekt"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// TestSearchCode_GlobalScope verifies SearchCode when global scope.
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

// TestSearchCode_GroupScope verifies SearchCode when group scope.
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

// TestSearchCode_EmptyQuery verifies SearchCode when empty query.
func TestSearchCode_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Code(context.Background(), client, CodeInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCode_APIError verifies SearchCode when API error.
func TestSearchCode_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "test"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestSearchMerge_RequestsProjectScope verifies SearchMerge when requests project scope.
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

// TestSearchMerge_RequestsGlobalScope verifies SearchMerge when requests global scope.
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

// TestSearchMergeRequests_EmptyQuery verifies SearchMergeRequests when empty query.
func TestSearchMergeRequests_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := MergeRequests(context.Background(), client, MergeRequestsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCode_CancelledContext verifies SearchCode when cancelled context.
func TestSearchCode_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Code(ctx, client, CodeInput{ProjectID: "42", Query: "test"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSearchIssuesGlobal_Success verifies SearchIssuesGlobal when success.
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

// TestSearchIssuesByProject_Success verifies SearchIssuesByProject when success.
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

// TestSearchIssues_EmptyQuery verifies SearchIssues when empty query.
func TestSearchIssues_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Issues(context.Background(), client, IssuesInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchIssues_APIError verifies SearchIssues when API error.
func TestSearchIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Issues(context.Background(), client, IssuesInput{Query: "bug"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestSearchIssues_CancelledContext verifies SearchIssues when cancelled context.
func TestSearchIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Issues(ctx, client, IssuesInput{Query: "bug"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Commits
// ---------------------------------------------------------------------------.

// TestSearchCommits_GlobalScope verifies SearchCommits when global scope.
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

// TestSearchCommits_ProjectScope verifies SearchCommits when project scope.
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

// TestSearchCommits_EmptyQuery verifies SearchCommits when empty query.
func TestSearchCommits_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Commits(context.Background(), client, CommitsInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchCommits_CancelledContext verifies SearchCommits when cancelled context.
func TestSearchCommits_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Commits(ctx, client, CommitsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Milestones
// ---------------------------------------------------------------------------.

// TestSearchMilestones_GlobalScope verifies SearchMilestones when global scope.
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

// TestSearchMilestones_ProjectScope verifies SearchMilestones when project scope.
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

// TestSearchMilestones_EmptyQuery verifies SearchMilestones when empty query.
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

// TestSearchNotes_ProjectScope verifies SearchNotes when project scope.
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

// TestSearchNotes_MissingProjectID verifies SearchNotes when missing project ID.
func TestSearchNotes_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Notes(context.Background(), client, NotesInput{Query: "test"})
	if err == nil {
		t.Fatal("expected error for missing project_id, got nil")
	}
}

// TestSearchNotes_EmptyQuery verifies SearchNotes when empty query.
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

// TestSearchProjects_GlobalScope verifies SearchProjects when global scope.
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

// TestSearchProjects_GroupScope verifies SearchProjects when group scope.
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

// TestSearchProjects_EmptyQuery verifies SearchProjects when empty query.
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

// TestSearchSnippets_GlobalScope verifies SearchSnippets when global scope.
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

// TestSearchSnippets_EmptyQuery verifies SearchSnippets when empty query.
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

// TestSearchUsers_GlobalScope verifies SearchUsers when global scope.
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

// TestSearchUsers_ProjectScope verifies SearchUsers when project scope.
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

// TestSearchUsers_EmptyQuery verifies SearchUsers when empty query.
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

// TestSearchWiki_GlobalScope verifies SearchWiki when global scope.
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

// TestSearchWiki_ProjectScope verifies SearchWiki when project scope.
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

// TestSearchWiki_EmptyQuery verifies SearchWiki when empty query.
func TestSearchWiki_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := Wiki(context.Background(), client, WikiInput{})
	if err == nil {
		t.Fatal(errExpEmptyQuery)
	}
}

// TestSearchWiki_CancelledContext verifies SearchWiki when cancelled context.
func TestSearchWiki_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Wiki(ctx, client, WikiInput{Query: "test"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	// errExpected identifies the err expected constant used by this package.
	errExpected = "expected error"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// errExpectedHdr identifies the err expected hdr constant used by this package.
	errExpectedHdr = "expected header with count"
	// fmtLenWant1 identifies the fmt len want 1 constant used by this package.
	fmtLenWant1 = "len=%d, want 1"
)

// ---------------------------------------------------------------------------
// searchOpts helper
// ---------------------------------------------------------------------------.

// TestSearchOpts_Defaults verifies SearchOpts when defaults.
func TestSearchOpts_Defaults(t *testing.T) {
	opts, err := searchOpts(0, 0, "", "")
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if opts.Ref != nil {
		t.Errorf("expected nil Ref")
	}
	if opts.SearchType != nil {
		t.Errorf("expected nil SearchType")
	}
	if opts.Page != 0 {
		t.Errorf("expected Page 0, got %d", opts.Page)
	}
}

// TestSearchOpts_AllParams verifies SearchOpts when all params.
func TestSearchOpts_AllParams(t *testing.T) {
	opts, err := searchOpts(3, 50, "develop", "advanced")
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if opts.Ref == nil || *opts.Ref != "develop" {
		t.Error("expected Ref=develop")
	}
	if opts.SearchType == nil || string(*opts.SearchType) != "advanced" {
		t.Error("expected SearchType=advanced")
	}
	if opts.Page != 3 {
		t.Errorf("expected Page=3, got %d", opts.Page)
	}
	if opts.PerPage != 50 {
		t.Errorf("expected PerPage=50, got %d", opts.PerPage)
	}
}

// TestSearchOpts_InvalidSearchType verifies that invalid search_type values
// fail locally with an actionable message before calling GitLab.
func TestSearchOpts_InvalidSearchType(t *testing.T) {
	opts, err := searchOpts(0, 0, "", "semantic")
	if err == nil {
		t.Fatal("expected invalid search_type error, got nil")
	}
	if opts != nil {
		t.Fatalf("expected nil opts for invalid search_type, got %#v", opts)
	}
	if !strings.Contains(err.Error(), "invalid search_type") || !strings.Contains(err.Error(), "basic") {
		t.Fatalf("expected actionable search_type error, got: %v", err)
	}
}

// TestSearchHandlers_InvalidSearchType_ReturnValidationError verifies that every
// search handler rejects unsupported search_type values before calling GitLab.
func TestSearchHandlers_InvalidSearchType_ReturnValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not call GitLab API for invalid search_type: %s %s", r.Method, r.URL.Path)
	}))

	tests := []struct {
		name string
		call func(context.Context, *gitlabclient.Client) error
	}{
		{
			name: "code",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Code(ctx, client, CodeInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "merge requests",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := MergeRequests(ctx, client, MergeRequestsInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "issues",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Issues(ctx, client, IssuesInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "commits",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Commits(ctx, client, CommitsInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "milestones",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Milestones(ctx, client, MilestonesInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "notes",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Notes(ctx, client, NotesInput{ProjectID: "42", Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "projects",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Projects(ctx, client, ProjectsInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "snippets",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Snippets(ctx, client, SnippetsInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "users",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Users(ctx, client, UsersInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
		{
			name: "wiki",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Wiki(ctx, client, WikiInput{Query: "alpha", TypeInput: TypeInput{SearchType: "semantic"}})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), "invalid search_type") {
				t.Fatalf("handler error = %v, want invalid search_type validation error", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group-scope tests (missing from search_test.go)
// ---------------------------------------------------------------------------.

// TestSearchMerge_RequestsGroupScope verifies SearchMerge when requests group scope.
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

// TestSearchIssues_GroupScope verifies SearchIssues when group scope.
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

// TestSearchCommits_GroupScope verifies SearchCommits when group scope.
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

// TestSearchMilestones_GroupScope verifies SearchMilestones when group scope.
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

// TestSearchUsers_GroupScope verifies SearchUsers when group scope.
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

// TestSearchWiki_GroupScope verifies SearchWiki when group scope.
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

// TestSearchMergeRequests_APIError verifies SearchMergeRequests when API error.
func TestSearchMergeRequests_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := MergeRequests(context.Background(), client, MergeRequestsInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchCommits_APIError verifies SearchCommits when API error.
func TestSearchCommits_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Commits(context.Background(), client, CommitsInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchMilestones_APIError verifies SearchMilestones when API error.
func TestSearchMilestones_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Milestones(context.Background(), client, MilestonesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchNotes_APIError verifies SearchNotes when API error.
func TestSearchNotes_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Notes(context.Background(), client, NotesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchProjects_APIError verifies SearchProjects when API error.
func TestSearchProjects_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Projects(context.Background(), client, ProjectsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchSnippets_APIError verifies SearchSnippets when API error.
func TestSearchSnippets_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Snippets(context.Background(), client, SnippetsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchUsers_APIError verifies SearchUsers when API error.
func TestSearchUsers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := Users(context.Background(), client, UsersInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchWiki_APIError verifies SearchWiki when API error.
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

// TestSearchMergeRequests_CancelledCtx verifies SearchMergeRequests when cancelled ctx.
func TestSearchMergeRequests_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := MergeRequests(ctx, client, MergeRequestsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchMilestones_CancelledCtx verifies SearchMilestones when cancelled ctx.
func TestSearchMilestones_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Milestones(ctx, client, MilestonesInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchNotes_CancelledCtx verifies SearchNotes when cancelled ctx.
func TestSearchNotes_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Notes(ctx, client, NotesInput{ProjectID: "42", Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchProjects_CancelledCtx verifies SearchProjects when cancelled ctx.
func TestSearchProjects_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Projects(ctx, client, ProjectsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchSnippets_CancelledCtx verifies SearchSnippets when cancelled ctx.
func TestSearchSnippets_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Snippets(ctx, client, SnippetsInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestSearchUsers_CancelledCtx verifies SearchUsers when cancelled ctx.
func TestSearchUsers_CancelledCtx(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Users(ctx, client, UsersInput{Query: "x"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// Code search with Ref parameter
// ---------------------------------------------------------------------------.

// TestSearchCode_WithRef verifies SearchCode when with ref.
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

// TestSearchNotes_NilAuthorAndDates verifies SearchNotes when nil author and dates.
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

// TestSearchSnippets_NilAuthorAndDates verifies SearchSnippets when nil author and dates.
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

// TestFormatCodeMarkdown_Empty verifies FormatCodeMarkdown when empty.
func TestFormatCodeMarkdown_Empty(t *testing.T) {
	s := FormatCodeMarkdown(CodeOutput{})
	if !strings.Contains(s, "No code search results found") {
		t.Errorf("expected 'No code search results found', got %q", s)
	}
}

// TestFormatCodeMarkdown_WithResults verifies FormatCodeMarkdown when with results.
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

// TestFormatMRsMarkdown_Empty verifies FormatMRsMarkdown when empty.
func TestFormatMRsMarkdown_Empty(t *testing.T) {
	s := FormatMRsMarkdown(MergeRequestsOutput{})
	if !strings.Contains(s, "No merge requests found") {
		t.Errorf("expected 'No merge requests found', got %q", s)
	}
}

// TestFormatMRsMarkdown_WithResults verifies FormatMRsMarkdown when with results.
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

// TestMarkdownForResult_CodeOutput verifies MarkdownForResult when code output.
func TestMarkdownForResult_CodeOutput(t *testing.T) {
	result := markdownForResult(CodeOutput{})
	if result == nil {
		t.Error("expected non-nil result for CodeOutput")
	}
}

// TestMarkdownForResult_MROutput verifies MarkdownForResult when MR output.
func TestMarkdownForResult_MROutput(t *testing.T) {
	result := markdownForResult(MergeRequestsOutput{})
	if result == nil {
		t.Error("expected non-nil result for MergeRequestsOutput")
	}
}

// TestMarkdownForResult_Unknown verifies MarkdownForResult when unknown.
func TestMarkdownForResult_Unknown(t *testing.T) {
	result := markdownForResult("unknown")
	if result != nil {
		t.Error("expected nil for unknown type")
	}
}

// ---------------------------------------------------------------------------
// New formatter tests: Issues
// ---------------------------------------------------------------------------.

// TestFormatIssuesMarkdown_Empty verifies FormatIssuesMarkdown when empty.
func TestFormatIssuesMarkdown_Empty(t *testing.T) {
	s := FormatIssuesMarkdown(IssuesOutput{})
	if !strings.Contains(s, "No issues found") {
		t.Errorf("expected 'No issues found', got %q", s)
	}
}

// TestFormatIssuesMarkdown_WithResults verifies FormatIssuesMarkdown when with results.
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

// TestFormatCommitsMarkdown_Empty verifies FormatCommitsMarkdown when empty.
func TestFormatCommitsMarkdown_Empty(t *testing.T) {
	s := FormatCommitsMarkdown(CommitsOutput{})
	if !strings.Contains(s, "No commits found") {
		t.Errorf("expected 'No commits found', got %q", s)
	}
}

// TestFormatCommitsMarkdown_WithResults verifies FormatCommitsMarkdown when with results.
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

// TestFormatMilestonesMarkdown_Empty verifies FormatMilestonesMarkdown when empty.
func TestFormatMilestonesMarkdown_Empty(t *testing.T) {
	s := FormatMilestonesMarkdown(MilestonesOutput{})
	if !strings.Contains(s, "No milestones found") {
		t.Errorf("expected 'No milestones found', got %q", s)
	}
}

// TestFormatMilestonesMarkdown_WithResults verifies FormatMilestonesMarkdown when with results.
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

// TestFormatMilestonesMarkdown_NoDueDate verifies FormatMilestonesMarkdown when no due date.
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

// TestFormatNotesMarkdown_Empty verifies FormatNotesMarkdown when empty.
func TestFormatNotesMarkdown_Empty(t *testing.T) {
	s := FormatNotesMarkdown(NotesOutput{})
	if !strings.Contains(s, "No note search results found") {
		t.Errorf("expected 'No note search results found', got %q", s)
	}
}

// TestFormatNotesMarkdown_WithResults verifies FormatNotesMarkdown when with results.
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

// TestFormatProjectsMarkdown_Empty verifies FormatProjectsMarkdown when empty.
func TestFormatProjectsMarkdown_Empty(t *testing.T) {
	s := FormatProjectsMarkdown(ProjectsOutput{})
	if !strings.Contains(s, "No projects found") {
		t.Errorf("expected 'No projects found', got %q", s)
	}
}

// TestFormatProjectsMarkdown_WithResults verifies FormatProjectsMarkdown when with results.
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

// TestFormatSnippetsMarkdown_Empty verifies FormatSnippetsMarkdown when empty.
func TestFormatSnippetsMarkdown_Empty(t *testing.T) {
	s := FormatSnippetsMarkdown(SnippetsOutput{})
	if !strings.Contains(s, "No snippets found") {
		t.Errorf("expected 'No snippets found', got %q", s)
	}
}

// TestFormatSnippetsMarkdown_WithResults verifies FormatSnippetsMarkdown when with results.
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

// TestFormatUsersMarkdown_Empty verifies FormatUsersMarkdown when empty.
func TestFormatUsersMarkdown_Empty(t *testing.T) {
	s := FormatUsersMarkdown(UsersOutput{})
	if !strings.Contains(s, "No users found") {
		t.Errorf("expected 'No users found', got %q", s)
	}
}

// TestFormatUsersMarkdown_WithResults verifies FormatUsersMarkdown when with results.
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

// TestFormatWikiMarkdown_Empty verifies FormatWikiMarkdown when empty.
func TestFormatWikiMarkdown_Empty(t *testing.T) {
	s := FormatWikiMarkdown(WikiOutput{})
	if !strings.Contains(s, "No wiki pages found") {
		t.Errorf("expected 'No wiki pages found', got %q", s)
	}
}

// TestFormatWikiMarkdown_WithResults verifies FormatWikiMarkdown when with results.
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

// TestTruncateBody covers TruncateBody with table-driven subtests.
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

// TestNoteableRef covers NoteableRef with table-driven subtests.
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

// TestMarkdownForResult_IssuesOutput verifies MarkdownForResult when issues output.
func TestMarkdownForResult_IssuesOutput(t *testing.T) {
	result := markdownForResult(IssuesOutput{})
	if result == nil {
		t.Error("expected non-nil result for IssuesOutput")
	}
}

// TestMarkdownForResult_CommitsOutput verifies MarkdownForResult when commits output.
func TestMarkdownForResult_CommitsOutput(t *testing.T) {
	result := markdownForResult(CommitsOutput{})
	if result == nil {
		t.Error("expected non-nil result for CommitsOutput")
	}
}

// TestMarkdownForResult_MilestonesOutput verifies MarkdownForResult when milestones output.
func TestMarkdownForResult_MilestonesOutput(t *testing.T) {
	result := markdownForResult(MilestonesOutput{})
	if result == nil {
		t.Error("expected non-nil result for MilestonesOutput")
	}
}

// TestMarkdownForResult_NotesOutput verifies MarkdownForResult when notes output.
func TestMarkdownForResult_NotesOutput(t *testing.T) {
	result := markdownForResult(NotesOutput{})
	if result == nil {
		t.Error("expected non-nil result for NotesOutput")
	}
}

// TestMarkdownForResult_ProjectsOutput verifies MarkdownForResult projects output.
func TestMarkdownForResult_ProjectsOutput(t *testing.T) {
	result := markdownForResult(ProjectsOutput{})
	if result == nil {
		t.Error("expected non-nil result for ProjectsOutput")
	}
}

// TestMarkdownForResult_SnippetsOutput verifies MarkdownForResult when snippets output.
func TestMarkdownForResult_SnippetsOutput(t *testing.T) {
	result := markdownForResult(SnippetsOutput{})
	if result == nil {
		t.Error("expected non-nil result for SnippetsOutput")
	}
}

// TestMarkdownForResult_UsersOutput verifies MarkdownForResult when users output.
func TestMarkdownForResult_UsersOutput(t *testing.T) {
	result := markdownForResult(UsersOutput{})
	if result == nil {
		t.Error("expected non-nil result for UsersOutput")
	}
}

// TestMarkdownForResult_WikiOutput verifies MarkdownForResult when wiki output.
func TestMarkdownForResult_WikiOutput(t *testing.T) {
	result := markdownForResult(WikiOutput{})
	if result == nil {
		t.Error("expected non-nil result for WikiOutput")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs tests
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for search actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := searchSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 10 {
		t.Fatalf("len(ActionSpecs) = %d, want 10", len(byTool))
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "search" {
			t.Errorf("OwnerPackage for %s = %q, want search", spec.Name, spec.OwnerPackage)
		}
		if !spec.ReadOnly || !spec.Idempotent {
			t.Errorf("%s should be read-only and idempotent", spec.Name)
		}
	}
}

// TestRegisterMeta_NoPanic verifies RegisterMeta when no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	registerSearchMetaForTest(t, server, client)
}

// TestActionSpecs_SearchTypeSchemaEnum verifies that every search route exposes
// search_type as a constrained enum in its input schema.
func TestActionSpecs_SearchTypeSchemaEnum(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := searchSpecsByTool(t, ActionSpecs(client))

	for _, name := range []string{
		"gitlab_search_code",
		"gitlab_search_merge_requests",
		"gitlab_search_issues",
		"gitlab_search_commits",
		"gitlab_search_milestones",
		"gitlab_search_notes",
		"gitlab_search_projects",
		"gitlab_search_snippets",
		"gitlab_search_users",
		"gitlab_search_wiki",
	} {
		t.Run(name, func(t *testing.T) {
			requireSearchTypeEnum(t, schemaMapFromAny(t, byTool[name].Route.InputSchema))
		})
	}
}

// TestActionSpecs_SearchDisambiguationUsage verifies confusing search scopes
// expose selection hints for meta and dynamic tool descriptions.
func TestActionSpecs_SearchDisambiguationUsage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := searchSpecsByTool(t, ActionSpecs(client))

	code := byTool["gitlab_search_code"]
	if !strings.Contains(code.Usage, "file contents") || !strings.Contains(code.Usage, "do not use for project") {
		t.Fatalf("code Usage = %q, want file-content/project distinction", code.Usage)
	}

	projects := byTool["gitlab_search_projects"]
	if !strings.Contains(projects.Usage, "fuzzy project name") || !strings.Contains(projects.Usage, "use project.get instead") || !strings.Contains(projects.Usage, "Do not use for code") {
		t.Fatalf("projects Usage = %q, want project/code distinction", projects.Usage)
	}
}

// TestRegisterMeta_SearchTypeActionSchemaEnum verifies that gitlab_search
// action schemas expose the same search_type enum as the individual tools.
func TestRegisterMeta_SearchTypeActionSchemaEnum(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	routes := searchActionSpecRoutes(t, client)

	for _, action := range []string{"code", "merge_requests", "issues", "commits", "milestones", "notes", "projects", "snippets", "users", "wiki"} {
		t.Run(action, func(t *testing.T) {
			schema, ok := toolutil.LookupMetaActionSchema(map[string]toolutil.ActionMap{"gitlab_search": routes}, "gitlab_search", action)
			if !ok {
				t.Fatalf("missing gitlab_search/%s schema", action)
			}
			requireSearchTypeEnum(t, schema)
		})
	}
}

// TestRegisterMeta_UsesActionSpecs verifies that gitlab_search meta routes are
// projected from the canonical ActionSpec definitions.
func TestRegisterMeta_UsesActionSpecs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	got := searchActionSpecRoutes(t, client)
	want := ActionSpecs(client)

	if len(got) != len(want) {
		t.Fatalf("search route count = %d, want %d", len(got), len(want))
	}
	for _, spec := range want {
		t.Run(spec.Name, func(t *testing.T) {
			gotRoute, ok := got[spec.Name]
			if !ok {
				t.Fatalf("search routes missing %q", spec.Name)
			}
			if gotRoute.Destructive != spec.Route.Destructive {
				t.Fatalf("destructive = %t, want %t", gotRoute.Destructive, spec.Route.Destructive)
			}
			if !reflect.DeepEqual(gotRoute.InputSchema, spec.Route.InputSchema) {
				t.Fatal("input schema differs from ActionSpec projection")
			}
			if !reflect.DeepEqual(gotRoute.OutputSchema, spec.Route.OutputSchema) {
				t.Fatal("output schema differs from ActionSpec projection")
			}
		})
	}
}

// searchActionSpecRoutes supports search action spec routes assertions in search tests.
func searchActionSpecRoutes(t *testing.T, client *gitlabclient.Client) toolutil.ActionMap {
	t.Helper()
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	return routes
}

// TestSearchInputSchema_UnsupportedTypePanics verifies that unsupported schema
// shapes panic with search input schema context during registration.
func TestSearchInputSchema_UnsupportedTypePanics(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("searchInputSchema() did not panic for unsupported map key type")
		}
		if !strings.Contains(fmt.Sprint(recovered), "search input schema") {
			t.Fatalf("panic = %v, want search input schema context", recovered)
		}
	}()

	_ = searchInputSchema[unsupportedSearchSchemaInput]()
}

// TestSearchSchemaPanic_ErrorPanics verifies that searchSchemaPanic ignores nil
// errors and panics with operation context for non-nil errors.
func TestSearchSchemaPanic_ErrorPanics(t *testing.T) {
	searchSchemaPanic("noop", nil)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("searchSchemaPanic() did not panic for non-nil error")
		}
		if !strings.Contains(fmt.Sprint(recovered), "search input schema marshal") {
			t.Fatalf("panic = %v, want operation context", recovered)
		}
	}()

	searchSchemaPanic("marshal", errors.New("boom"))
}

// schemaMapFromAny extracts schema map from any details for schema assertions.
func schemaMapFromAny(t *testing.T, raw any) map[string]any {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema map[string]any
	if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
		t.Fatalf("unmarshal schema: %v", unmarshalErr)
	}
	return schema
}

// requireSearchTypeEnum returns search type enum test data or fails the test.
func requireSearchTypeEnum(t *testing.T, schema map[string]any) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing or invalid: %#v", schema["properties"])
	}
	searchType, ok := properties["search_type"].(map[string]any)
	if !ok {
		t.Fatalf("search_type property missing or invalid: %#v", properties["search_type"])
	}
	got, ok := searchType["enum"].([]any)
	if !ok {
		t.Fatalf("search_type enum missing or invalid: %#v", searchType["enum"])
	}
	want := []any{"basic", "advanced", "zoekt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search_type enum = %#v, want %#v", got, want)
	}
	if !strings.Contains(fmt.Sprint(searchType["description"]), "enabled on the GitLab instance") {
		t.Fatalf("search_type description lacks backend availability hint: %#v", searchType["description"])
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestActionSpecs_AllSearchRoutes validates all search routes across multiple scenarios.
func TestActionSpecs_AllSearchRoutes(t *testing.T) {
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
	byTool := searchSpecsByTool(t, ActionSpecs(client))

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
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler %s: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}

// searchSpecsByTool supports search specs by tool assertions in search tests.
func searchSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestMCPRound_TripMetaTool verifies MCPRound when trip meta tool.
func TestMCPRound_TripMetaTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"basename":"f","data":"d","path":"f.go","filename":"f.go","ref":"main","startline":1,"project_id":1}]`, defaultPagination)
	}))
	registerSearchMetaForTest(t, server, client)

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

// registerSearchMetaForTest supports register search meta for test assertions in search tests.
func registerSearchMetaForTest(t *testing.T, server *mcp.Server, client *gitlabclient.Client) {
	t.Helper()
	toolutil.AddReadOnlyMetaTool(server, "gitlab_search", "Search GitLab by scope.", searchActionSpecRoutes(t, client), toolutil.IconSearch, markdownForResult)
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

// TestWrapSearchErr_422 verifies that wrapSearchErr enriches 422 errors with
// a query-syntax hint instead of using the generic WrapErrWithMessage fallback.
func TestWrapSearchErr_422(t *testing.T) {
	glErr := &gl.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusUnprocessableEntity},
		Message:  "invalid query syntax",
	}
	err := wrapSearchErr("search_code", glErr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "query format") {
		t.Errorf("expected 422 hint about query format, got: %v", err)
	}
}

// TestWrapSearchErr_400 verifies that bad request errors include guidance for
// unsupported or disabled search_type backends.
func TestWrapSearchErr_400(t *testing.T) {
	glErr := &gl.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusBadRequest},
		Message:  "search_type is not supported",
	}
	err := wrapSearchErr("search_code", glErr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "search_type") || !strings.Contains(err.Error(), "backend") {
		t.Errorf("expected 400 hint about search_type backend availability, got: %v", err)
	}
}

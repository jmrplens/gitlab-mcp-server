// commits_test.go contains unit tests for GitLab commit operations
// (single-file create, multi-action create, start branch, and error handling).
// Tests use httptest to mock the GitLab Commits API.
package commits

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Test endpoint paths and fixture values used across commit operation tests.
const (
	errExpAPIFailure      = "expected error for API failure, got nil"
	errExpEmptyProjectID  = "expected error for empty project_id, got nil"
	errExpCancelledNil    = "expected error for canceled context, got nil"
	pathRepoCommits       = "/api/v4/projects/42/repository/commits"
	testCommitMsgAdd      = "feat: add main.go"
	fmtCommitCreateErr    = "Create() unexpected error: %v"
	testCommitMsgRefactor = "refactor: restructure project"
	testShortID           = "abc123de"
	actionCreate          = "create"
	fmtOutShortIDWant     = "out.ShortID = %q, want %q"
	testFileMainGo        = "main.go"
	testCIURL             = "https://ci.example.com"
	errExpShortID         = "expected short ID"
	errExpHeader          = "expected header"
	testReviewer          = "reviewer1"
	argProjectID          = "project_id"
	testSHA               = "abc123"
	fmtCommitListErr      = "List() unexpected error: %v"
	fmtAuthorWant         = "Author = %q, want %q"
)

// TestCommitCreate_SingleFile verifies that Create creates a commit with
// a single file action and returns the correct short ID, title, and web URL.
// The mock returns HTTP 201 with complete commit metadata.
func TestCommitCreate_SingleFile(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoCommits {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"abc123def456789",
				"short_id":"abc123de",
				"title":"feat: add main.go",
				"author_name":"Test User",
				"author_email":"test@example.com",
				"committed_date":"2026-03-02T10:00:00Z",
				"web_url":"https://gitlab.example.com/mygroup/api/-/commit/abc123def456789"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: testCommitMsgAdd,
		Actions: []Action{
			{Action: actionCreate, FilePath: testFileMainGo, Content: "package main\n"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCommitCreateErr, err)
	}
	if out.ShortID != testShortID {
		t.Errorf(fmtOutShortIDWant, out.ShortID, testShortID)
	}
	if out.Title != testCommitMsgAdd {
		t.Errorf("out.Title = %q, want %q", out.Title, testCommitMsgAdd)
	}
	if out.WebURL == "" {
		t.Error("out.WebURL is empty")
	}
}

// TestCommitCreate_MultipleActions verifies that Create handles a commit
// with multiple file actions (create, update, delete, move) in a single request.
// The mock returns HTTP 201 and the test asserts the title is correct.
func TestCommitCreate_MultipleActions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoCommits {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"multi123",
				"short_id":"multi123",
				"title":"refactor: restructure project"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		Branch:        "develop",
		CommitMessage: testCommitMsgRefactor,
		Actions: []Action{
			{Action: actionCreate, FilePath: "cmd/main.go", Content: "package main\n"},
			{Action: "update", FilePath: "README.md", Content: "# Updated\n"},
			{Action: "delete", FilePath: "old_file.go"},
			{Action: "move", FilePath: "pkg/utils.go", PreviousPath: "utils.go"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCommitCreateErr, err)
	}
	if out.Title != testCommitMsgRefactor {
		t.Errorf("out.Title = %q, want %q", out.Title, testCommitMsgRefactor)
	}
}

// TestCommitCreate_WithStartBranch verifies that Create supports the
// start_branch parameter, allowing a commit on a new branch derived from an
// existing one. The mock returns HTTP 201 with the expected short ID.
func TestCommitCreate_WithStartBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoCommits {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":"start123","short_id":"start123","title":"feat: new file on new branch"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		Branch:        "feature/new",
		CommitMessage: "feat: new file on new branch",
		StartBranch:   "main",
		Actions: []Action{
			{Action: actionCreate, FilePath: "new_file.go", Content: "package new\n"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCommitCreateErr, err)
	}
	if out.ShortID != "start123" {
		t.Errorf(fmtOutShortIDWant, out.ShortID, "start123")
	}
}

// TestCommitCreateServer_Error verifies that Create returns an error
// when the GitLab API responds with an error (e.g., duplicate file). The mock
// returns HTTP 400.
func TestCommitCreateServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"A file with this name already exists"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: "duplicate",
		Actions: []Action{
			{Action: actionCreate, FilePath: "existing.go", Content: "dup\n"},
		},
	})
	if err == nil {
		t.Fatal("Create() expected error, got nil")
	}
}

// TestCommitList_Success verifies that List returns commits with correct
// metadata and pagination headers.
func TestCommitList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCommits {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":"abc123def456789",
					"short_id":"abc123de",
					"title":"feat: add main.go",
					"author_name":"Test User",
					"author_email":"test@example.com",
					"committed_date":"2026-03-02T10:00:00Z",
					"web_url":"https://gitlab.example.com/mygroup/api/-/commit/abc123def456789"
				},
				{
					"id":"def456abc789012",
					"short_id":"def456ab",
					"title":"fix: update readme",
					"author_name":"Another User",
					"author_email":"another@example.com",
					"committed_date":"2026-03-01T09:00:00Z",
					"web_url":"https://gitlab.example.com/mygroup/api/-/commit/def456abc789012"
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf(fmtCommitListErr, err)
	}
	if len(out.Commits) != 2 {
		t.Fatalf("len(Commits) = %d, want 2", len(out.Commits))
	}
	if out.Commits[0].ShortID != testShortID {
		t.Errorf("Commits[0].ShortID = %q, want %q", out.Commits[0].ShortID, testShortID)
	}
	if out.Commits[1].Title != "fix: update readme" {
		t.Errorf("Commits[1].Title = %q, want %q", out.Commits[1].Title, "fix: update readme")
	}
	if out.Commits[0].WebURL == "" {
		t.Error("Commits[0].WebURL is empty")
	}
}

// TestCommitList_WithRefName verifies that List passes the ref_name filter
// to the GitLab API.
func TestCommitList_WithRefName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCommits {
			q := r.URL.Query()
			if q.Get("ref_name") != "develop" {
				t.Errorf("expected ref_name=develop, got %q", q.Get("ref_name"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":"abc","short_id":"abc","title":"commit on develop","web_url":"https://gitlab.example.com/-/commit/abc"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		RefName:   "develop",
	})
	if err != nil {
		t.Fatalf(fmtCommitListErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
}

// TestCommitList_EmptyProjectID verifies that List returns an actionable
// error when project_id is empty.
func TestCommitList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for empty project_id, got nil")
	}
}

// TestCommitListServer_Error verifies that List returns an error when
// the GitLab API responds with an error.
func TestCommitListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

// TestCommitList_CancelledContext verifies that List returns an error
// when the context is canceled.
func TestCommitList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestCommitGet_Success verifies that Get retrieves a single commit
// with full details including stats and parent IDs.
func TestCommitGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":"abc123def456789",
				"short_id":"abc123de",
				"title":"feat: add main.go",
				"message":"feat: add main.go\n\nDetailed description",
				"author_name":"Test User",
				"author_email":"test@example.com",
				"committed_date":"2026-03-02T10:00:00Z",
				"web_url":"https://gitlab.example.com/-/commit/abc123def456789",
				"parent_ids":["parent1","parent2"],
				"stats":{"additions":10,"deletions":2,"total":12}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		SHA:       testSHA,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ShortID != testShortID {
		t.Errorf(fmtOutShortIDWant, out.ShortID, testShortID)
	}
	if out.Message != "feat: add main.go\n\nDetailed description" {
		t.Errorf("out.Message = %q, want detailed message", out.Message)
	}
	if len(out.ParentIDs) != 2 {
		t.Errorf("len(ParentIDs) = %d, want 2", len(out.ParentIDs))
	}
	if out.Stats == nil {
		t.Fatal("out.Stats is nil, want non-nil")
	}
	if out.Stats.Additions != 10 {
		t.Errorf("out.Stats.Additions = %d, want 10", out.Stats.Additions)
	}
	if out.Stats.Total != 12 {
		t.Errorf("out.Stats.Total = %d, want 12", out.Stats.Total)
	}
}

// TestCommitGet_EmptyProjectID verifies Get returns an error for empty project_id.
func TestCommitGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestCommitDiff_Success verifies that Diff returns diffs for a commit.
func TestCommitDiff_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/diff" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"old_path":"main.go",
					"new_path":"main.go",
					"diff":"@@ -0,0 +1,3 @@\n+package main",
					"new_file":true,
					"renamed_file":false,
					"deleted_file":false
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Diff(context.Background(), client, DiffInput{
		ProjectID: "42",
		SHA:       testSHA,
	})
	if err != nil {
		t.Fatalf("Diff() unexpected error: %v", err)
	}
	if len(out.Diffs) != 1 {
		t.Fatalf("len(Diffs) = %d, want 1", len(out.Diffs))
	}
	if out.Diffs[0].NewPath != testFileMainGo {
		t.Errorf("Diffs[0].NewPath = %q, want %q", out.Diffs[0].NewPath, testFileMainGo)
	}
	if !out.Diffs[0].NewFile {
		t.Error("Diffs[0].NewFile = false, want true")
	}
}

// TestCommitDiff_EmptyProjectID verifies Diff returns an error for empty project_id.
func TestCommitDiff_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := Diff(context.Background(), client, DiffInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetRefs tests
// ---------------------------------------------------------------------------.

// TestGetRefs_Success verifies GetRefs returns refs referencing a commit.
func TestGetRefs_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/refs" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"type":"branch","name":"main"},
				{"type":"tag","name":"v1.0.0"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRefs(context.Background(), client, RefsInput{ProjectID: "42", SHA: testSHA})
	if err != nil {
		t.Fatalf("GetRefs() unexpected error: %v", err)
	}
	if len(out.Refs) != 2 {
		t.Fatalf("len(Refs) = %d, want 2", len(out.Refs))
	}
	if out.Refs[0].Type != "branch" || out.Refs[0].Name != "main" {
		t.Errorf("Refs[0] = %+v, want branch/main", out.Refs[0])
	}
	if out.Refs[1].Type != "tag" || out.Refs[1].Name != "v1.0.0" {
		t.Errorf("Refs[1] = %+v, want tag/v1.0.0", out.Refs[1])
	}
}

// TestGetRefs_EmptyProjectID verifies GetRefs returns an error for empty project_id.
func TestGetRefs_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := GetRefs(context.Background(), client, RefsInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetComments / PostComment tests
// ---------------------------------------------------------------------------.

// TestGetComments_Success verifies GetComments returns commit comments.
func TestGetComments_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/comments" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"note":"Looks good!",
					"path":"main.go",
					"line":10,
					"line_type":"new",
					"author":{"username":"reviewer1","name":"Reviewer One"}
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetComments(context.Background(), client, CommentsInput{ProjectID: "42", SHA: testSHA})
	if err != nil {
		t.Fatalf("GetComments() unexpected error: %v", err)
	}
	if len(out.Comments) != 1 {
		t.Fatalf("len(Comments) = %d, want 1", len(out.Comments))
	}
	c := out.Comments[0]
	if c.Note != "Looks good!" {
		t.Errorf("Note = %q, want %q", c.Note, "Looks good!")
	}
	if c.Author != testReviewer {
		t.Errorf(fmtAuthorWant, c.Author, testReviewer)
	}
	if c.Path != testFileMainGo {
		t.Errorf("Path = %q, want %q", c.Path, testFileMainGo)
	}
}

// TestPostComment_Success verifies PostComment creates a commit comment.
func TestPostComment_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/comments" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"note":"LGTM",
				"path":"",
				"line":0,
				"line_type":"",
				"author":{"username":"dev1","name":"Dev One"}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PostComment(context.Background(), client, PostCommentInput{
		ProjectID: "42",
		SHA:       testSHA,
		Note:      "LGTM",
	})
	if err != nil {
		t.Fatalf("PostComment() unexpected error: %v", err)
	}
	if out.Note != "LGTM" {
		t.Errorf("Note = %q, want %q", out.Note, "LGTM")
	}
	if out.Author != "dev1" {
		t.Errorf(fmtAuthorWant, out.Author, "dev1")
	}
}

// TestPostComment_EmptyProjectID verifies PostComment returns an error for empty project_id.
func TestPostComment_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "{}")
	}))

	_, err := PostComment(context.Background(), client, PostCommentInput{SHA: "abc", Note: "test"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetStatuses / SetStatus tests
// ---------------------------------------------------------------------------.

// TestGetStatuses_Success verifies GetStatuses returns commit pipeline statuses.
func TestGetStatuses_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/statuses" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":1,
					"sha":"abc123",
					"ref":"main",
					"status":"success",
					"name":"build",
					"target_url":"https://ci.example.com/1",
					"description":"Build passed",
					"allow_failure":false
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatuses(context.Background(), client, StatusesInput{ProjectID: "42", SHA: testSHA})
	if err != nil {
		t.Fatalf("GetStatuses() unexpected error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("len(Statuses) = %d, want 1", len(out.Statuses))
	}
	s := out.Statuses[0]
	if s.Status != "success" {
		t.Errorf("Status = %q, want %q", s.Status, "success")
	}
	if s.Name != "build" {
		t.Errorf("Name = %q, want %q", s.Name, "build")
	}
}

// TestSetStatus_Success verifies SetStatus sets a commit pipeline status.
func TestSetStatus_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/statuses/abc123" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":2,
				"sha":"abc123",
				"ref":"main",
				"status":"success",
				"name":"deploy",
				"description":"Deployed to staging",
				"allow_failure":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{
		ProjectID:   "42",
		SHA:         testSHA,
		State:       "success",
		Name:        "deploy",
		Description: "Deployed to staging",
	})
	if err != nil {
		t.Fatalf("SetStatus() unexpected error: %v", err)
	}
	if out.Status != "success" {
		t.Errorf("Status = %q, want %q", out.Status, "success")
	}
	if out.Name != "deploy" {
		t.Errorf("Name = %q, want %q", out.Name, "deploy")
	}
}

// TestSetStatus_EmptyProjectID verifies SetStatus returns an error for empty project_id.
func TestSetStatus_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "{}")
	}))

	_, err := SetStatus(context.Background(), client, SetStatusInput{SHA: "abc", State: "success"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// ListMRsByCommit tests
// ---------------------------------------------------------------------------.

// TestListMRsByCommit_Success verifies ListMRsByCommit returns MRs for a commit.
func TestListMRsByCommit_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/merge_requests" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{
					"id":100,
					"iid":1,
					"title":"Feature: add login",
					"state":"merged",
					"source_branch":"feature/login",
					"target_branch":"main",
					"web_url":"https://gitlab.example.com/mygroup/api/-/merge_requests/1",
					"author":{"username":"dev1"}
				}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListMRsByCommit(context.Background(), client, MRsByCommitInput{ProjectID: "42", SHA: testSHA})
	if err != nil {
		t.Fatalf("ListMRsByCommit() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
	mr := out.MergeRequests[0]
	if mr.IID != 1 {
		t.Errorf("IID = %d, want 1", mr.IID)
	}
	if mr.State != "merged" {
		t.Errorf("State = %q, want %q", mr.State, "merged")
	}
}

// TestListMRsByCommit_EmptyProjectID verifies ListMRsByCommit returns an error for empty project_id.
func TestListMRsByCommit_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := ListMRsByCommit(context.Background(), client, MRsByCommitInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// CherryPick tests
// ---------------------------------------------------------------------------.

// TestCherryPick_Success verifies CherryPick returns the cherry-picked commit.
func TestCherryPick_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/cherry_pick" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"def456789012345",
				"short_id":"def45678",
				"title":"feat: add main.go",
				"author_name":"Test User",
				"author_email":"test@example.com",
				"committed_date":"2026-03-02T10:00:00Z",
				"web_url":"https://gitlab.example.com/mygroup/api/-/commit/def456789012345"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CherryPick(context.Background(), client, CherryPickInput{
		ProjectID: "42",
		SHA:       testSHA,
		Branch:    "develop",
	})
	if err != nil {
		t.Fatalf("CherryPick() unexpected error: %v", err)
	}
	if out.ShortID != "def45678" {
		t.Errorf("ShortID = %q, want %q", out.ShortID, "def45678")
	}
}

// TestCherryPick_EmptyProjectID verifies CherryPick returns an error for empty project_id.
func TestCherryPick_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "{}")
	}))

	_, err := CherryPick(context.Background(), client, CherryPickInput{SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Revert tests
// ---------------------------------------------------------------------------.

// TestRevert_Success verifies Revert returns the revert commit.
func TestRevert_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/revert" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":"ghi789012345678",
				"short_id":"ghi78901",
				"title":"Revert \"feat: add main.go\"",
				"author_name":"Test User",
				"author_email":"test@example.com",
				"committed_date":"2026-03-02T11:00:00Z",
				"web_url":"https://gitlab.example.com/mygroup/api/-/commit/ghi789012345678"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Revert(context.Background(), client, RevertInput{
		ProjectID: "42",
		SHA:       testSHA,
		Branch:    "main",
	})
	if err != nil {
		t.Fatalf("Revert() unexpected error: %v", err)
	}
	if out.ShortID != "ghi78901" {
		t.Errorf("ShortID = %q, want %q", out.ShortID, "ghi78901")
	}
}

// TestRevert_EmptyProjectID verifies Revert returns an error for empty project_id.
func TestRevert_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "{}")
	}))

	_, err := Revert(context.Background(), client, RevertInput{SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetGPGSignature tests
// ---------------------------------------------------------------------------.

// TestGetGPGSignature_Success verifies GetGPGSignature returns signature info.
func TestGetGPGSignature_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123/signature" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"gpg_key_id":1,
				"gpg_key_primary_keyid":"ABC123DEF456",
				"gpg_key_user_name":"Test User",
				"gpg_key_user_email":"test@example.com",
				"verification_status":"verified",
				"gpg_key_subkey_id":0
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetGPGSignature(context.Background(), client, GPGSignatureInput{ProjectID: "42", SHA: testSHA})
	if err != nil {
		t.Fatalf("GetGPGSignature() unexpected error: %v", err)
	}
	if out.VerificationStatus != "verified" {
		t.Errorf("VerificationStatus = %q, want %q", out.VerificationStatus, "verified")
	}
	if out.KeyPrimaryKeyID != "ABC123DEF456" {
		t.Errorf("KeyPrimaryKeyID = %q, want %q", out.KeyPrimaryKeyID, "ABC123DEF456")
	}
	if out.KeyUserName != "Test User" {
		t.Errorf("KeyUserName = %q, want %q", out.KeyUserName, "Test User")
	}
}

// TestGetGPGSignature_EmptyProjectID verifies GetGPGSignature returns an error for empty project_id.
func TestGetGPGSignature_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "{}")
	}))

	_, err := GetGPGSignature(context.Background(), client, GPGSignatureInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Canceled Context Tests
// ---------------------------------------------------------------------------.

// TestCommitCreate_CancelledContext verifies the behavior of commit create cancelled context.
func TestCommitCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Branch: "main", CommitMessage: "t", Actions: []Action{{Action: actionCreate, FilePath: "f"}}})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCommitGet_CancelledContext verifies the behavior of commit get cancelled context.
func TestCommitGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCommitDiff_CancelledContext verifies the behavior of commit diff cancelled context.
func TestCommitDiff_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Diff(ctx, client, DiffInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetRefs_CancelledContext verifies the behavior of get refs cancelled context.
func TestGetRefs_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetRefs(ctx, client, RefsInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetComments_CancelledContext verifies the behavior of get comments cancelled context.
func TestGetComments_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetComments(ctx, client, CommentsInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestPostComment_CancelledContext verifies the behavior of post comment cancelled context.
func TestPostComment_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := PostComment(ctx, client, PostCommentInput{ProjectID: "42", SHA: "abc", Note: "t"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetStatuses_CancelledContext verifies the behavior of get statuses cancelled context.
func TestGetStatuses_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetStatuses(ctx, client, StatusesInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSetStatus_CancelledContext verifies the behavior of set status cancelled context.
func TestSetStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := SetStatus(ctx, client, SetStatusInput{ProjectID: "42", SHA: "abc", State: "success"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListMRsByCommit_CancelledContext verifies the behavior of list m rs by commit cancelled context.
func TestListMRsByCommit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListMRsByCommit(ctx, client, MRsByCommitInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCherryPick_CancelledContext verifies the behavior of cherry pick cancelled context.
func TestCherryPick_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CherryPick(ctx, client, CherryPickInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRevert_CancelledContext verifies the behavior of revert cancelled context.
func TestRevert_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Revert(ctx, client, RevertInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetGPGSignature_CancelledContext verifies the behavior of get g p g signature cancelled context.
func TestGetGPGSignature_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGPGSignature(ctx, client, GPGSignatureInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// API Error Tests
// ---------------------------------------------------------------------------.

// TestCommitGet_APIError verifies the behavior of commit get a p i error.
func TestCommitGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Commit Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", SHA: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCommitDiff_APIError verifies the behavior of commit diff a p i error.
func TestCommitDiff_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Diff(context.Background(), client, DiffInput{ProjectID: "42", SHA: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGetRefs_APIError verifies the behavior of get refs a p i error.
func TestGetRefs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := GetRefs(context.Background(), client, RefsInput{ProjectID: "42", SHA: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGetComments_APIError verifies the behavior of get comments a p i error.
func TestGetComments_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500 Error"}`)
	}))
	_, err := GetComments(context.Background(), client, CommentsInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestPostComment_APIError verifies the behavior of post comment a p i error.
func TestPostComment_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := PostComment(context.Background(), client, PostCommentInput{ProjectID: "42", SHA: "abc", Note: "x"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGetStatuses_APIError verifies the behavior of get statuses a p i error.
func TestGetStatuses_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := GetStatuses(context.Background(), client, StatusesInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestSetStatus_APIError verifies the behavior of set status a p i error.
func TestSetStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := SetStatus(context.Background(), client, SetStatusInput{ProjectID: "42", SHA: "abc", State: "success"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestListMRsByCommit_APIError verifies the behavior of list m rs by commit a p i error.
func TestListMRsByCommit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := ListMRsByCommit(context.Background(), client, MRsByCommitInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestCherryPick_APIError verifies the behavior of cherry pick a p i error.
func TestCherryPick_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
	}))
	_, err := CherryPick(context.Background(), client, CherryPickInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRevert_APIError verifies the behavior of revert a p i error.
func TestRevert_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
	}))
	_, err := Revert(context.Background(), client, RevertInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestGetGPGSignature_APIError verifies the behavior of get g p g signature a p i error.
func TestGetGPGSignature_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := GetGPGSignature(context.Background(), client, GPGSignatureInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// Handler Edge Cases (optional fields, filters)
// ---------------------------------------------------------------------------.

// TestCommitCreate_WithAllOptions verifies the behavior of commit create with all options.
func TestCommitCreate_WithAllOptions(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoCommits {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":"opt1","short_id":"opt1","title":"t"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		Branch:        "feat",
		CommitMessage: "t",
		StartSHA:      testSHA,
		AuthorEmail:   "a@t.com",
		AuthorName:    "Author",
		Force:         true,
		Actions: []Action{
			{Action: actionCreate, FilePath: "f.go", Content: "x", LastCommitID: "prev1"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCommitCreateErr, err)
	}
	if out.ShortID != "opt1" {
		t.Errorf(fmtOutShortIDWant, out.ShortID, "opt1")
	}
	for _, want := range []string{"start_sha", "author_email", "author_name", "force", "actions"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestCommitCreate_EmptyProjectID verifies the behavior of commit create empty project i d.
func TestCommitCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	_, err := Create(context.Background(), client, CreateInput{Branch: "main", CommitMessage: "t", Actions: []Action{{Action: actionCreate, FilePath: "f"}}})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// commitListAllOptionsHandler is an internal helper for the commits package.
func commitListAllOptionsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != pathRepoCommits {
			http.NotFound(w, r)
			return
		}
		testutil.AssertQueryParam(t, r, "ref_name", "main")
		testutil.AssertQueryParam(t, r, "since", "2026-01-01T00:00:00Z")
		testutil.AssertQueryParam(t, r, "until", "2026-12-31T23:59:59Z")
		testutil.AssertQueryParam(t, r, "path", "src/")
		testutil.AssertQueryParam(t, r, "author", "Alice")
		testutil.AssertQueryParam(t, r, "with_stats", "true")
		testutil.AssertQueryParam(t, r, "first_parent", "true")
		testutil.RespondJSON(w, http.StatusOK, `[{"id":"a","short_id":"a","title":"t","committed_date":"2026-01-01T00:00:00Z","web_url":"u","stats":{"additions":5,"deletions":2,"total":7}}]`)
	}
}

// TestCommitList_WithAllOptions verifies the behavior of commit list with all options.
func TestCommitList_WithAllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, commitListAllOptionsHandler(t))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:   "42",
		RefName:     "main",
		Since:       "2026-01-01T00:00:00Z",
		Until:       "2026-12-31T23:59:59Z",
		Path:        "src/",
		Author:      "Alice",
		WithStats:   true,
		FirstParent: true,
	})
	if err != nil {
		t.Fatalf(fmtCommitListErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	if out.Commits[0].Stats == nil {
		t.Error("expected Stats to be non-nil")
	}
}

// TestCommitDiff_WithUnidiff verifies the behavior of commit diff with unidiff.
func TestCommitDiff_WithUnidiff(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc/diff" {
			q := r.URL.Query()
			if q.Get("unidiff") != "true" {
				t.Errorf("expected unidiff=true, got %q", q.Get("unidiff"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Diff(context.Background(), client, DiffInput{ProjectID: "42", SHA: "abc", Unidiff: true})
	if err != nil {
		t.Fatalf("Diff() unexpected error: %v", err)
	}
}

// TestGetRefs_WithType verifies the behavior of get refs with type.
func TestGetRefs_WithType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc/refs" {
			q := r.URL.Query()
			if q.Get("type") != "tag" {
				t.Errorf("expected type=tag, got %q", q.Get("type"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"type":"tag","name":"v1.0"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRefs(context.Background(), client, RefsInput{ProjectID: "42", SHA: "abc", Type: "tag"})
	if err != nil {
		t.Fatalf("GetRefs() unexpected error: %v", err)
	}
	if len(out.Refs) != 1 || out.Refs[0].Type != "tag" {
		t.Errorf("expected 1 tag ref, got %+v", out.Refs)
	}
}

// TestPostComment_Inline verifies the behavior of post comment inline.
func TestPostComment_Inline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/commits/abc/comments" {
			testutil.RespondJSON(w, http.StatusCreated, `{"note":"inline","path":"main.go","line":10,"line_type":"new","author":{"username":"dev"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PostComment(context.Background(), client, PostCommentInput{
		ProjectID: "42",
		SHA:       "abc",
		Note:      "inline",
		Path:      testFileMainGo,
		Line:      10,
		LineType:  "new",
	})
	if err != nil {
		t.Fatalf("PostComment() unexpected error: %v", err)
	}
	if out.Path != testFileMainGo {
		t.Errorf("Path = %q, want %q", out.Path, testFileMainGo)
	}
	if out.Line != 10 {
		t.Errorf("Line = %d, want 10", out.Line)
	}
}

// TestGetStatuses_WithFilters verifies the behavior of get statuses with filters.
func TestGetStatuses_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc/statuses" {
			q := r.URL.Query()
			if q.Get("ref") != "main" {
				t.Errorf("expected ref=main, got %q", q.Get("ref"))
			}
			if q.Get("stage") != "test" {
				t.Errorf("expected stage=test, got %q", q.Get("stage"))
			}
			if q.Get("all") != "true" {
				t.Errorf("expected all=true, got %q", q.Get("all"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"sha":"abc","ref":"main","status":"success","name":"lint"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatuses(context.Background(), client, StatusesInput{
		ProjectID:  "42",
		SHA:        "abc",
		Ref:        "main",
		Stage:      "test",
		Name:       "lint",
		PipelineID: 100,
		All:        true,
	})
	if err != nil {
		t.Fatalf("GetStatuses() unexpected error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("len(Statuses) = %d, want 1", len(out.Statuses))
	}
}

// TestSetStatus_WithAllOptions verifies the behavior of set status with all options.
func TestSetStatus_WithAllOptions(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/statuses/abc" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":3,"sha":"abc","ref":"main","status":"success","name":"deploy",
				"target_url":"https://ci.example.com","description":"OK","coverage":95.5,
				"allow_failure":false,"pipeline_id":100,
				"created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:01:00Z","finished_at":"2026-01-01T00:02:00Z",
				"author":{"username":"bot"}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetStatus(context.Background(), client, SetStatusInput{
		ProjectID:   "42",
		SHA:         "abc",
		State:       "success",
		Ref:         "main",
		Name:        "deploy",
		Context:     "ci/deploy",
		TargetURL:   testCIURL,
		Description: "OK",
		Coverage:    95.5,
		PipelineID:  100,
	})
	if err != nil {
		t.Fatalf("SetStatus() unexpected error: %v", err)
	}
	if out.Coverage != 95.5 {
		t.Errorf("Coverage = %f, want 95.5", out.Coverage)
	}
	if out.Author != "bot" {
		t.Errorf(fmtAuthorWant, out.Author, "bot")
	}
	if out.CreatedAt == "" {
		t.Error("CreatedAt is empty")
	}
	for _, want := range []string{"ref", "name", "context", "target_url", "description", "coverage", "pipeline_id"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// TestCherryPick_WithOptions verifies the behavior of cherry pick with options.
func TestCherryPick_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/commits/abc/cherry_pick" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":"cp1","short_id":"cp1","title":"cherry-picked"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CherryPick(context.Background(), client, CherryPickInput{
		ProjectID: "42",
		SHA:       "abc",
		Branch:    "release",
		DryRun:    true,
		Message:   "custom cherry-pick msg",
	})
	if err != nil {
		t.Fatalf("CherryPick() unexpected error: %v", err)
	}
	if out.ShortID != "cp1" {
		t.Errorf(fmtOutShortIDWant, out.ShortID, "cp1")
	}
}

// TestGetComments_EmptyProjectID verifies the behavior of get comments empty project i d.
func TestGetComments_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := GetComments(context.Background(), client, CommentsInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestGetStatuses_EmptyProjectID verifies the behavior of get statuses empty project i d.
func TestGetStatuses_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := GetStatuses(context.Background(), client, StatusesInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestGetRefs_EmptySHA verifies the behavior of get refs empty s h a.
func TestGetRefs_EmptySHA(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := GetRefs(context.Background(), client, RefsInput{SHA: "abc"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestCherryPick_EmptyBranch verifies the behavior of cherry pick empty branch.
func TestCherryPick_EmptyBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := CherryPick(context.Background(), client, CherryPickInput{SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRevert_EmptyBranch verifies the behavior of revert empty branch.
func TestRevert_EmptyBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Revert(context.Background(), client, RevertInput{SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Format*Markdown Tests
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
func TestFormatOutputMarkdown(t *testing.T) {
	c := Output{ShortID: "abc12", Title: "feat: init", AuthorName: "Alice", AuthorEmail: "a@t.com", CommittedDate: "2026-01-01", WebURL: "https://example.com"}
	md := FormatOutputMarkdown(c)
	if !strings.Contains(md, "abc12") {
		t.Error(errExpShortID)
	}
	if !strings.Contains(md, "feat: init") {
		t.Error("expected title")
	}
	if !strings.Contains(md, "Alice") {
		t.Error("expected author")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Commits:    []Output{{ShortID: "a1", Title: "feat: x", AuthorName: "A", CommittedDate: "2026-01-01"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "Commits (1)") {
		t.Errorf("expected header, got:\n%s", md)
	}
	if !strings.Contains(md, "a1") {
		t.Error(errExpShortID)
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Commits: nil, Pagination: toolutil.PaginationOutput{}}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No commits found") {
		t.Error("expected 'No commits found'")
	}
}

// TestFormatListMarkdown_ClickableCommitLinks verifies that commit short IDs
// in the list are rendered as clickable Markdown links [shortID](weburl).
func TestFormatListMarkdown_ClickableCommitLinks(t *testing.T) {
	out := ListOutput{
		Commits: []Output{
			{ShortID: "abc123", Title: "feat: x", AuthorName: "A",
				CommittedDate: "2026-01-01",
				WebURL:        "https://gitlab.example.com/commit/abc123"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "[abc123](https://gitlab.example.com/commit/abc123)") {
		t.Errorf("expected clickable commit link, got:\n%s", md)
	}
}

// TestFormatDetailMarkdown verifies the behavior of format detail markdown.
func TestFormatDetailMarkdown(t *testing.T) {
	c := DetailOutput{
		ShortID:     "abc",
		Title:       "feat: test",
		Message:     "feat: test\n\nLong description",
		AuthorName:  "Bob",
		AuthorEmail: "b@t.com",
		ParentIDs:   []string{"p1", "p2"},
		Stats:       &StatsOutput{Additions: 10, Deletions: 3, Total: 13},
		WebURL:      "https://example.com",
	}
	md := FormatDetailMarkdown(c)
	if !strings.Contains(md, "abc") {
		t.Error(errExpShortID)
	}
	if !strings.Contains(md, "p1, p2") {
		t.Error("expected parent IDs")
	}
	if !strings.Contains(md, "+10") {
		t.Error("expected additions stat")
	}
	if !strings.Contains(md, "Long description") {
		t.Error("expected message body")
	}
}

// TestFormatDetailMarkdown_Minimal verifies the behavior of format detail markdown minimal.
func TestFormatDetailMarkdown_Minimal(t *testing.T) {
	c := DetailOutput{ShortID: "x", Title: "t", Message: "t", WebURL: "u"}
	md := FormatDetailMarkdown(c)
	if strings.Contains(md, "Parents") {
		t.Error("should not show Parents when empty")
	}
	if strings.Contains(md, "Stats") {
		t.Error("should not show Stats when nil")
	}
	if strings.Contains(md, "### Message") {
		t.Error("should not show Message section when title == message")
	}
}

// TestFormatDiffMarkdown verifies the behavior of format diff markdown.
func TestFormatDiffMarkdown(t *testing.T) {
	out := DiffOutput{
		Diffs: []toolutil.DiffOutput{
			{OldPath: "a.go", NewPath: "a.go", NewFile: true},
			{OldPath: "b.go", NewPath: "b.go", DeletedFile: true},
			{OldPath: "c.go", NewPath: "d.go", RenamedFile: true},
			{OldPath: "e.go", NewPath: "e.go"},
		},
	}
	md := FormatDiffMarkdown(out)
	if !strings.Contains(md, "4 files") {
		t.Error("expected '4 files' header")
	}
	if !strings.Contains(md, "added") {
		t.Error("expected 'added' status")
	}
	if !strings.Contains(md, "deleted") {
		t.Error("expected 'deleted' status")
	}
	if !strings.Contains(md, "renamed") {
		t.Error("expected 'renamed' status")
	}
	if !strings.Contains(md, "modified") {
		t.Error("expected 'modified' status")
	}
}

// TestFormatDiffMarkdown_Empty verifies the behavior of format diff markdown empty.
func TestFormatDiffMarkdown_Empty(t *testing.T) {
	out := DiffOutput{Diffs: nil}
	md := FormatDiffMarkdown(out)
	if !strings.Contains(md, "No diffs found") {
		t.Error("expected 'No diffs found'")
	}
}

// TestFormatRefsMarkdown verifies the behavior of format refs markdown.
func TestFormatRefsMarkdown(t *testing.T) {
	out := RefsOutput{
		Refs: []RefOutput{
			{Type: "branch", Name: "main"},
			{Type: "tag", Name: "v1.0"},
		},
	}
	md := FormatRefsMarkdown(out)
	if !strings.Contains(md, "Commit Refs (2)") {
		t.Error("expected header with count")
	}
	if !strings.Contains(md, "main") {
		t.Error("expected 'main' branch")
	}
	if !strings.Contains(md, "v1.0") {
		t.Error("expected 'v1.0' tag")
	}
}

// TestFormatRefsMarkdown_Empty verifies the behavior of format refs markdown empty.
func TestFormatRefsMarkdown_Empty(t *testing.T) {
	out := RefsOutput{Refs: nil}
	md := FormatRefsMarkdown(out)
	if !strings.Contains(md, "No branch or tag refs found") {
		t.Error("expected 'No branch or tag refs found'")
	}
}

// TestFormatCommentsMarkdown verifies the behavior of format comments markdown.
func TestFormatCommentsMarkdown(t *testing.T) {
	out := CommentsOutput{
		Comments: []CommentOutput{
			{Author: "dev", Note: "LGTM", Path: testFileMainGo, Line: 10},
			{Author: "bot", Note: "OK"},
		},
	}
	md := FormatCommentsMarkdown(out)
	if !strings.Contains(md, "Commit Comments (2)") {
		t.Error("expected header with count")
	}
	if !strings.Contains(md, "LGTM") {
		t.Error("expected note text")
	}
	if !strings.Contains(md, "10") {
		t.Error("expected line number")
	}
	if !strings.Contains(md, "—") {
		t.Error("expected dash for empty path")
	}
}

// TestFormatCommentsMarkdown_Empty verifies the behavior of format comments markdown empty.
func TestFormatCommentsMarkdown_Empty(t *testing.T) {
	out := CommentsOutput{Comments: nil}
	md := FormatCommentsMarkdown(out)
	if !strings.Contains(md, "No commit comments found") {
		t.Error("expected 'No commit comments found'")
	}
}

// TestFormatCommentMarkdown verifies the behavior of format comment markdown.
func TestFormatCommentMarkdown(t *testing.T) {
	c := CommentOutput{Author: "dev", Note: "Nice!", Path: testFileMainGo, Line: 5}
	md := FormatCommentMarkdown(c)
	if !strings.Contains(md, "Commit Comment") {
		t.Error(errExpHeader)
	}
	if !strings.Contains(md, "Nice!") {
		t.Error("expected note")
	}
	if !strings.Contains(md, testFileMainGo) {
		t.Error("expected path")
	}
}

// TestFormatCommentMarkdown_NoPath verifies the behavior of format comment markdown no path.
func TestFormatCommentMarkdown_NoPath(t *testing.T) {
	c := CommentOutput{Author: "dev", Note: "OK"}
	md := FormatCommentMarkdown(c)
	if strings.Contains(md, "Path") {
		t.Error("should not show Path when empty")
	}
}

// TestFormatStatusesMarkdown verifies the behavior of format statuses markdown.
func TestFormatStatusesMarkdown(t *testing.T) {
	out := StatusesOutput{
		Statuses: []StatusOutput{
			{ID: 1, Status: "success", Name: "build", Ref: "main", Description: "OK"},
		},
	}
	md := FormatStatusesMarkdown(out)
	if !strings.Contains(md, "Commit Statuses (1)") {
		t.Error(errExpHeader)
	}
	if !strings.Contains(md, "success") {
		t.Error("expected status")
	}
}

// TestFormatStatusesMarkdown_Empty verifies the behavior of format statuses markdown empty.
func TestFormatStatusesMarkdown_Empty(t *testing.T) {
	out := StatusesOutput{Statuses: nil}
	md := FormatStatusesMarkdown(out)
	if !strings.Contains(md, "No commit statuses found") {
		t.Error("expected 'No commit statuses found'")
	}
}

// TestFormatStatusMarkdown verifies the behavior of format status markdown.
func TestFormatStatusMarkdown(t *testing.T) {
	s := StatusOutput{ID: 1, Status: "success", Name: "build", Ref: "main", Description: "Passed", TargetURL: testCIURL}
	md := FormatStatusMarkdown(s)
	if !strings.Contains(md, "Commit Status #1") {
		t.Error(errExpHeader)
	}
	if !strings.Contains(md, "Passed") {
		t.Error("expected description")
	}
	if !strings.Contains(md, testCIURL) {
		t.Error("expected target URL")
	}
}

// TestFormatStatusMarkdown_Minimal verifies the behavior of format status markdown minimal.
func TestFormatStatusMarkdown_Minimal(t *testing.T) {
	s := StatusOutput{ID: 2, Status: "pending", Name: "test", Ref: "dev"}
	md := FormatStatusMarkdown(s)
	if strings.Contains(md, "Description") {
		t.Error("should not show Description when empty")
	}
	if strings.Contains(md, "http") {
		t.Error("should not show URL when empty")
	}
}

// TestFormatMRsByCommitMarkdown verifies the behavior of format m rs by commit markdown.
func TestFormatMRsByCommitMarkdown(t *testing.T) {
	out := MRsByCommitOutput{
		MergeRequests: []BasicMROutput{
			{IID: 1, Title: "Feature", State: "merged", SourceBranch: "feat", TargetBranch: "main", Author: "dev"},
		},
	}
	md := FormatMRsByCommitMarkdown(out)
	if !strings.Contains(md, "Merge Requests for Commit (1)") {
		t.Error(errExpHeader)
	}
	if !strings.Contains(md, "Feature") {
		t.Error("expected MR title")
	}
	if !strings.Contains(md, "merged") {
		t.Error("expected state")
	}
}

// TestFormatMRsByCommitMarkdown_Empty verifies the behavior of format m rs by commit markdown empty.
func TestFormatMRsByCommitMarkdown_Empty(t *testing.T) {
	out := MRsByCommitOutput{MergeRequests: nil}
	md := FormatMRsByCommitMarkdown(out)
	if !strings.Contains(md, "No merge requests found") {
		t.Error("expected 'No merge requests found'")
	}
}

// TestFormatGPGSignatureMarkdown verifies the behavior of format g p g signature markdown.
func TestFormatGPGSignatureMarkdown(t *testing.T) {
	sig := GPGSignatureOutput{
		KeyID:              1,
		KeyPrimaryKeyID:    "ABC123",
		KeyUserName:        "Test",
		KeyUserEmail:       "t@t.com",
		VerificationStatus: "verified",
	}
	md := FormatGPGSignatureMarkdown(sig)
	if !strings.Contains(md, "GPG Signature") {
		t.Error(errExpHeader)
	}
	if !strings.Contains(md, "verified") {
		t.Error("expected verification status")
	}
	if !strings.Contains(md, "ABC123") {
		t.Error("expected key ID")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools Tests
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// commitMockResp holds a canned response for a mock commit endpoint.
type commitMockResp struct {
	status int
	body   string
	pgHdr  *testutil.PaginationHeaders
}

// commitRouteHandler returns an http.HandlerFunc that dispatches requests
// based on method+path to canned responses in the routes map.
func commitRouteHandler(routes map[string]commitMockResp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		resp, ok := routes[key]
		if !ok {
			http.NotFound(w, r)
			return
		}

		if resp.pgHdr != nil {
			testutil.RespondJSONWithPagination(w, resp.status, resp.body, *resp.pgHdr)
		} else if resp.body != "" {
			testutil.RespondJSON(w, resp.status, resp.body)
		} else {
			w.WriteHeader(resp.status)
		}
	}
}

// TestMCPRoundTrip_GetNotFound covers the 404 NotFoundResult path in
// gitlab_commit_get when the commit does not exist.
func TestMCPRoundTrip_GetNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Commit Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_commit_get",
		Arguments: map[string]any{"project_id": "42", "sha": "deadbeef"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestToOutput_DateFields covers the non-nil date and status branches
// in ToOutput (CommittedDate, AuthoredDate, CreatedAt, Status).
func TestToOutput_DateFields(t *testing.T) {
	now := time.Now()
	status := gl.BuildStateValue("success")
	c := &gl.Commit{
		ID:            "abc123",
		ShortID:       "abc",
		CommittedDate: &now,
		AuthoredDate:  &now,
		CreatedAt:     &now,
		Status:        &status,
	}
	out := ToOutput(c)
	if out.CommittedDate == "" {
		t.Error("expected non-empty CommittedDate")
	}
	if out.AuthoredDate == "" {
		t.Error("expected non-empty AuthoredDate")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.Status != "success" {
		t.Errorf("Status = %q, want %q", out.Status, "success")
	}
}

// TestCommentToOutput_AuthorNameFallback covers the fallback to Author.Name
// when Author.Username is empty.
func TestCommentToOutput_AuthorNameFallback(t *testing.T) {
	c := &gl.CommitComment{
		Note:   "test",
		Author: gl.Author{Name: "John"},
	}
	out := commentToOutput(c)
	if out.Author != "John" {
		t.Errorf("Author = %q, want %q", out.Author, "John")
	}
}

// TestCherryPick_400Error covers the 400 BadRequest error branch.
func TestCherryPick_400Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := CherryPick(context.Background(), client, CherryPickInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

// TestCherryPick_409Conflict covers the 409 Conflict error branch.
func TestCherryPick_409Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
	}))
	_, err := CherryPick(context.Background(), client, CherryPickInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

// TestRevert_400Error covers the 400 BadRequest error branch.
func TestRevert_400Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := Revert(context.Background(), client, RevertInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

// TestRevert_409Conflict covers the 409 Conflict error branch.
func TestRevert_409Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
	}))
	_, err := Revert(context.Background(), client, RevertInput{ProjectID: "42", SHA: "abc", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

// newCommitsMCPSession is an internal helper for the commits package.
func newCommitsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	commitJSON := `{"id":"c1","short_id":"c1","title":"t","author_name":"A","committed_date":"2026-01-01T00:00:00Z","web_url":"u"}`
	commitDetailJSON := `{"id":"c1","short_id":"c1","title":"t","message":"t","author_name":"A","committed_date":"2026-01-01T00:00:00Z","web_url":"u","parent_ids":["p1"],"stats":{"additions":1,"deletions":0,"total":1}}`
	pg1 := &testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}
	base := "/api/v4/projects/42/repository/commits"
	c1 := base + "/c1"

	routes := map[string]commitMockResp{
		"GET " + base:                          {http.StatusOK, `[` + commitJSON + `]`, pg1},
		"POST " + base:                         {http.StatusCreated, commitJSON, nil},
		"GET " + c1:                            {http.StatusOK, commitDetailJSON, nil},
		"GET " + c1 + "/diff":                  {http.StatusOK, `[{"old_path":"f.go","new_path":"f.go","diff":"","new_file":true}]`, pg1},
		"GET " + c1 + "/refs":                  {http.StatusOK, `[{"type":"branch","name":"main"}]`, pg1},
		"GET " + c1 + "/comments":              {http.StatusOK, `[{"note":"hi","author":{"username":"dev"}}]`, pg1},
		"POST " + c1 + "/comments":             {http.StatusCreated, `{"note":"posted","author":{"username":"dev"}}`, nil},
		"GET " + c1 + "/statuses":              {http.StatusOK, `[{"id":1,"sha":"c1","ref":"main","status":"success","name":"build"}]`, pg1},
		"POST /api/v4/projects/42/statuses/c1": {http.StatusCreated, `{"id":2,"sha":"c1","ref":"main","status":"success","name":"deploy"}`, nil},
		"GET " + c1 + "/merge_requests":        {http.StatusOK, `[{"id":1,"iid":1,"title":"MR","state":"merged","source_branch":"feat","target_branch":"main","web_url":"u","author":{"username":"dev"}}]`, nil},
		"POST " + c1 + "/cherry_pick":          {http.StatusCreated, commitJSON, nil},
		"POST " + c1 + "/revert":               {http.StatusCreated, commitJSON, nil},
		"GET " + c1 + "/signature":             {http.StatusOK, `{"gpg_key_id":1,"gpg_key_primary_keyid":"K1","gpg_key_user_name":"A","gpg_key_user_email":"a@t.com","verification_status":"verified"}`, nil},
	}

	client := testutil.NewTestClient(t, commitRouteHandler(routes))

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

// assertToolCallSuccess is an internal helper for the commits package.
func assertToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", name)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newCommitsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_commit_list", map[string]any{argProjectID: "42"}},
		{"gitlab_commit_create", map[string]any{argProjectID: "42", "branch": "main", "commit_message": "t", "actions": []any{map[string]any{"action": "create", "file_path": "f.go", "content": "x"}}}},
		{"gitlab_commit_get", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_diff", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_refs", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_comments", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_comment_create", map[string]any{argProjectID: "42", "sha": "c1", "note": "hi"}},
		{"gitlab_commit_statuses", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_status_set", map[string]any{argProjectID: "42", "sha": "c1", "state": "success"}},
		{"gitlab_commit_merge_requests", map[string]any{argProjectID: "42", "sha": "c1"}},
		{"gitlab_commit_cherry_pick", map[string]any{argProjectID: "42", "sha": "c1", "branch": "main"}},
		{"gitlab_commit_revert", map[string]any{argProjectID: "42", "sha": "c1", "branch": "main"}},
		{"gitlab_commit_signature", map[string]any{argProjectID: "42", "sha": "c1"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallSuccess(t, session, ctx, tt.name, tt.args)
		})
	}
}

// TestCommitGet_EmbedsCanonicalResource asserts gitlab_commit_get attaches
// an EmbeddedResource block with URI gitlab://project/{id}/commit/{sha}.
func TestCommitGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":"abc123","short_id":"abc123","title":"T","message":"M","author_name":"A","author_email":"a@b","authored_date":"2026-01-01T00:00:00Z","committed_date":"2026-01-01T00:00:00Z","web_url":"https://gitlab.example.com/g/p/-/commit/abc123"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/commits/abc123" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "sha": "abc123"}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_commit_get", args, "gitlab://project/42/commit/abc123", toolutil.EnableEmbeddedResources)
}

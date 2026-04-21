// mergerequests_test.go contains unit tests for GitLab merge request CRUD
// operations (create, get, list, update, merge, approve, unapprove). Tests use
// httptest to mock the GitLab API and verify both success and error paths.
package mergerequests

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Test constants for merge request endpoint paths and reusable values.
const (
	pathMRs                = "/api/v4/projects/42/merge_requests"
	pathMR1                = pathMRs + "/1"
	testBranchFeatureLogin = "feature/login"
	fmtStateWant           = "out.State = %q, want %q"
	fmtMRGetErr            = "Get() unexpected error: %v"
	fmtMRListErr           = "List() unexpected error: %v"
	fmtAuthorWant          = "out.Author = %q, want %q"
	fmtIIDWant1            = "out.IID = %d, want 1"

	// mrJSONRich is a full MR JSON with all enriched fields populated.
	mrJSONRich = `{
		"id":100,"iid":1,
		"title":"feat: add login","description":"Implements login screen",
		"state":"opened",
		"source_branch":"feature/login","target_branch":"develop",
		"web_url":"https://gitlab.example.com/project/merge_requests/1",
		"detailed_merge_status":"can_be_merged",
		"draft":true,
		"has_conflicts":true,
		"blocking_discussions_resolved":false,
		"author":{"username":"alice"},
		"assignees":[{"username":"bob"},{"username":"carol"}],
		"reviewers":[{"username":"dave"}],
		"labels":["bug","enhancement"],
		"created_at":"2026-02-01T10:00:00Z",
		"updated_at":"2026-02-15T14:30:00Z",
		"merged_at":null,
		"user_notes_count":5,
		"sha":"abc123def456",
		"milestone":{"title":"v2.0"},
		"squash":true,
		"task_completion_status":{"count":4,"completed_count":2}
	}`

	// mrJSONMerged is a merged MR JSON with merged_at populated.
	mrJSONMerged = `{
		"id":200,"iid":2,
		"title":"fix: auth bug","description":"Fixes OAuth redirect",
		"state":"merged",
		"source_branch":"fix/auth","target_branch":"main",
		"web_url":"https://gitlab.example.com/project/merge_requests/2",
		"detailed_merge_status":"merged",
		"draft":false,
		"has_conflicts":false,
		"blocking_discussions_resolved":true,
		"author":{"username":"eve"},
		"assignees":[],
		"reviewers":[{"username":"frank"}],
		"labels":[],
		"created_at":"2026-01-10T09:00:00Z",
		"updated_at":"2026-01-12T16:00:00Z",
		"merged_at":"2026-01-12T16:00:00Z",
		"user_notes_count":12
	}`

	// mrJSONMinimalMR is a minimal MR JSON with no optional fields populated.
	mrJSONMinimalMR = `{
		"id":300,"iid":3,
		"title":"chore: cleanup","description":"",
		"state":"opened",
		"source_branch":"chore/cleanup","target_branch":"main",
		"web_url":"https://gitlab.example.com/project/merge_requests/3",
		"detailed_merge_status":"can_be_merged"
	}`
)

// TestMRCreate_Success verifies that Create correctly creates a merge request
// with the minimum required fields. The mock returns a 201 response and the
// test asserts IID, state, and source branch match expected values.
func TestMRCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMRs {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":100,"iid":1,"title":"feat: add login","description":"Implements login screen","state":"opened","source_branch":"feature/login","target_branch":"develop","web_url":"https://gitlab.example.com/project/merge_requests/1","merge_status":"can_be_merged"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		SourceBranch: testBranchFeatureLogin,
		TargetBranch: "develop",
		Title:        "feat: add login",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant1, out.IID)
	}
	if out.State != "opened" {
		t.Errorf(fmtStateWant, out.State, "opened")
	}
	if out.SourceBranch != testBranchFeatureLogin {
		t.Errorf("out.SourceBranch = %q, want %q", out.SourceBranch, testBranchFeatureLogin)
	}
}

// TestMRCreateSourceBranch_NotFound verifies that Create returns an error
// when the GitLab API responds with 422 for an invalid source branch.
func TestMRCreateSourceBranch_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":{"source_branch":["is invalid"]}}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		SourceBranch: "nonexistent",
		TargetBranch: "develop",
		Title:        "test",
	})
	if err == nil {
		t.Fatal("Create() expected error for invalid source branch, got nil")
	}
}

// TestMRGet_Success verifies that Get retrieves a single merge request by
// its internal ID. The mock returns a 200 response and the test asserts the
// IID matches.
func TestMRGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":100,"iid":1,"title":"feat: add login","description":"","state":"opened","source_branch":"feature/login","target_branch":"develop","web_url":"https://gitlab.example.com/project/merge_requests/1","merge_status":"can_be_merged"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf(fmtMRGetErr, err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant1, out.IID)
	}
}

// TestMRGet_NotFound verifies that Get returns an error when the GitLab API
// responds with 404 for a non-existent merge request.
func TestMRGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 9999})
	if err == nil {
		t.Fatal("Get() expected error for non-existent MR, got nil")
	}
}

// TestMRList_ByState verifies that List returns merge requests filtered by
// state. The mock returns two opened merge requests and the test asserts the
// correct count.
func TestMRList_ByState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMRs {
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.AssertQueryParam(t, r, "state", "opened")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"iid":1,"title":"MR 1","description":"","state":"opened","source_branch":"feature/a","target_branch":"main","web_url":"https://gitlab.example.com/mr/1","merge_status":"can_be_merged"},{"id":101,"iid":2,"title":"MR 2","description":"","state":"opened","source_branch":"feature/b","target_branch":"main","web_url":"https://gitlab.example.com/mr/2","merge_status":"cannot_be_merged"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, State: "opened"})
	if err != nil {
		t.Fatalf(fmtMRListErr, err)
	}
	if len(out.MergeRequests) != 2 {
		t.Errorf("len(out.MergeRequests) = %d, want 2", len(out.MergeRequests))
	}
}

// TestMRList_PaginationQueryParamsAndMetadata verifies that List forwards
// page and per_page query parameters to the GitLab API and correctly parses
// pagination metadata from response headers.
func TestMRList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMRs {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("per_page"); got != "10" {
				t.Errorf("query param per_page = %q, want %q", got, "10")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":200,"iid":11,"title":"MR 11","description":"","state":"opened","source_branch":"feat/x","target_branch":"main","web_url":"https://gitlab.example.com/mr/11","merge_status":"can_be_merged"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "25", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID, PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10}})
	if err != nil {
		t.Fatalf(fmtMRListErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 25 {
		t.Errorf("Pagination.TotalItems = %d, want 25", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("Pagination.TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("Pagination.NextPage = %d, want 3", out.Pagination.NextPage)
	}
}

// TestMRUpdate_Close verifies that Update transitions a merge request to the
// closed state when StateEvent is set to "close". The mock returns a merge
// request with state "closed".
func TestMRUpdate_Close(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":100,"iid":1,"title":"feat: add login","description":"","state":"closed","source_branch":"feature/login","target_branch":"develop","web_url":"https://gitlab.example.com/project/merge_requests/1","merge_status":"cannot_be_merged"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  testProjectID,
		MRIID:      1,
		StateEvent: "close",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.State != "closed" {
		t.Errorf(fmtStateWant, out.State, "closed")
	}
}

// TestMRMerge_Success verifies that Merge successfully merges a merge request.
// The mock returns a 200 response with state "merged" and the test asserts the
// output state.
func TestMRMerge_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/merge" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":100,"iid":1,"title":"feat: add login","description":"","state":"merged","source_branch":"feature/login","target_branch":"develop","web_url":"https://gitlab.example.com/project/merge_requests/1","merge_status":"merged"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Merge(context.Background(), client, MergeInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.State != "merged" {
		t.Errorf(fmtStateWant, out.State, "merged")
	}
}

// TestMRMerge_Conflicts verifies that Merge returns an error when the GitLab
// API responds with 405, indicating the merge request has conflicts.
func TestMRMerge_Conflicts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
	}))

	_, err := Merge(context.Background(), client, MergeInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Merge() expected error for conflict, got nil")
	}
}

// TestMRApprove_Success verifies that Approve approves a merge request and
// returns the correct approval state. The mock returns approved=true with one
// approver.
func TestMRApprove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approve" {
			testutil.RespondJSON(w, http.StatusCreated, `{"approvals_required":1,"approved_by":[{"user":{"username":"jmrplens"}}],"approved":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Approve(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Approve() unexpected error: %v", err)
	}
	if !out.Approved {
		t.Error("out.Approved = false, want true")
	}
}

// TestMRUnapprove_Success verifies that Unapprove removes the current user's
// approval. The mock returns 204 No Content and the test asserts no error.
func TestMRUnapprove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/merge_requests/1/unapprove" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := Unapprove(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1}); err != nil {
		t.Errorf("Unapprove() unexpected error: %v", err)
	}
}

// TestMRCommits_Success verifies that Commits returns a list of commits for a MR.
func TestMRCommits_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1/commits" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":"abc123","short_id":"abc123d","title":"feat: add feature","author_name":"Test","committed_date":"2026-03-01T10:00:00Z","web_url":"https://gitlab.example.com/-/commit/abc123"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Commits(context.Background(), client, CommitsInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Commits() unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	if out.Commits[0].Title != "feat: add feature" {
		t.Errorf("Commits[0].Title = %q, want %q", out.Commits[0].Title, "feat: add feature")
	}
}

// TestMRCommits_EmptyProjectID verifies Commits returns an error for empty project_id.
func TestMRCommits_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := Commits(context.Background(), client, CommitsInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestMRPipelines_Success verifies that Pipelines returns pipelines for a MR.
func TestMRPipelines_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1/pipelines" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":10,"iid":10,"project_id":42,"status":"success","ref":"feature","sha":"abc123","web_url":"https://gitlab.example.com/-/pipelines/10","created_at":"2026-03-01T10:00:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Pipelines(context.Background(), client, PipelinesInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Pipelines() unexpected error: %v", err)
	}
	if len(out.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(out.Pipelines))
	}
	if out.Pipelines[0].Status != "success" {
		t.Errorf("Pipelines[0].Status = %q, want %q", out.Pipelines[0].Status, "success")
	}
}

// TestMRPipelines_EmptyProjectID verifies Pipelines returns an error for empty project_id.
func TestMRPipelines_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := Pipelines(context.Background(), client, PipelinesInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestMRDelete_Success verifies that Delete removes a merge request.
func TestMRDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathMR1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestMRDelete_EmptyProjectID verifies Delete returns an error for empty project_id.
func TestMRDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestMRRebase_Success verifies that Rebase triggers a rebase and returns in-progress status.
func TestMRRebase_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/rebase" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Rebase(context.Background(), client, RebaseInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Rebase() unexpected error: %v", err)
	}
	if !out.RebaseInProgress {
		t.Error("out.RebaseInProgress = false, want true")
	}
}

// TestMRRebase_EmptyProjectID verifies Rebase returns an error for empty project_id.
func TestMRRebase_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	_, err := Rebase(context.Background(), client, RebaseInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// mrGetEnrichedSetup creates a test client with a rich MR mock and calls Get,
// returning the output for assertion in individual sub-tests.
func mrGetEnrichedSetup(t *testing.T) Output {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR1 {
			testutil.RespondJSON(w, http.StatusOK, mrJSONRich)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf(fmtMRGetErr, err)
	}
	return out
}

// TestMRGetEnrichedDraftAnd_Conflicts verifies the behavior of m r get enriched draft and conflicts.
func TestMRGetEnrichedDraftAnd_Conflicts(t *testing.T) {
	out := mrGetEnrichedSetup(t)
	if !out.Draft {
		t.Error("out.Draft = false, want true")
	}
	if !out.HasConflicts {
		t.Error("out.HasConflicts = false, want true")
	}
	if out.BlockingDiscussionsResolved {
		t.Error("out.BlockingDiscussionsResolved = true, want false")
	}
}

// TestMRGet_EnrichedPeople verifies the behavior of m r get enriched people.
func TestMRGet_EnrichedPeople(t *testing.T) {
	out := mrGetEnrichedSetup(t)
	if out.Author != "alice" {
		t.Errorf(fmtAuthorWant, out.Author, "alice")
	}
	if len(out.Assignees) != 2 {
		t.Fatalf("len(out.Assignees) = %d, want 2", len(out.Assignees))
	}
	if out.Assignees[0] != "bob" || out.Assignees[1] != "carol" {
		t.Errorf("out.Assignees = %v, want [bob carol]", out.Assignees)
	}
	if len(out.Reviewers) != 1 || out.Reviewers[0] != "dave" {
		t.Errorf("out.Reviewers = %v, want [dave]", out.Reviewers)
	}
}

// TestMRGet_EnrichedLabels verifies the behavior of m r get enriched labels.
func TestMRGet_EnrichedLabels(t *testing.T) {
	out := mrGetEnrichedSetup(t)
	if len(out.Labels) != 2 {
		t.Fatalf("len(out.Labels) = %d, want 2", len(out.Labels))
	}
	if out.Labels[0] != "bug" || out.Labels[1] != "enhancement" {
		t.Errorf("out.Labels = %v, want [bug enhancement]", out.Labels)
	}
}

// TestMRGet_EnrichedTimestamps verifies the behavior of m r get enriched timestamps.
func TestMRGet_EnrichedTimestamps(t *testing.T) {
	out := mrGetEnrichedSetup(t)
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt should not be empty")
	}
	if out.UpdatedAt == "" {
		t.Error("out.UpdatedAt should not be empty")
	}
	if out.MergedAt != "" {
		t.Errorf("out.MergedAt = %q, want empty for non-merged MR", out.MergedAt)
	}
}

// TestMRGet_EnrichedNotesCount verifies the behavior of m r get enriched notes count.
func TestMRGet_EnrichedNotesCount(t *testing.T) {
	out := mrGetEnrichedSetup(t)
	if out.UserNotesCount != 5 {
		t.Errorf("out.UserNotesCount = %d, want 5", out.UserNotesCount)
	}
}

// TestMRGet_MergedTimestamps verifies that merged_at is populated for merged MRs.
func TestMRGet_MergedTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/2" {
			testutil.RespondJSON(w, http.StatusOK, mrJSONMerged)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 2})
	if err != nil {
		t.Fatalf(fmtMRGetErr, err)
	}
	if out.State != "merged" {
		t.Errorf(fmtStateWant, out.State, "merged")
	}
	if out.MergedAt == "" {
		t.Error("out.MergedAt should not be empty for merged MR")
	}
	if !out.BlockingDiscussionsResolved {
		t.Error("out.BlockingDiscussionsResolved = false, want true")
	}
	if out.Draft {
		t.Error("out.Draft = true, want false")
	}
	if out.Author != "eve" {
		t.Errorf(fmtAuthorWant, out.Author, "eve")
	}
	if out.UserNotesCount != 12 {
		t.Errorf("out.UserNotesCount = %d, want 12", out.UserNotesCount)
	}
}

// TestMRGet_MinimalFields verifies that enriched fields gracefully default to
// zero values when the API response omits them. This confirms omitempty safety.
func TestMRGet_MinimalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/3" {
			testutil.RespondJSON(w, http.StatusOK, mrJSONMinimalMR)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 3})
	if err != nil {
		t.Fatalf(fmtMRGetErr, err)
	}
	if out.Draft {
		t.Error("out.Draft should default to false")
	}
	if out.HasConflicts {
		t.Error("out.HasConflicts should default to false")
	}
	if out.Author != "" {
		t.Errorf("out.Author = %q, want empty for minimal MR", out.Author)
	}
	if len(out.Assignees) != 0 {
		t.Errorf("len(out.Assignees) = %d, want 0", len(out.Assignees))
	}
	if len(out.Reviewers) != 0 {
		t.Errorf("len(out.Reviewers) = %d, want 0", len(out.Reviewers))
	}
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty for minimal MR", out.CreatedAt)
	}
	if out.UserNotesCount != 0 {
		t.Errorf("out.UserNotesCount = %d, want 0", out.UserNotesCount)
	}
}

// TestMRList_EnrichedFields verifies that List populates enriched fields from
// BasicMergeRequest including draft, author, labels, and timestamps.
func TestMRList_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMRs {
			testutil.RespondJSON(w, http.StatusOK, `[`+mrJSONRich+`,`+mrJSONMerged+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtMRListErr, err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("len(out.MergeRequests) = %d, want 2", len(out.MergeRequests))
	}

	// First MR (draft with conflicts)
	mr1 := out.MergeRequests[0]
	if !mr1.Draft {
		t.Error("MR[0].Draft = false, want true")
	}
	if mr1.Author != "alice" {
		t.Errorf("MR[0].Author = %q, want %q", mr1.Author, "alice")
	}
	if len(mr1.Labels) != 2 {
		t.Errorf("len(MR[0].Labels) = %d, want 2", len(mr1.Labels))
	}
	if mr1.CreatedAt == "" {
		t.Error("MR[0].CreatedAt should not be empty")
	}
	if mr1.HasConflicts != true {
		t.Error("MR[0].HasConflicts = false, want true")
	}
	if len(mr1.Assignees) != 2 {
		t.Errorf("len(MR[0].Assignees) = %d, want 2", len(mr1.Assignees))
	}
	if len(mr1.Reviewers) != 1 {
		t.Errorf("len(MR[0].Reviewers) = %d, want 1", len(mr1.Reviewers))
	}

	// Second MR (merged)
	mr2 := out.MergeRequests[1]
	if mr2.State != "merged" {
		t.Errorf(fmtStateWant, mr2.State, "merged")
	}
	if mr2.MergedAt == "" {
		t.Error("MR[1].MergedAt should not be empty for merged MR")
	}
	if mr2.Author != "eve" {
		t.Errorf("MR[1].Author = %q, want %q", mr2.Author, "eve")
	}
}

// TestMRCreate_EnrichedFields verifies that Create returns all enriched fields
// when the API response includes them.
func TestMRCreate_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMRs {
			testutil.RespondJSON(w, http.StatusCreated, mrJSONRich)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		SourceBranch: testBranchFeatureLogin,
		TargetBranch: "develop",
		Title:        "feat: add login",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Author != "alice" {
		t.Errorf(fmtAuthorWant, out.Author, "alice")
	}
	if !out.Draft {
		t.Error("out.Draft = false, want true")
	}
	if len(out.Labels) != 2 {
		t.Errorf("len(out.Labels) = %d, want 2", len(out.Labels))
	}
	if out.UserNotesCount != 5 {
		t.Errorf("out.UserNotesCount = %d, want 5", out.UserNotesCount)
	}
}

// TestMRGet_SuccessPipelineFields verifies that Get maps pipeline fields
// (PipelineID, PipelineWebURL, PipelineName) from the API response.
func TestMRGet_SuccessPipelineFields(t *testing.T) {
	mrWithPipeline := `{
		"id":400,"iid":4,
		"title":"feat: pipeline test","description":"",
		"state":"opened",
		"source_branch":"feature/pipe","target_branch":"main",
		"web_url":"https://gitlab.example.com/project/merge_requests/4",
		"detailed_merge_status":"can_be_merged",
		"pipeline":{"id":100,"web_url":"https://gitlab.example.com/pipelines/100","name":"my-pipeline"}
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/4" {
			testutil.RespondJSON(w, http.StatusOK, mrWithPipeline)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 4})
	if err != nil {
		t.Fatalf(fmtMRGetErr, err)
	}
	if out.PipelineID != 100 {
		t.Errorf("out.PipelineID = %d, want 100", out.PipelineID)
	}
	if out.PipelineWebURL != "https://gitlab.example.com/pipelines/100" {
		t.Errorf("out.PipelineWebURL = %q, want %q", out.PipelineWebURL, "https://gitlab.example.com/pipelines/100")
	}
	if out.PipelineName != "my-pipeline" {
		t.Errorf("out.PipelineName = %q, want %q", out.PipelineName, "my-pipeline")
	}
}

// TestPrefixAt verifies username @mention formatting.
func TestPrefixAt(t *testing.T) {
	got := prefixAt([]string{"alice", "bob"})
	want := []string{"@alice", "@bob"}
	if len(got) != len(want) {
		t.Fatalf("prefixAt length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prefixAt[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// ListGlobal tests
// ---------------------------------------------------------------------------.

const pathGlobalMRs = "/api/v4/merge_requests"

// TestMRListGlobal_Success verifies the behavior of m r list global success.
func TestMRListGlobal_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGlobalMRs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":100,"iid":1,"title":"global mr","state":"opened","source_branch":"a","target_branch":"b","web_url":"http://x","project_id":10}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGlobal(context.Background(), client, ListGlobalInput{State: "opened"})
	if err != nil {
		t.Fatalf("ListGlobal() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("ListGlobal() returned %d MRs, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "global mr" {
		t.Errorf("MR title = %q, want %q", out.MergeRequests[0].Title, "global mr")
	}
}

// TestMRListGlobal_Error verifies the behavior of m r list global error.
func TestMRListGlobal_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"error":"server error"}`)
	}))

	_, err := ListGlobal(context.Background(), client, ListGlobalInput{})
	if err == nil {
		t.Fatal("ListGlobal() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListGroup tests
// ---------------------------------------------------------------------------.

const pathGroupMRs = "/api/v4/groups/99/merge_requests"

// TestMRListGroup_Success verifies the behavior of m r list group success.
func TestMRListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMRs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":200,"iid":2,"title":"group mr","state":"merged","source_branch":"c","target_branch":"d","web_url":"http://y","project_id":20}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "99", State: "merged"})
	if err != nil {
		t.Fatalf("ListGroup() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("ListGroup() returned %d MRs, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].State != "merged" {
		t.Errorf(fmtStateWant, out.MergeRequests[0].State, "merged")
	}
}

// TestMRListGroup_MissingGroupID verifies the behavior of m r list group missing group i d.
func TestMRListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("ListGroup() expected error for missing group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Participants tests
// ---------------------------------------------------------------------------.

// TestMRParticipants_Success verifies the behavior of m r participants success.
func TestMRParticipants_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+"/participants" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","name":"Alice","state":"active","web_url":"http://alice"},{"id":2,"username":"bob","name":"Bob","state":"active","web_url":"http://bob"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Participants(context.Background(), client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Participants() unexpected error: %v", err)
	}
	if len(out.Participants) != 2 {
		t.Fatalf("Participants() returned %d, want 2", len(out.Participants))
	}
	if out.Participants[0].Username != "alice" {
		t.Errorf("participant[0].Username = %q, want %q", out.Participants[0].Username, "alice")
	}
}

// TestMRParticipants_MissingProject verifies the behavior of m r participants missing project.
func TestMRParticipants_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Participants(context.Background(), client, ParticipantsInput{MRIID: 1})
	if err == nil {
		t.Fatal("Participants() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Reviewers tests
// ---------------------------------------------------------------------------.

// TestMRReviewers_Success verifies the behavior of m r reviewers success.
func TestMRReviewers_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+"/reviewers" {
			testutil.RespondJSON(w, http.StatusOK, `[{"user":{"id":10,"username":"carol","name":"Carol","state":"active","web_url":"http://carol"},"state":"reviewed","created_at":"2026-03-01T10:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Reviewers(context.Background(), client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Reviewers() unexpected error: %v", err)
	}
	if len(out.Reviewers) != 1 {
		t.Fatalf("Reviewers() returned %d, want 1", len(out.Reviewers))
	}
	if out.Reviewers[0].Username != "carol" {
		t.Errorf("reviewer[0].Username = %q, want %q", out.Reviewers[0].Username, "carol")
	}
	if out.Reviewers[0].Review != "reviewed" {
		t.Errorf("reviewer[0].Review = %q, want %q", out.Reviewers[0].Review, "reviewed")
	}
}

// TestMRReviewers_Error verifies the behavior of m r reviewers error.
func TestMRReviewers_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"error":"forbidden"}`)
	}))

	_, err := Reviewers(context.Background(), client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Reviewers() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreatePipeline tests
// ---------------------------------------------------------------------------.

// TestMRCreatePipeline_Success verifies the behavior of m r create pipeline success.
func TestMRCreatePipeline_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/pipelines" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":500,"iid":10,"project_id":42,"status":"pending","source":"merge_request_event","ref":"feature/login","sha":"abc123","web_url":"http://pipe/500","created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreatePipeline(context.Background(), client, CreatePipelineInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("CreatePipeline() unexpected error: %v", err)
	}
	if out.ID != 500 {
		t.Errorf("out.ID = %d, want 500", out.ID)
	}
	if out.Status != "pending" {
		t.Errorf("out.Status = %q, want %q", out.Status, "pending")
	}
}

// TestMRCreatePipeline_MissingProject verifies the behavior of m r create pipeline missing project.
func TestMRCreatePipeline_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreatePipeline(context.Background(), client, CreatePipelineInput{MRIID: 1})
	if err == nil {
		t.Fatal("CreatePipeline() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// IssuesClosed tests
// ---------------------------------------------------------------------------.

// TestMRIssuesClosed_Success verifies the behavior of m r issues closed success.
func TestMRIssuesClosed_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+"/closes_issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":10,"iid":5,"title":"Bug fix","state":"opened","author":{"username":"alice"},"labels":["bug"],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","web_url":"http://issue/5","project_id":42}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := IssuesClosed(context.Background(), client, IssuesClosedInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("IssuesClosed() unexpected error: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("IssuesClosed() returned %d issues, want 1", len(out.Issues))
	}
	if out.Issues[0].IID != 5 {
		t.Errorf("issue IID = %d, want 5", out.Issues[0].IID)
	}
	if out.Issues[0].Title != "Bug fix" {
		t.Errorf("issue Title = %q, want %q", out.Issues[0].Title, "Bug fix")
	}
}

// TestMRIssuesClosed_Error verifies the behavior of m r issues closed error.
func TestMRIssuesClosed_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"error":"not found"}`)
	}))

	_, err := IssuesClosed(context.Background(), client, IssuesClosedInput{ProjectID: testProjectID, MRIID: 999})
	if err == nil {
		t.Fatal("IssuesClosed() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// CancelAutoMerge tests
// ---------------------------------------------------------------------------.

// TestMRCancelAutoMerge_Success verifies the behavior of m r cancel auto merge success.
func TestMRCancelAutoMerge_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/cancel_merge_when_pipeline_succeeds" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":100,"iid":1,"title":"feat: add login","state":"opened","source_branch":"feature/login","target_branch":"develop","web_url":"https://gitlab.example.com/project/merge_requests/1","detailed_merge_status":"can_be_merged","merge_when_pipeline_succeeds":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CancelAutoMerge(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("CancelAutoMerge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant1, out.IID)
	}
	if out.MergeWhenPipelineSucceeds {
		t.Error("out.MergeWhenPipelineSucceeds should be false after cancel")
	}
}

// TestMR_CancelAutoMergeNotAutoMerging verifies the behavior of m r cancel auto merge not auto merging.
func TestMR_CancelAutoMergeNotAutoMerging(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"Method Not Allowed"}`)
	}))

	_, err := CancelAutoMerge(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("CancelAutoMerge() expected error for non-auto-merging MR, got nil")
	}
}

// TestMRCancelAutoMerge_MissingProject verifies the behavior of m r cancel auto merge missing project.
func TestMRCancelAutoMerge_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CancelAutoMerge(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal("CancelAutoMerge() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Subscribe / Unsubscribe tests
// ---------------------------------------------------------------------------.

// TestMRSubscribe_Success verifies the behavior of m r subscribe success.
func TestMRSubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/subscribe" {
			testutil.RespondJSON(w, http.StatusOK, `{"iid":1,"title":"Test MR","state":"opened","subscribed":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Subscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	if !out.Subscribed {
		t.Error("Subscribe() out.Subscribed = false, want true")
	}
}

// TestMRSubscribe_MissingProject verifies the behavior of m r subscribe missing project.
func TestMRSubscribe_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Subscribe(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal("Subscribe() expected error for missing project_id")
	}
}

// TestMRUnsubscribe_Success verifies the behavior of m r unsubscribe success.
func TestMRUnsubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/unsubscribe" {
			testutil.RespondJSON(w, http.StatusOK, `{"iid":1,"title":"Test MR","state":"opened","subscribed":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Unsubscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Unsubscribe() unexpected error: %v", err)
	}
	if out.Subscribed {
		t.Error("Unsubscribe() out.Subscribed = true, want false")
	}
}

// TestMRUnsubscribe_MissingProject verifies the behavior of m r unsubscribe missing project.
func TestMRUnsubscribe_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Unsubscribe(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal("Unsubscribe() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// Time Tracking tests
// ---------------------------------------------------------------------------.

const timeStatsResponse = `{"human_time_estimate":"3h","human_total_time_spent":"1h30m","time_estimate":10800,"total_time_spent":5400}`

// TestMRSetTimeEstimate_Success verifies the behavior of m r set time estimate success.
func TestMRSetTimeEstimate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/time_estimate" {
			testutil.RespondJSON(w, http.StatusOK, timeStatsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, MRIID: 1, Duration: "3h"})
	if err != nil {
		t.Fatalf("SetTimeEstimate() unexpected error: %v", err)
	}
	if out.TimeEstimate != 10800 {
		t.Errorf("TimeEstimate = %d, want 10800", out.TimeEstimate)
	}
	if out.HumanTimeEstimate != "3h" {
		t.Errorf("HumanTimeEstimate = %q, want %q", out.HumanTimeEstimate, "3h")
	}
}

// TestMRSetTimeEstimate_MissingDuration verifies the behavior of m r set time estimate missing duration.
func TestMRSetTimeEstimate_MissingDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("SetTimeEstimate() expected error for missing duration")
	}
}

// TestMRResetTimeEstimate_Success verifies the behavior of m r reset time estimate success.
func TestMRResetTimeEstimate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/reset_time_estimate" {
			testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate":"","human_total_time_spent":"1h30m","time_estimate":0,"total_time_spent":5400}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ResetTimeEstimate(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("ResetTimeEstimate() unexpected error: %v", err)
	}
	if out.TimeEstimate != 0 {
		t.Errorf("TimeEstimate = %d, want 0", out.TimeEstimate)
	}
}

// TestMRAddSpentTime_Success verifies the behavior of m r add spent time success.
func TestMRAddSpentTime_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/add_spent_time" {
			testutil.RespondJSON(w, http.StatusCreated, timeStatsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, MRIID: 1, Duration: "1h30m", Summary: "code review"})
	if err != nil {
		t.Fatalf("AddSpentTime() unexpected error: %v", err)
	}
	if out.TotalTimeSpent != 5400 {
		t.Errorf("TotalTimeSpent = %d, want 5400", out.TotalTimeSpent)
	}
}

// TestMRAddSpentTime_MissingDuration verifies the behavior of m r add spent time missing duration.
func TestMRAddSpentTime_MissingDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("AddSpentTime() expected error for missing duration")
	}
}

// TestMRResetSpentTime_Success verifies the behavior of m r reset spent time success.
func TestMRResetSpentTime_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/reset_spent_time" {
			testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate":"3h","human_total_time_spent":"","time_estimate":10800,"total_time_spent":0}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ResetSpentTime(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("ResetSpentTime() unexpected error: %v", err)
	}
	if out.TotalTimeSpent != 0 {
		t.Errorf("TotalTimeSpent = %d, want 0", out.TotalTimeSpent)
	}
}

// TestMRGetTimeStats_Success verifies the behavior of m r get time stats success.
func TestMRGetTimeStats_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+"/time_stats" {
			testutil.RespondJSON(w, http.StatusOK, timeStatsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetTimeStats(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("GetTimeStats() unexpected error: %v", err)
	}
	if out.TimeEstimate != 10800 {
		t.Errorf("TimeEstimate = %d, want 10800", out.TimeEstimate)
	}
	if out.TotalTimeSpent != 5400 {
		t.Errorf("TotalTimeSpent = %d, want 5400", out.TotalTimeSpent)
	}
}

// TestMRGetTimeStats_MissingProject verifies the behavior of m r get time stats missing project.
func TestMRGetTimeStats_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetTimeStats(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal("GetTimeStats() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// MRIID validation tests — ensures all functions reject mr_iid <= 0
// ---------------------------------------------------------------------------.

// TestMRIIDRequired_Validation verifies that all functions requiring mr_iid
// return an error when MRIID is 0 (the zero value when the parameter is
// missing or has the wrong name in meta-tool dispatch).
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt(testProjectID)
	const wantSubstr = "mr_iid"

	t.Run("Get", func(t *testing.T) {
		_, err := Get(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Update", func(t *testing.T) {
		_, err := Update(ctx, client, UpdateInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Merge", func(t *testing.T) {
		_, err := Merge(ctx, client, MergeInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Approve", func(t *testing.T) {
		_, err := Approve(ctx, client, ApproveInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Unapprove", func(t *testing.T) {
		err := Unapprove(ctx, client, ApproveInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Commits", func(t *testing.T) {
		_, err := Commits(ctx, client, CommitsInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Pipelines", func(t *testing.T) {
		_, err := Pipelines(ctx, client, PipelinesInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Delete", func(t *testing.T) {
		err := Delete(ctx, client, DeleteInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Rebase", func(t *testing.T) {
		_, err := Rebase(ctx, client, RebaseInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Participants", func(t *testing.T) {
		_, err := Participants(ctx, client, ParticipantsInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Reviewers", func(t *testing.T) {
		_, err := Reviewers(ctx, client, ParticipantsInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("CreatePipeline", func(t *testing.T) {
		_, err := CreatePipeline(ctx, client, CreatePipelineInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("IssuesClosed", func(t *testing.T) {
		_, err := IssuesClosed(ctx, client, IssuesClosedInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("CancelAutoMerge", func(t *testing.T) {
		_, err := CancelAutoMerge(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Subscribe", func(t *testing.T) {
		_, err := Subscribe(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Unsubscribe", func(t *testing.T) {
		_, err := Unsubscribe(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("SetTimeEstimate", func(t *testing.T) {
		_, err := SetTimeEstimate(ctx, client, SetTimeEstimateInput{ProjectID: pid, MRIID: 0, Duration: "1h"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("ResetTimeEstimate", func(t *testing.T) {
		_, err := ResetTimeEstimate(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("AddSpentTime", func(t *testing.T) {
		_, err := AddSpentTime(ctx, client, AddSpentTimeInput{ProjectID: pid, MRIID: 0, Duration: "1h"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("ResetSpentTime", func(t *testing.T) {
		_, err := ResetSpentTime(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("GetTimeStats", func(t *testing.T) {
		_, err := GetTimeStats(ctx, client, GetInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("RelatedIssues", func(t *testing.T) {
		_, err := RelatedIssues(ctx, client, RelatedIssuesInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("CreateTodo", func(t *testing.T) {
		_, err := CreateTodo(ctx, client, CreateTodoInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("CreateDependency", func(t *testing.T) {
		_, err := CreateDependency(ctx, client, DependencyInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("DeleteDependency", func(t *testing.T) {
		err := DeleteDependency(ctx, client, DeleteDependencyInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("GetDependencies", func(t *testing.T) {
		_, err := GetDependencies(ctx, client, GetDependenciesInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
}

// assertContains is a test helper that verifies err is non-nil and its
// message contains the expected substring.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// ---------------------------------------------------------------------------
// ProjectPath extraction tests
// ---------------------------------------------------------------------------.

// TestOutput_ProjectPathFromReferences verifies that ProjectPath is correctly
// extracted from the GitLab References.Full field (format: "group/project!IID").
func TestOutput_ProjectPathFromReferences(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantPath string
	}{
		{
			name:     "full reference with group",
			json:     `{"id":1,"iid":1,"state":"opened","references":{"full":"mygroup/myproject!1"}}`,
			wantPath: "mygroup/myproject",
		},
		{
			name:     "full reference with nested groups",
			json:     `{"id":2,"iid":5,"state":"merged","references":{"full":"org/team/myproject!5"}}`,
			wantPath: "org/team/myproject",
		},
		{
			name:     "no references",
			json:     `{"id":3,"iid":10,"state":"opened"}`,
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tt.json)
			}))
			out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if out.ProjectPath != tt.wantPath {
				t.Errorf("ProjectPath = %q, want %q", out.ProjectPath, tt.wantPath)
			}
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	testProjectID     = "42"
	testFeatureBranch = "feature/login"
	testCreatedAt     = "2026-01-01T00:00:00Z"
	testMRWebURL      = "https://gitlab.example.com/mr/1"
	testMRTitle       = "feat: login"
	testBlockerTitle  = "Blocker MR"
	testLabels        = "bug,critical"
	testCreatedBefore = "2026-12-31T23:59:59Z"
	testStateOpened   = "opened"
	testStateMerged   = "merged"
	testStatePending  = "pending"
	testStateActive   = "active"
	testLabelBug      = "bug"
	testLabelWontfix  = "wontfix"
	testBranchMain    = "main"
	testBranchFeat    = "feat"
	testBranchFeatA   = "feat/a"
	testAuthorAlice   = "alice"
	testAuthorBob     = "bob"
	testAuthorCarol   = "carol"
	testActionMarked  = "marked"
	testTargetTypeMR  = "MergeRequest"
	testDepTitleA     = "Dep A"
	testMilestoneV1   = "v1.0"
	testVersion       = "0.0.1"
	testSHAAbc        = "abc123"
	pathSuffixBlocks  = "/blocks"
	fmtIIDWant        = "IID = %d, want 1"
	testDate20260101  = "2026-01-01"
)

// ---------------------------------------------------------------------------
// Format*Markdown tests
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Populated verifies the behavior of format markdown populated.
func TestFormatMarkdown_Populated(t *testing.T) {
	md := FormatMarkdown(Output{
		IID: 1, Title: "feat: new login", State: testStateOpened,
		SourceBranch: testFeatureBranch, TargetBranch: testBranchMain,
		MergeStatus: "can_be_merged", Draft: true, HasConflicts: true,
		Author: testAuthorAlice, Assignees: []string{testAuthorBob, testAuthorCarol},
		Reviewers: []string{"dave"}, Labels: []string{testLabelBug, "enhancement"},
		CreatedAt: testCreatedAt, UserNotesCount: 5,
		Description: "Full description here", WebURL: testMRWebURL,
	})
	for _, want := range []string{
		"feat: new login", testStateOpened, testFeatureBranch, testBranchMain,
		"Draft", "Has Conflicts", "@alice", "@bob", "@carol", "@dave",
		testLabelBug, "enhancement", "1 Jan 2026", "Comments", "5",
		"Full description here", testMRWebURL,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q", want)
		}
	}
}

// TestFormatMarkdown_Empty verifies the behavior of format markdown empty.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown(Output{})
	if md == "" {
		t.Error("FormatMarkdown returned empty string for zero Output")
	}
}

// TestFormatListMarkdown_Populated verifies the behavior of format list markdown populated.
func TestFormatListMarkdown_Populated(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		MergeRequests: []Output{
			{IID: 1, Title: "MR1", State: testStateOpened, Draft: true, Author: testAuthorAlice, SourceBranch: "a", TargetBranch: "b"},
			{IID: 2, Title: "MR2", State: testStateMerged, Author: testAuthorBob, SourceBranch: "c", TargetBranch: "d"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})
	for _, want := range []string{"MR1", "MR2", testAuthorAlice, testAuthorBob, "📝"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown missing %q", want)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No merge requests found") {
		t.Error("FormatListMarkdown should say no MRs found for empty list")
	}
}

// TestFormatApproveMarkdown_Populated verifies the behavior of format approve markdown populated.
func TestFormatApproveMarkdown_Populated(t *testing.T) {
	md := FormatApproveMarkdown(ApproveOutput{Approved: true, ApprovalsRequired: 2, ApprovedBy: 1})
	for _, want := range []string{"Approved", "true", "2", "1"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatApproveMarkdown missing %q", want)
		}
	}
}

// TestFormatApproveMarkdown_Empty verifies the behavior of format approve markdown empty.
func TestFormatApproveMarkdown_Empty(t *testing.T) {
	md := FormatApproveMarkdown(ApproveOutput{})
	if md == "" {
		t.Error("FormatApproveMarkdown returned empty string for zero value")
	}
}

// TestFormatCommitsMarkdown_Populated verifies the behavior of format commits markdown populated.
func TestFormatCommitsMarkdown_Populated(t *testing.T) {
	md := FormatCommitsMarkdown(CommitsOutput{
		Commits: []commits.Output{
			{ShortID: "abc1234", Title: "feat: add login", AuthorName: "Alice", CommittedDate: testDate20260101},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"abc1234", "feat: add login", "Alice", "1 Jan 2026"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatCommitsMarkdown missing %q", want)
		}
	}
}

// TestFormatCommitsMarkdown_Empty verifies the behavior of format commits markdown empty.
func TestFormatCommitsMarkdown_Empty(t *testing.T) {
	md := FormatCommitsMarkdown(CommitsOutput{})
	if !strings.Contains(md, "No commits found") {
		t.Error("FormatCommitsMarkdown should say no commits for empty output")
	}
}

// TestFormatPipelinesMarkdown_Populated verifies the behavior of format pipelines markdown populated.
func TestFormatPipelinesMarkdown_Populated(t *testing.T) {
	md := FormatPipelinesMarkdown(PipelinesOutput{
		Pipelines: []pipelines.Output{
			{ID: 10, Status: "success", Source: "push", Ref: testBranchMain},
		},
	})
	for _, want := range []string{"10", "success", "push", testBranchMain} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatPipelinesMarkdown missing %q", want)
		}
	}
}

// TestFormatPipelinesMarkdown_Empty verifies the behavior of format pipelines markdown empty.
func TestFormatPipelinesMarkdown_Empty(t *testing.T) {
	md := FormatPipelinesMarkdown(PipelinesOutput{})
	if !strings.Contains(md, "No pipelines found") {
		t.Error("FormatPipelinesMarkdown should say no pipelines for empty output")
	}
}

// TestFormatRebaseMarkdown_InProgress verifies the behavior of format rebase markdown in progress.
func TestFormatRebaseMarkdown_InProgress(t *testing.T) {
	md := FormatRebaseMarkdown(RebaseOutput{RebaseInProgress: true})
	if !strings.Contains(md, "in progress") {
		t.Error("FormatRebaseMarkdown should indicate rebase in progress")
	}
}

// TestFormatRebaseMarkdown_Completed verifies the behavior of format rebase markdown completed.
func TestFormatRebaseMarkdown_Completed(t *testing.T) {
	md := FormatRebaseMarkdown(RebaseOutput{RebaseInProgress: false})
	if !strings.Contains(md, "completed") {
		t.Error("FormatRebaseMarkdown should indicate rebase completed")
	}
}

// TestFormatParticipantsMarkdown_Populated verifies the behavior of format participants markdown populated.
func TestFormatParticipantsMarkdown_Populated(t *testing.T) {
	md := FormatParticipantsMarkdown(ParticipantsOutput{
		Participants: []ParticipantOutput{
			{ID: 1, Username: testAuthorAlice, Name: "Alice A", State: testStateActive},
			{ID: 2, Username: testAuthorBob, Name: "Bob B", State: testStateActive},
		},
	})
	for _, want := range []string{testAuthorAlice, testAuthorBob, "Alice A", "Bob B", testStateActive, "Participants (2)"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatParticipantsMarkdown missing %q", want)
		}
	}
}

// TestFormatParticipantsMarkdown_Empty verifies the behavior of format participants markdown empty.
func TestFormatParticipantsMarkdown_Empty(t *testing.T) {
	md := FormatParticipantsMarkdown(ParticipantsOutput{})
	if !strings.Contains(md, "No participants found") {
		t.Error("FormatParticipantsMarkdown should say no participants for empty output")
	}
}

// TestFormatReviewersMarkdown_Populated verifies the behavior of format reviewers markdown populated.
func TestFormatReviewersMarkdown_Populated(t *testing.T) {
	md := FormatReviewersMarkdown(ReviewersOutput{
		Reviewers: []ReviewerOutput{
			{ID: 10, Username: testAuthorCarol, Name: "Carol C", State: testStateActive, Review: "reviewed", CreatedAt: "2026-03-01T10:00:00Z"},
		},
	})
	for _, want := range []string{testAuthorCarol, "Carol C", "reviewed", "1 Mar 2026", "Reviewers (1)"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatReviewersMarkdown missing %q", want)
		}
	}
}

// TestFormatReviewersMarkdown_Empty verifies the behavior of format reviewers markdown empty.
func TestFormatReviewersMarkdown_Empty(t *testing.T) {
	md := FormatReviewersMarkdown(ReviewersOutput{})
	if !strings.Contains(md, "No reviewers found") {
		t.Error("FormatReviewersMarkdown should say no reviewers for empty output")
	}
}

// TestFormatIssuesClosedMarkdown_Populated verifies the behavior of format issues closed markdown populated.
func TestFormatIssuesClosedMarkdown_Populated(t *testing.T) {
	md := FormatIssuesClosedMarkdown(IssuesClosedOutput{
		Issues: []issues.Output{
			{IID: 5, Title: "Bug fix", State: testStateOpened, Author: testAuthorAlice, Labels: []string{testLabelBug}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"Bug fix", testStateOpened, testAuthorAlice, testLabelBug, "#5"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatIssuesClosedMarkdown missing %q", want)
		}
	}
}

// TestFormatIssuesClosedMarkdown_Empty verifies the behavior of format issues closed markdown empty.
func TestFormatIssuesClosedMarkdown_Empty(t *testing.T) {
	md := FormatIssuesClosedMarkdown(IssuesClosedOutput{})
	if !strings.Contains(md, "No issues will be closed") {
		t.Error("FormatIssuesClosedMarkdown should say no issues for empty output")
	}
}

// TestFormatCreatePipelineMarkdown verifies the behavior of format create pipeline markdown.
func TestFormatCreatePipelineMarkdown(t *testing.T) {
	md := FormatCreatePipelineMarkdown(pipelines.Output{
		ID: 500, Status: testStatePending, Source: "merge_request_event", Ref: testFeatureBranch,
		SHA: testSHAAbc, WebURL: "https://gitlab.example.com/pipelines/500",
	})
	for _, want := range []string{"500", testStatePending, "merge_request_event", testFeatureBranch, testSHAAbc, "https://gitlab.example.com/pipelines/500"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatCreatePipelineMarkdown missing %q", want)
		}
	}
}

// TestFormatCreatePipelineMarkdown_Minimal verifies the behavior of format create pipeline markdown minimal.
func TestFormatCreatePipelineMarkdown_Minimal(t *testing.T) {
	md := FormatCreatePipelineMarkdown(pipelines.Output{ID: 1, Status: "created"})
	if md == "" {
		t.Error("FormatCreatePipelineMarkdown returned empty string")
	}
	if !strings.Contains(md, "created") {
		t.Error("FormatCreatePipelineMarkdown should contain status")
	}
}

// TestFormatTimeStatsMarkdown_Populated verifies the behavior of format time stats markdown populated.
func TestFormatTimeStatsMarkdown_Populated(t *testing.T) {
	md := FormatTimeStatsMarkdown(TimeStatsOutput{
		HumanTimeEstimate: "3h", HumanTotalTimeSpent: "1h30m",
		TimeEstimate: 10800, TotalTimeSpent: 5400,
	})
	for _, want := range []string{"3h", "10800", "1h30m", "5400"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatTimeStatsMarkdown missing %q", want)
		}
	}
}

// TestFormatTimeStatsMarkdown_Empty verifies the behavior of format time stats markdown empty.
func TestFormatTimeStatsMarkdown_Empty(t *testing.T) {
	md := FormatTimeStatsMarkdown(TimeStatsOutput{})
	if !strings.Contains(md, "not set") {
		t.Error("FormatTimeStatsMarkdown should say 'not set' for empty estimate")
	}
	if !strings.Contains(md, "none") {
		t.Error("FormatTimeStatsMarkdown should say 'none' for empty spent")
	}
}

// TestFormatRelatedIssuesMarkdown_Populated verifies the behavior of format related issues markdown populated.
func TestFormatRelatedIssuesMarkdown_Populated(t *testing.T) {
	md := FormatRelatedIssuesMarkdown(RelatedIssuesOutput{
		Issues: []issues.Output{
			{IID: 10, Title: "Related bug", State: testStateOpened, Author: testAuthorAlice, Labels: []string{testLabelBug, "critical"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"Related bug", testStateOpened, testAuthorAlice, testLabelBug, "critical", "#10"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatRelatedIssuesMarkdown missing %q", want)
		}
	}
}

// TestFormatRelatedIssuesMarkdown_Empty verifies the behavior of format related issues markdown empty.
func TestFormatRelatedIssuesMarkdown_Empty(t *testing.T) {
	md := FormatRelatedIssuesMarkdown(RelatedIssuesOutput{})
	if !strings.Contains(md, "No related issues found") {
		t.Error("FormatRelatedIssuesMarkdown should say no related issues for empty output")
	}
}

// TestFormatCreateTodoMarkdown_Populated verifies the behavior of format create todo markdown populated.
func TestFormatCreateTodoMarkdown_Populated(t *testing.T) {
	md := FormatCreateTodoMarkdown(CreateTodoOutput{
		ID: 42, ActionName: testActionMarked, TargetType: testTargetTypeMR,
		TargetTitle: testMRTitle, TargetURL: testMRWebURL,
		State: testStatePending,
	})
	for _, want := range []string{"42", testActionMarked, testTargetTypeMR, testMRTitle, testMRWebURL, testStatePending} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatCreateTodoMarkdown missing %q", want)
		}
	}
}

// TestFormatCreateTodoMarkdown_Empty verifies the behavior of format create todo markdown empty.
func TestFormatCreateTodoMarkdown_Empty(t *testing.T) {
	md := FormatCreateTodoMarkdown(CreateTodoOutput{})
	if md == "" {
		t.Error("FormatCreateTodoMarkdown returned empty string for zero value")
	}
}

// TestFormatListMarkdown_ClickableMRLinks verifies that the MR list
// renders IIDs as clickable Markdown links [!IID](weburl).
func TestFormatListMarkdown_ClickableMRLinks(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		MergeRequests: []Output{
			{IID: 7, Title: "MR7", State: testStateOpened, Author: testAuthorAlice,
				WebURL: "https://gitlab.example.com/mr/7", SourceBranch: "a", TargetBranch: "b"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "[!7](https://gitlab.example.com/mr/7)") {
		t.Errorf("expected clickable MR link, got:\n%s", md)
	}
}

// TestFormatMarkdown_ClickableURL verifies that the MR detail view
// renders the URL as a clickable Markdown link [url](url).
func TestFormatMarkdown_ClickableURL(t *testing.T) {
	md := FormatMarkdown(Output{
		IID: 3, Title: "test", State: testStateOpened,
		WebURL: "https://gitlab.example.com/mr/3",
	})
	if !strings.Contains(md, "[https://gitlab.example.com/mr/3](https://gitlab.example.com/mr/3)") {
		t.Errorf("expected clickable URL in detail, got:\n%s", md)
	}
}

// TestFormatCreatePipelineMarkdown_ClickableURL verifies the created pipeline
// markdown renders the URL as a clickable link.
func TestFormatCreatePipelineMarkdown_ClickableURL(t *testing.T) {
	md := FormatCreatePipelineMarkdown(pipelines.Output{
		ID: 500, Status: testStatePending,
		WebURL: "https://gitlab.example.com/pipelines/500",
	})
	if !strings.Contains(md, "[https://gitlab.example.com/pipelines/500](https://gitlab.example.com/pipelines/500)") {
		t.Errorf("expected clickable pipeline URL, got:\n%s", md)
	}
}

// TestFormatCreateTodoMarkdown_ClickableURL verifies the created todo
// markdown renders the TargetURL as a clickable link.
func TestFormatCreateTodoMarkdown_ClickableURL(t *testing.T) {
	md := FormatCreateTodoMarkdown(CreateTodoOutput{
		ID: 42, ActionName: testActionMarked, TargetType: testTargetTypeMR,
		TargetTitle: testMRTitle, TargetURL: "https://gitlab.example.com/todo/42",
		State: testStatePending,
	})
	if !strings.Contains(md, "[https://gitlab.example.com/todo/42](https://gitlab.example.com/todo/42)") {
		t.Errorf("expected clickable todo URL, got:\n%s", md)
	}
}

// TestFormatDependencyMarkdown_Populated verifies the behavior of format dependency markdown populated.
func TestFormatDependencyMarkdown_Populated(t *testing.T) {
	md := FormatDependencyMarkdown(DependencyOutput{
		ID: 1, BlockingMRID: 100, BlockingMRIID: 10, BlockingMRTitle: testBlockerTitle,
		BlockingMRState: testStateOpened, BlockingSourceBranch: testBranchFeatA, BlockingTargetBranch: testBranchMain,
	})
	for _, want := range []string{testBlockerTitle, "!10", testStateOpened, testBranchFeatA, testBranchMain} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatDependencyMarkdown missing %q", want)
		}
	}
}

// TestFormatDependencyMarkdown_Empty verifies the behavior of format dependency markdown empty.
func TestFormatDependencyMarkdown_Empty(t *testing.T) {
	md := FormatDependencyMarkdown(DependencyOutput{})
	if md == "" {
		t.Error("FormatDependencyMarkdown returned empty string for zero value")
	}
}

// TestFormatDependenciesMarkdown_Populated verifies the behavior of format dependencies markdown populated.
func TestFormatDependenciesMarkdown_Populated(t *testing.T) {
	md := FormatDependenciesMarkdown(DependenciesOutput{
		Dependencies: []DependencyOutput{
			{ID: 1, BlockingMRIID: 10, BlockingMRTitle: testDepTitleA, BlockingMRState: testStateOpened},
			{ID: 2, BlockingMRIID: 20, BlockingMRTitle: "Dep B", BlockingMRState: testStateMerged},
		},
	})
	for _, want := range []string{testDepTitleA, "Dep B", "!10", "!20", testStateOpened, testStateMerged, "Dependencies (2)"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatDependenciesMarkdown missing %q", want)
		}
	}
}

// TestFormatDependenciesMarkdown_Empty verifies the behavior of format dependencies markdown empty.
func TestFormatDependenciesMarkdown_Empty(t *testing.T) {
	md := FormatDependenciesMarkdown(DependenciesOutput{})
	if !strings.Contains(md, "No dependencies found") {
		t.Error("FormatDependenciesMarkdown should say no dependencies for empty output")
	}
}

// ---------------------------------------------------------------------------
// RelatedIssues handler tests
// ---------------------------------------------------------------------------.

// TestRelatedIssues_Success verifies the behavior of related issues success.
func TestRelatedIssues_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+"/related_issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"iid":5,"title":"Related issue","state":"opened","author":{"username":"alice"},"labels":["bug"],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","web_url":"http://issue/5","project_id":42}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RelatedIssues(context.Background(), client, RelatedIssuesInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("RelatedIssues() unexpected error: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("RelatedIssues() returned %d issues, want 1", len(out.Issues))
	}
	if out.Issues[0].Title != "Related issue" {
		t.Errorf("issue Title = %q, want %q", out.Issues[0].Title, "Related issue")
	}
	if out.Issues[0].IID != 5 {
		t.Errorf("issue IID = %d, want 5", out.Issues[0].IID)
	}
}

// TestRelatedIssues_MissingProject verifies the behavior of related issues missing project.
func TestRelatedIssues_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := RelatedIssues(context.Background(), client, RelatedIssuesInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestRelatedIssues_CancelledContext verifies the behavior of related issues cancelled context.
func TestRelatedIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := RelatedIssues(ctx, client, RelatedIssuesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("RelatedIssues() expected error for canceled context, got nil")
	}
}

// TestRelatedIssues_APIError verifies the behavior of related issues a p i error.
func TestRelatedIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"error":"server error"}`)
	}))
	_, err := RelatedIssues(context.Background(), client, RelatedIssuesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("RelatedIssues() expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreateTodo handler tests
// ---------------------------------------------------------------------------.

// TestCreateTodo_Success verifies the behavior of create todo success.
func TestCreateTodo_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/todo" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":42,"action_name":"marked","target_type":"MergeRequest",
				"target":{"title":"feat: login"},
				"target_url":"https://gitlab.example.com/mr/1",
				"state":"pending",
				"project":{"name":"my-project"},
				"created_at":"2026-03-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateTodo(context.Background(), client, CreateTodoInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("CreateTodo() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
	if out.ActionName != testActionMarked {
		t.Errorf("out.ActionName = %q, want %q", out.ActionName, testActionMarked)
	}
	if out.TargetType != testTargetTypeMR {
		t.Errorf("out.TargetType = %q, want %q", out.TargetType, testTargetTypeMR)
	}
	if out.TargetTitle != testMRTitle {
		t.Errorf("out.TargetTitle = %q, want %q", out.TargetTitle, testMRTitle)
	}
	if out.State != testStatePending {
		t.Errorf("out.State = %q, want %q", out.State, testStatePending)
	}
	if out.ProjectName != "my-project" {
		t.Errorf("out.ProjectName = %q, want %q", out.ProjectName, "my-project")
	}
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt should not be empty")
	}
}

// TestCreateTodo_MissingProject verifies the behavior of create todo missing project.
func TestCreateTodo_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateTodo(context.Background(), client, CreateTodoInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateTodo_CancelledContext verifies the behavior of create todo cancelled context.
func TestCreateTodo_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateTodo(ctx, client, CreateTodoInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("CreateTodo() expected error for canceled context, got nil")
	}
}

// TestCreateTodo_APIError verifies the behavior of create todo a p i error.
func TestCreateTodo_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"error":"forbidden"}`)
	}))
	_, err := CreateTodo(context.Background(), client, CreateTodoInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("CreateTodo() expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreateDependency handler tests
// ---------------------------------------------------------------------------.

// TestCreateDependency_Success verifies the behavior of create dependency success.
func TestCreateDependency_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+pathSuffixBlocks {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":1,"project_id":42,
				"blocking_merge_request":{
					"id":100,"iid":10,"title":"Blocker MR","state":"opened",
					"project_id":42,"source_branch":"feat/a","target_branch":"main"
				}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateDependency(context.Background(), client, DependencyInput{
		ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 100,
	})
	if err != nil {
		t.Fatalf("CreateDependency() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.BlockingMRIID != 10 {
		t.Errorf("out.BlockingMRIID = %d, want 10", out.BlockingMRIID)
	}
	if out.BlockingMRTitle != testBlockerTitle {
		t.Errorf("out.BlockingMRTitle = %q, want %q", out.BlockingMRTitle, testBlockerTitle)
	}
	if out.BlockingMRState != testStateOpened {
		t.Errorf("out.BlockingMRState = %q, want %q", out.BlockingMRState, testStateOpened)
	}
	if out.BlockingSourceBranch != testBranchFeatA {
		t.Errorf("out.BlockingSourceBranch = %q, want %q", out.BlockingSourceBranch, testBranchFeatA)
	}
	if out.BlockingTargetBranch != testBranchMain {
		t.Errorf("out.BlockingTargetBranch = %q, want %q", out.BlockingTargetBranch, testBranchMain)
	}
}

// TestCreateDependency_MissingProject verifies the behavior of create dependency missing project.
func TestCreateDependency_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateDependency(context.Background(), client, DependencyInput{MRIID: 1, BlockingMergeRequestID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateDependency_CancelledContext verifies the behavior of create dependency cancelled context.
func TestCreateDependency_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateDependency(ctx, client, DependencyInput{ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 100})
	if err == nil {
		t.Fatal("CreateDependency() expected error for canceled context, got nil")
	}
}

// TestCreateDependency_APIError verifies the behavior of create dependency a p i error.
func TestCreateDependency_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"already exists"}`)
	}))
	_, err := CreateDependency(context.Background(), client, DependencyInput{ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 100})
	if err == nil {
		t.Fatal("CreateDependency() expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteDependency handler tests
// ---------------------------------------------------------------------------.

// TestDeleteDependency_Success verifies the behavior of delete dependency success.
func TestDeleteDependency_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, pathMR1+pathSuffixBlocks) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteDependency(context.Background(), client, DeleteDependencyInput{
		ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 100,
	})
	if err != nil {
		t.Fatalf("DeleteDependency() unexpected error: %v", err)
	}
}

// TestDeleteDependency_MissingProject verifies the behavior of delete dependency missing project.
func TestDeleteDependency_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteDependency(context.Background(), client, DeleteDependencyInput{MRIID: 1, BlockingMergeRequestID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDeleteDependency_CancelledContext verifies the behavior of delete dependency cancelled context.
func TestDeleteDependency_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteDependency(ctx, client, DeleteDependencyInput{ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 100})
	if err == nil {
		t.Fatal("DeleteDependency() expected error for canceled context, got nil")
	}
}

// TestDeleteDependency_APIError verifies the behavior of delete dependency a p i error.
func TestDeleteDependency_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"error":"not found"}`)
	}))
	err := DeleteDependency(context.Background(), client, DeleteDependencyInput{ProjectID: testProjectID, MRIID: 1, BlockingMergeRequestID: 999})
	if err == nil {
		t.Fatal("DeleteDependency() expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetDependencies handler tests
// ---------------------------------------------------------------------------.

// TestGetDependencies_Success verifies the behavior of get dependencies success.
func TestGetDependencies_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1+pathSuffixBlocks {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":1,"project_id":42,
				"blocking_merge_request":{
					"id":100,"iid":10,"title":"Dep A","state":"opened",
					"project_id":42,"source_branch":"feat/a","target_branch":"main"
				}
			},{
				"id":2,"project_id":42,
				"blocking_merge_request":{
					"id":200,"iid":20,"title":"Dep B","state":"merged",
					"project_id":42,"source_branch":"feat/b","target_branch":"main"
				}
			}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetDependencies(context.Background(), client, GetDependenciesInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("GetDependencies() unexpected error: %v", err)
	}
	if len(out.Dependencies) != 2 {
		t.Fatalf("GetDependencies() returned %d deps, want 2", len(out.Dependencies))
	}
	if out.Dependencies[0].BlockingMRTitle != testDepTitleA {
		t.Errorf("dep[0].BlockingMRTitle = %q, want %q", out.Dependencies[0].BlockingMRTitle, testDepTitleA)
	}
	if out.Dependencies[1].BlockingMRState != testStateMerged {
		t.Errorf("dep[1].BlockingMRState = %q, want %q", out.Dependencies[1].BlockingMRState, testStateMerged)
	}
}

// TestGetDependencies_MissingProject verifies the behavior of get dependencies missing project.
func TestGetDependencies_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetDependencies(context.Background(), client, GetDependenciesInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGetDependencies_CancelledContext verifies the behavior of get dependencies cancelled context.
func TestGetDependencies_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetDependencies(ctx, client, GetDependenciesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("GetDependencies() expected error for canceled context, got nil")
	}
}

// TestGetDependencies_APIError verifies the behavior of get dependencies a p i error.
func TestGetDependencies_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"error":"forbidden"}`)
	}))
	_, err := GetDependencies(context.Background(), client, GetDependenciesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("GetDependencies() expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Context-canceled tests for previously untested handlers
// ---------------------------------------------------------------------------.

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, SourceBranch: "a", TargetBranch: "b", Title: "t"})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Get() expected error for canceled context, got nil")
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Update() expected error for canceled context, got nil")
	}
}

// TestMerge_CancelledContext verifies the behavior of merge cancelled context.
func TestMerge_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Merge(ctx, client, MergeInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Merge() expected error for canceled context, got nil")
	}
}

// TestApprove_CancelledContext verifies the behavior of approve cancelled context.
func TestApprove_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Approve(ctx, client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Approve() expected error for canceled context, got nil")
	}
}

// TestUnapprove_CancelledContext verifies the behavior of unapprove cancelled context.
func TestUnapprove_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Unapprove(ctx, client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Unapprove() expected error for canceled context, got nil")
	}
}

// TestCommits_CancelledContext verifies the behavior of commits cancelled context.
func TestCommits_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Commits(ctx, client, CommitsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Commits() expected error for canceled context, got nil")
	}
}

// TestPipelines_CancelledContext verifies the behavior of pipelines cancelled context.
func TestPipelines_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Pipelines(ctx, client, PipelinesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Pipelines() expected error for canceled context, got nil")
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for canceled context, got nil")
	}
}

// TestRebase_CancelledContext verifies the behavior of rebase cancelled context.
func TestRebase_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Rebase(ctx, client, RebaseInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Rebase() expected error for canceled context, got nil")
	}
}

// TestListGlobal_CancelledContext verifies the behavior of list global cancelled context.
func TestListGlobal_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGlobal(ctx, client, ListGlobalInput{})
	if err == nil {
		t.Fatal("ListGlobal() expected error for canceled context, got nil")
	}
}

// TestListGroup_CancelledContext verifies the behavior of list group cancelled context.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal("ListGroup() expected error for canceled context, got nil")
	}
}

// TestParticipants_CancelledContext verifies the behavior of participants cancelled context.
func TestParticipants_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Participants(ctx, client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Participants() expected error for canceled context, got nil")
	}
}

// TestReviewers_CancelledContext verifies the behavior of reviewers cancelled context.
func TestReviewers_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Reviewers(ctx, client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Reviewers() expected error for canceled context, got nil")
	}
}

// TestCreatePipeline_CancelledContext verifies the behavior of create pipeline cancelled context.
func TestCreatePipeline_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreatePipeline(ctx, client, CreatePipelineInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("CreatePipeline() expected error for canceled context, got nil")
	}
}

// TestIssuesClosed_CancelledContext verifies the behavior of issues closed cancelled context.
func TestIssuesClosed_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := IssuesClosed(ctx, client, IssuesClosedInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("IssuesClosed() expected error for canceled context, got nil")
	}
}

// TestCancelAutoMerge_CancelledContext verifies the behavior of cancel auto merge cancelled context.
func TestCancelAutoMerge_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CancelAutoMerge(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("CancelAutoMerge() expected error for canceled context, got nil")
	}
}

// TestSubscribe_CancelledContext verifies the behavior of subscribe cancelled context.
func TestSubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Subscribe(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Subscribe() expected error for canceled context, got nil")
	}
}

// TestUnsubscribe_CancelledContext verifies the behavior of unsubscribe cancelled context.
func TestUnsubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Unsubscribe(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("Unsubscribe() expected error for canceled context, got nil")
	}
}

// TestSetTimeEstimate_CancelledContext verifies the behavior of set time estimate cancelled context.
func TestSetTimeEstimate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := SetTimeEstimate(ctx, client, SetTimeEstimateInput{ProjectID: testProjectID, MRIID: 1, Duration: "3h"})
	if err == nil {
		t.Fatal("SetTimeEstimate() expected error for canceled context, got nil")
	}
}

// TestResetTimeEstimate_CancelledContext verifies the behavior of reset time estimate cancelled context.
func TestResetTimeEstimate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetTimeEstimate(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("ResetTimeEstimate() expected error for canceled context, got nil")
	}
}

// TestAddSpentTime_CancelledContext verifies the behavior of add spent time cancelled context.
func TestAddSpentTime_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddSpentTime(ctx, client, AddSpentTimeInput{ProjectID: testProjectID, MRIID: 1, Duration: "1h"})
	if err == nil {
		t.Fatal("AddSpentTime() expected error for canceled context, got nil")
	}
}

// TestResetSpentTime_CancelledContext verifies the behavior of reset spent time cancelled context.
func TestResetSpentTime_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetSpentTime(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("ResetSpentTime() expected error for canceled context, got nil")
	}
}

// TestGetTimeStats_CancelledContext verifies the behavior of get time stats cancelled context.
func TestGetTimeStats_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetTimeStats(ctx, client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("GetTimeStats() expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// Missing project_id tests for handlers not already tested
// ---------------------------------------------------------------------------.

// TestCreate_MissingProject verifies the behavior of create missing project.
func TestCreate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{SourceBranch: "a", TargetBranch: "b", Title: "t"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_MissingProject verifies the behavior of get missing project.
func TestGet_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_MissingProject verifies the behavior of list missing project.
func TestList_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_MissingProject verifies the behavior of update missing project.
func TestUpdate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestMerge_MissingProject verifies the behavior of merge missing project.
func TestMerge_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Merge(context.Background(), client, MergeInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestApprove_MissingProject verifies the behavior of approve missing project.
func TestApprove_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Approve(context.Background(), client, ApproveInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUnapprove_MissingProject verifies the behavior of unapprove missing project.
func TestUnapprove_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Unapprove(context.Background(), client, ApproveInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestSubscribe_MissingProject verifies the behavior of subscribe missing project.
func TestSubscribe_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Subscribe(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUnsubscribe_MissingProject verifies the behavior of unsubscribe missing project.
func TestUnsubscribe_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Unsubscribe(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestSetTimeEstimate_MissingProject verifies the behavior of set time estimate missing project.
func TestSetTimeEstimate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{MRIID: 1, Duration: "3h"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestResetTimeEstimate_MissingProject verifies the behavior of reset time estimate missing project.
func TestResetTimeEstimate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ResetTimeEstimate(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestAddSpentTime_MissingProject verifies the behavior of add spent time missing project.
func TestAddSpentTime_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{MRIID: 1, Duration: "1h"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestResetSpentTime_MissingProject verifies the behavior of reset spent time missing project.
func TestResetSpentTime_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ResetSpentTime(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestIssuesClosed_MissingProject verifies the behavior of issues closed missing project.
func TestIssuesClosed_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := IssuesClosed(context.Background(), client, IssuesClosedInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCancelAutoMerge_MissingProject2 verifies the behavior of cancel auto merge missing project2.
func TestCancelAutoMerge_MissingProject2(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CancelAutoMerge(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools MCP integration test
// ---------------------------------------------------------------------------.

const (
	mrJSONCoverage = `{"id":100,"iid":1,"title":"Test MR","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://mr/1","detailed_merge_status":"can_be_merged"}`

	dependencyJSONCoverage = `{
		"id":1,"project_id":42,
		"blocking_merge_request":{"id":100,"iid":10,"title":"Dep","state":"opened","project_id":42,"source_branch":"a","target_branch":"b"}
	}`

	todoJSONCoverage = `{"id":1,"action_name":"marked","target_type":"MergeRequest","target":{"title":"Test"},"target_url":"http://mr/1","state":"pending","created_at":"2026-01-01T00:00:00Z"}`
)

// mrMockResp holds a canned response for a mock MR endpoint.
type mrMockResp struct {
	status int
	body   string
	pgHdr  *testutil.PaginationHeaders
}

// defaultPgHdr returns the default single-page pagination header.
func defaultPgHdr() *testutil.PaginationHeaders {
	return &testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}
}

// newMRMCPSession is an internal helper for the mergerequests package.
func newMRMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	mrListBody := `[` + mrJSONCoverage + `]`
	routes := map[string]mrMockResp{
		// Create MR
		"POST " + pathMRs: {http.StatusCreated, mrJSONCoverage, nil},
		// Get MR
		"GET " + pathMR1: {http.StatusOK, mrJSONCoverage, nil},
		// Update MR
		"PUT " + pathMR1: {http.StatusOK, mrJSONCoverage, nil},
		// List MRs (project)
		"GET " + pathMRs: {http.StatusOK, mrListBody, defaultPgHdr()},
		// Merge
		"PUT " + pathMR1 + "/merge": {http.StatusOK, mrJSONCoverage, nil},
		// Approve
		"POST " + pathMR1 + "/approve": {http.StatusCreated, `{"approvals_required":1,"approved_by":[{"user":{"username":"test"}}],"approved":true}`, nil},
		// Unapprove
		"POST " + pathMR1 + "/unapprove": {http.StatusNoContent, "", nil},
		// Commits
		"GET " + pathMR1 + "/commits": {http.StatusOK, `[{"id":"abc","short_id":"abc","title":"commit","author_name":"test","committed_date":"2026-01-01T00:00:00Z","web_url":"http://c/1"}]`, defaultPgHdr()},
		// Pipelines
		"GET " + pathMR1 + "/pipelines": {http.StatusOK, `[{"id":10,"iid":10,"project_id":42,"status":"success","ref":"main","sha":"abc","web_url":"http://p/10","created_at":"2026-01-01T00:00:00Z"}]`, nil},
		// Delete MR
		"DELETE " + pathMR1: {http.StatusNoContent, "", nil},
		// Rebase
		"PUT " + pathMR1 + "/rebase": {http.StatusAccepted, "", nil},
		// List Global
		"GET /api/v4/merge_requests": {http.StatusOK, mrListBody, defaultPgHdr()},
		// List Group
		"GET /api/v4/groups/99/merge_requests": {http.StatusOK, mrListBody, defaultPgHdr()},
		// Participants
		"GET " + pathMR1 + "/participants": {http.StatusOK, `[{"id":1,"username":"alice","name":"Alice","state":"active"}]`, nil},
		// Reviewers
		"GET " + pathMR1 + "/reviewers": {http.StatusOK, `[{"user":{"id":1,"username":"bob","name":"Bob","state":"active"},"state":"reviewed","created_at":"2026-01-01T00:00:00Z"}]`, nil},
		// Create Pipeline
		"POST " + pathMR1 + "/pipelines": {http.StatusCreated, `{"id":500,"iid":10,"project_id":42,"status":"pending","ref":"feat","sha":"abc","web_url":"http://p/500","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`, nil},
		// Issues Closed
		"GET " + pathMR1 + "/closes_issues": {http.StatusOK, `[{"id":10,"iid":5,"title":"Bug","state":"opened","author":{"username":"test"},"labels":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","web_url":"http://i/5","project_id":42}]`, defaultPgHdr()},
		// Cancel Auto Merge
		"POST " + pathMR1 + "/cancel_merge_when_pipeline_succeeds": {http.StatusOK, mrJSONCoverage, nil},
		// Subscribe / Unsubscribe
		"POST " + pathMR1 + "/subscribe":   {http.StatusOK, mrJSONCoverage, nil},
		"POST " + pathMR1 + "/unsubscribe": {http.StatusOK, mrJSONCoverage, nil},
		// Time tracking
		"POST " + pathMR1 + "/time_estimate":       {http.StatusOK, `{"human_time_estimate":"3h","human_total_time_spent":"","time_estimate":10800,"total_time_spent":0}`, nil},
		"POST " + pathMR1 + "/reset_time_estimate": {http.StatusOK, `{"human_time_estimate":"","human_total_time_spent":"","time_estimate":0,"total_time_spent":0}`, nil},
		"POST " + pathMR1 + "/add_spent_time":      {http.StatusCreated, `{"human_time_estimate":"","human_total_time_spent":"1h","time_estimate":0,"total_time_spent":3600}`, nil},
		"POST " + pathMR1 + "/reset_spent_time":    {http.StatusOK, `{"human_time_estimate":"","human_total_time_spent":"","time_estimate":0,"total_time_spent":0}`, nil},
		"GET " + pathMR1 + "/time_stats":           {http.StatusOK, `{"human_time_estimate":"3h","human_total_time_spent":"1h","time_estimate":10800,"total_time_spent":3600}`, nil},
		// Related Issues
		"GET " + pathMR1 + "/related_issues": {http.StatusOK, `[{"id":10,"iid":5,"title":"Related","state":"opened","author":{"username":"test"},"labels":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","web_url":"http://i/5","project_id":42}]`, defaultPgHdr()},
		// Todos & Dependencies
		"POST " + pathMR1 + "/todo":          {http.StatusCreated, todoJSONCoverage, nil},
		"POST " + pathMR1 + pathSuffixBlocks: {http.StatusCreated, dependencyJSONCoverage, nil},
		"GET " + pathMR1 + pathSuffixBlocks:  {http.StatusOK, `[` + dependencyJSONCoverage + `]`, nil},
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delete Dependency uses prefix match
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, pathMR1+pathSuffixBlocks) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

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
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: testVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// callToolAndVerify calls the named MCP tool and fails if it returns an error.
func callToolAndVerify(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
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
	session := newMRMCPSession(t)
	ctx := context.Background()

	pid := testProjectID
	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_create", map[string]any{"project_id": pid, "source_branch": testBranchFeat, "target_branch": testBranchMain, "title": "Test"}},
		{"gitlab_mr_get", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_list", map[string]any{"project_id": pid}},
		{"gitlab_mr_update", map[string]any{"project_id": pid, "mr_iid": 1, "title": "Updated"}},
		{"gitlab_mr_merge", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_approve", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_unapprove", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_commits", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_pipelines", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_delete", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_rebase", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_list_global", map[string]any{}},
		{"gitlab_mr_list_group", map[string]any{"group_id": "99"}},
		{"gitlab_mr_participants", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_reviewers", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_create_pipeline", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_issues_closed", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_cancel_auto_merge", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_subscribe", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_unsubscribe", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_set_time_estimate", map[string]any{"project_id": pid, "mr_iid": 1, "duration": "3h"}},
		{"gitlab_mr_reset_time_estimate", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_add_spent_time", map[string]any{"project_id": pid, "mr_iid": 1, "duration": "1h"}},
		{"gitlab_mr_reset_spent_time", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_time_stats", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_related_issues", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_create_todo", map[string]any{"project_id": pid, "mr_iid": 1}},
		{"gitlab_mr_dependency_create", map[string]any{"project_id": pid, "mr_iid": 1, "blocking_merge_request_id": 100}},
		{"gitlab_mr_dependency_delete", map[string]any{"project_id": pid, "mr_iid": 1, "blocking_merge_request_id": 100}},
		{"gitlab_mr_dependencies_list", map[string]any{"project_id": pid, "mr_iid": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			callToolAndVerify(t, session, ctx, tt.name, tt.args)
		})
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// Rich JSON responses to exercise ToOutput / populatePeople / populateTimestamps
// ---------------------------------------------------------------------------.

const mrJSONRichCoverage = `{
	"id":100,"iid":1,"project_id":42,"title":"Rich MR","state":"merged",
	"source_branch":"feat","target_branch":"main","web_url":"http://mr/1",
	"detailed_merge_status":"merged","draft":false,"has_conflicts":false,
	"sha":"abc123","merge_commit_sha":"def456","changes_count":"5",
	"rebase_in_progress":false,"user_notes_count":3,
	"description":"Full desc","merge_error":"",
	"blocking_discussions_resolved":true,"squash":true,
	"source_project_id":42,"target_project_id":42,
	"discussion_locked":true,"merge_when_pipeline_succeeds":true,
	"should_remove_source_branch":true,"force_remove_source_branch":true,
	"allow_collaboration":true,"squash_on_merge":true,"squash_commit_sha":"sq1",
	"upvotes":3,"downvotes":1,"subscribed":true,"first_contribution":true,
	"diverged_commits_count":2,
	"diff_refs":{"base_sha":"b1","head_sha":"h1","start_sha":"s1"},
	"pipeline":{"id":10,"web_url":"http://p/10","name":"pipeline1"},
	"head_pipeline":{"id":20},
	"latest_build_started_at":"2026-01-01T00:00:00Z",
	"latest_build_finished_at":"2026-01-02T00:00:00Z",
	"author":{"username":"alice"},
	"merged_by":{"username":"bob"},
	"merge_user":{"username":"bob"},
	"closed_by":{"username":"carol"},
	"assignees":[{"username":"dave"},{"username":"eve"}],
	"reviewers":[{"username":"frank"}],
	"labels":["bug","critical"],
	"milestone":{"title":"v1.0"},
	"task_completion_status":{"count":5,"completed_count":3},
	"time_stats":{"time_estimate":3600,"total_time_spent":1800},
	"merge_after":"2026-06-01T00:00:00Z",
	"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",
	"merged_at":"2026-01-03T00:00:00Z","closed_at":"2026-01-04T00:00:00Z",
	"prepared_at":"2026-01-05T00:00:00Z",
	"references":{"full":"group/project!1"}
}`

// TestGet_RichOutputFields verifies the behavior of get rich output fields.
func TestGet_RichOutputFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMR1 {
			testutil.RespondJSON(w, http.StatusOK, mrJSONRichCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}

	assertRichMRPipeline(t, out)
	assertRichMRPeople(t, out)
	assertRichMRTimestamps(t, out)
}

// assertRichMRPipeline is an internal helper for the mergerequests package.
func assertRichMRPipeline(t *testing.T, out Output) {
	t.Helper()
	if out.DiffRefs == nil {
		t.Fatal("DiffRefs should not be nil for rich MR")
	}
	if out.DiffRefs.BaseSHA != "b1" {
		t.Errorf("DiffRefs.BaseSHA = %q, want %q", out.DiffRefs.BaseSHA, "b1")
	}
	if out.PipelineID != 10 {
		t.Errorf("PipelineID = %d, want 10", out.PipelineID)
	}
	if out.PipelineWebURL != "http://p/10" {
		t.Errorf("PipelineWebURL = %q", out.PipelineWebURL)
	}
	if out.HeadPipelineID != 20 {
		t.Errorf("HeadPipelineID = %d, want 20", out.HeadPipelineID)
	}
	if out.LatestBuildStartedAt == "" {
		t.Error("LatestBuildStartedAt should not be empty")
	}
	if out.LatestBuildFinishedAt == "" {
		t.Error("LatestBuildFinishedAt should not be empty")
	}
}

// assertRichMRPeople is an internal helper for the mergerequests package.
func assertRichMRPeople(t *testing.T, out Output) {
	t.Helper()
	if out.MergedBy != testAuthorBob {
		t.Errorf("MergedBy = %q, want %q", out.MergedBy, testAuthorBob)
	}
	if out.ClosedBy != testAuthorCarol {
		t.Errorf("ClosedBy = %q, want %q", out.ClosedBy, testAuthorCarol)
	}
	if out.Milestone != testMilestoneV1 {
		t.Errorf("Milestone = %q, want %q", out.Milestone, testMilestoneV1)
	}
	if out.TaskCompletionCount != 3 {
		t.Errorf("TaskCompletionCount = %d, want 3", out.TaskCompletionCount)
	}
	if out.TaskCompletionTotal != 5 {
		t.Errorf("TaskCompletionTotal = %d, want 5", out.TaskCompletionTotal)
	}
	if out.TimeEstimate != 3600 {
		t.Errorf("TimeEstimate = %d, want 3600", out.TimeEstimate)
	}
	if out.TotalTimeSpent != 1800 {
		t.Errorf("TotalTimeSpent = %d, want 1800", out.TotalTimeSpent)
	}
}

// assertRichMRTimestamps is an internal helper for the mergerequests package.
func assertRichMRTimestamps(t *testing.T, out Output) {
	t.Helper()
	if out.MergeAfter == "" {
		t.Error("MergeAfter should not be empty")
	}
	if out.MergedAt == "" {
		t.Error("MergedAt should not be empty")
	}
	if out.ClosedAt == "" {
		t.Error("ClosedAt should not be empty")
	}
	if out.PreparedAt == "" {
		t.Error("PreparedAt should not be empty")
	}
	if out.References != "group/project!1" {
		t.Errorf("References = %q, want %q", out.References, "group/project!1")
	}
}

// ---------------------------------------------------------------------------
// Tests for Create with all optional fields to cover Create branches
// ---------------------------------------------------------------------------.

// TestCreate_AllOptionalFields verifies the behavior of create all optional fields.
func TestCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMRs {
			testutil.RespondJSON(w, http.StatusCreated, mrJSONCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:          testProjectID,
		SourceBranch:       testBranchFeat,
		TargetBranch:       testBranchMain,
		Title:              "Full MR",
		Description:        "Full desc",
		AssigneeIDs:        []int64{1, 2},
		ReviewerIDs:        []int64{3},
		RemoveSourceBranch: &boolTrue,
		Squash:             &boolTrue,
		MilestoneID:        10,
		AllowCollaboration: &boolTrue,
		TargetProjectID:    99,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// TestCreate_AssigneeIDSingular verifies that assignee_id (singular) is sent
// in the HTTP request body when creating a merge request.
func TestCreate_AssigneeIDSingular(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMRs {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			bodyStr := string(body)
			if !strings.Contains(bodyStr, `"assignee_id":28`) {
				t.Errorf("request body missing assignee_id: %s", bodyStr)
			}
			testutil.RespondJSON(w, http.StatusCreated, mrJSONCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		SourceBranch: testBranchFeat,
		TargetBranch: testBranchMain,
		Title:        "MR with single assignee",
		AssigneeID:   28,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// TestUpdate_AssigneeIDSingular verifies that assignee_id (singular) is sent
// in the HTTP request body when updating a merge request.
func TestUpdate_AssigneeIDSingular(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			bodyStr := string(body)
			if !strings.Contains(bodyStr, `"assignee_id":28`) {
				t.Errorf("request body missing assignee_id: %s", bodyStr)
			}
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  testProjectID,
		MRIID:      1,
		AssigneeID: 28,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// ---------------------------------------------------------------------------
// Tests for Merge with all optional fields
// ---------------------------------------------------------------------------.

// TestMerge_AllOptionalFields verifies the behavior of merge all optional fields.
func TestMerge_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge" {
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID:                testProjectID,
		MRIID:                    1,
		MergeCommitMessage:       "Merge commit",
		Squash:                   &boolTrue,
		ShouldRemoveSourceBranch: &boolTrue,
		AutoMerge:                &boolTrue,
		SHA:                      testSHAAbc,
		SquashCommitMessage:      "Squash msg",
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// ---------------------------------------------------------------------------
// Tests for Update with all optional fields to cover buildUpdateOpts
// ---------------------------------------------------------------------------.

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathMR1 {
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:          testProjectID,
		MRIID:              1,
		Title:              "Updated",
		Description:        "New desc",
		TargetBranch:       "develop",
		StateEvent:         "close",
		AssigneeIDs:        []int64{1},
		ReviewerIDs:        []int64{2},
		AddLabels:          "new-label",
		RemoveLabels:       "old-label",
		MilestoneID:        5,
		RemoveSourceBranch: &boolTrue,
		Squash:             &boolTrue,
		DiscussionLocked:   &boolTrue,
		AllowCollaboration: &boolTrue,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// ---------------------------------------------------------------------------
// Tests for List with all optional filter fields to cover buildListOptions
// ---------------------------------------------------------------------------.

// TestList_AllFilterFields verifies the behavior of list all filter fields.
func TestList_AllFilterFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathMRs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	_, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		State:           testStateOpened,
		Labels:          testLabels,
		NotLabels:       testLabelWontfix,
		Milestone:       testMilestoneV1,
		Scope:           "all",
		Search:          "login",
		SourceBranch:    testBranchFeat,
		TargetBranch:    testBranchMain,
		AuthorUsername:  testAuthorAlice,
		Draft:           &boolTrue,
		IIDs:            []int64{1, 2},
		CreatedAfter:    testCreatedAt,
		CreatedBefore:   testCreatedBefore,
		UpdatedAfter:    testCreatedAt,
		UpdatedBefore:   testCreatedBefore,
		OrderBy:         "created_at",
		Sort:            "desc",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 50},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for ListGlobal with all optional filter fields
// ---------------------------------------------------------------------------.

// TestListGlobal_AllFilterFields verifies the behavior of list global all filter fields.
func TestListGlobal_AllFilterFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	_, err := ListGlobal(context.Background(), client, ListGlobalInput{
		State:            testStateOpened,
		Labels:           testLabels,
		NotLabels:        testLabelWontfix,
		Milestone:        testMilestoneV1,
		Scope:            "all",
		Search:           "login",
		SourceBranch:     testBranchFeat,
		TargetBranch:     testBranchMain,
		AuthorUsername:   testAuthorAlice,
		ReviewerUsername: testAuthorBob,
		Draft:            &boolTrue,
		CreatedAfter:     testCreatedAt,
		CreatedBefore:    testCreatedBefore,
		UpdatedAfter:     testCreatedAt,
		UpdatedBefore:    testCreatedBefore,
		OrderBy:          "created_at",
		Sort:             "desc",
		PaginationInput:  toolutil.PaginationInput{Page: 1, PerPage: 50},
	})
	if err != nil {
		t.Fatalf("ListGlobal() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for ListGroup with all optional filter fields
// ---------------------------------------------------------------------------.

// TestListGroup_AllFilterFields verifies the behavior of list group all filter fields.
func TestListGroup_AllFilterFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/99/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:          "99",
		State:            testStateOpened,
		Labels:           testLabels,
		NotLabels:        testLabelWontfix,
		Milestone:        testMilestoneV1,
		Scope:            "all",
		Search:           "login",
		SourceBranch:     testBranchFeat,
		TargetBranch:     testBranchMain,
		AuthorUsername:   testAuthorAlice,
		ReviewerUsername: testAuthorBob,
		Draft:            &boolTrue,
		CreatedAfter:     testCreatedAt,
		CreatedBefore:    testCreatedBefore,
		UpdatedAfter:     testCreatedAt,
		UpdatedBefore:    testCreatedBefore,
		OrderBy:          "created_at",
		Sort:             "desc",
		PaginationInput:  toolutil.PaginationInput{Page: 1, PerPage: 50},
	})
	if err != nil {
		t.Fatalf("ListGroup() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// timeStatsToOutput nil case
// ---------------------------------------------------------------------------.

// TestTimeStatsToOutput_NilReturnsZero verifies the behavior of time stats to output nil returns zero.
func TestTimeStatsToOutput_NilReturnsZero(t *testing.T) {
	out := timeStatsToOutput(nil)
	if out.TimeEstimate != 0 || out.TotalTimeSpent != 0 {
		t.Error("timeStatsToOutput(nil) should return zero-value output")
	}
}

// ---------------------------------------------------------------------------
// Additional handler success tests (SetTimeEstimate, AddSpentTime with Summary)
// ---------------------------------------------------------------------------.

// TestSetTimeEstimate_EmptyDuration verifies the behavior of set time estimate empty duration.
func TestSetTimeEstimate_EmptyDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, MRIID: 1, Duration: ""})
	if err == nil {
		t.Fatal("SetTimeEstimate() expected error for empty duration, got nil")
	}
}

// TestAddSpentTime_WithSummary verifies the behavior of add spent time with summary.
func TestAddSpentTime_WithSummary(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/add_spent_time" {
			testutil.RespondJSON(w, http.StatusCreated, `{"human_time_estimate":"","human_total_time_spent":"2h","time_estimate":0,"total_time_spent":7200}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{
		ProjectID: testProjectID, MRIID: 1, Duration: "2h", Summary: "code review",
	})
	if err != nil {
		t.Fatalf("AddSpentTime() unexpected error: %v", err)
	}
	if out.TotalTimeSpent != 7200 {
		t.Errorf("TotalTimeSpent = %d, want 7200", out.TotalTimeSpent)
	}
}

// TestAddSpentTime_EmptyDuration verifies the behavior of add spent time empty duration.
func TestAddSpentTime_EmptyDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("AddSpentTime() expected error for empty duration, got nil")
	}
}

// TestGetTimeStats_MissingProject verifies the behavior of get time stats missing project.
func TestGetTimeStats_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetTimeStats(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Tests for Merge auto-detection of project merge requirements
// ---------------------------------------------------------------------------.

// TestMerge_AutoDetectsSquashOnMerge verifies that Merge automatically sets
// squash=true when the MR has squash_on_merge=true and the caller did not
// explicitly set the Squash field.
func TestMerge_AutoDetectsSquashOnMerge(t *testing.T) {
	mrWithSquashOnMerge := `{"id":100,"iid":1,"title":"Test MR","state":"opened","squash_on_merge":true,"force_remove_source_branch":false,"detailed_merge_status":"can_be_merged"}`

	var mergeRequestBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrWithSquashOnMerge)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			mergeRequestBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
	if !strings.Contains(mergeRequestBody, `"squash":true`) {
		t.Errorf("expected merge body to contain squash=true, got %s", mergeRequestBody)
	}
}

// TestMerge_AutoDetectsForceRemoveSourceBranch verifies that Merge sets
// should_remove_source_branch=true when force_remove_source_branch is true
// on the MR and the caller omitted the parameter.
func TestMerge_AutoDetectsForceRemoveSourceBranch(t *testing.T) {
	mrWithForceRemove := `{"id":100,"iid":1,"title":"Test MR","state":"opened","squash_on_merge":false,"force_remove_source_branch":true,"detailed_merge_status":"can_be_merged"}`

	var mergeRequestBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrWithForceRemove)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			mergeRequestBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
	if !strings.Contains(mergeRequestBody, `"should_remove_source_branch":true`) {
		t.Errorf("expected merge body to contain should_remove_source_branch=true, got %s", mergeRequestBody)
	}
}

// TestMerge_EnforcedSquashOverridesExplicitFalse verifies that when the MR
// has squash_on_merge=true (enforced by project settings), the Merge function
// overrides an explicit squash=false from the caller to prevent API rejection.
func TestMerge_EnforcedSquashOverridesExplicitFalse(t *testing.T) {
	mrWithSquashOnMerge := `{"id":100,"iid":1,"title":"Test MR","state":"opened","squash_on_merge":true,"force_remove_source_branch":false,"detailed_merge_status":"can_be_merged"}`

	var mergeRequestBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrWithSquashOnMerge)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			mergeRequestBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
		default:
			http.NotFound(w, r)
		}
	}))

	boolFalse := false
	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID:                testProjectID,
		MRIID:                    1,
		Squash:                   &boolFalse,
		ShouldRemoveSourceBranch: &boolFalse,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
	if !strings.Contains(mergeRequestBody, `"squash":true`) {
		t.Errorf("expected enforced squash_on_merge to override explicit false, got body: %s", mergeRequestBody)
	}
}

// TestMerge_ExplicitSquashRespectedWhenNotEnforced verifies that when the MR
// has squash_on_merge=false, an explicit squash=true from the caller is
// preserved in the merge request.
func TestMerge_ExplicitSquashRespectedWhenNotEnforced(t *testing.T) {
	mrNoSquash := `{"id":100,"iid":1,"title":"Test MR","state":"opened","squash_on_merge":false,"force_remove_source_branch":false,"detailed_merge_status":"can_be_merged"}`

	var mergeRequestBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrNoSquash)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			mergeRequestBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
		default:
			http.NotFound(w, r)
		}
	}))

	boolTrue := true
	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
		Squash:    &boolTrue,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
	if !strings.Contains(mergeRequestBody, `"squash":true`) {
		t.Errorf("expected explicit squash=true to be preserved, got body: %s", mergeRequestBody)
	}
}

// TestMerge_AutoDetectGetFailsContinues verifies that if the pre-fetch GET
// fails, Merge still proceeds with the merge attempt without auto-detected values.
func TestMerge_AutoDetectGetFailsContinues(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf(fmtIIDWant, out.IID)
	}
}

// TestMerge_405PipelineRunning verifies that when merge returns 405 and the
// pre-fetched MR has detailed_merge_status=ci_still_running, the error message
// includes the pipeline blocker and suggests auto_merge.
func TestMerge_405PipelineRunning(t *testing.T) {
	mrRunning := `{"id":100,"iid":1,"title":"Test MR","state":"opened","detailed_merge_status":"ci_still_running","has_conflicts":false,"blocking_discussions_resolved":true,"draft":false}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrRunning)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ci_still_running") && !strings.Contains(err.Error(), "pipeline is still running") {
		t.Errorf("expected pipeline-related diagnostic, got: %v", err)
	}
	if !strings.Contains(err.Error(), "auto_merge") {
		t.Errorf("expected auto_merge suggestion, got: %v", err)
	}
}

// TestMerge_405Draft verifies that merging a draft MR returns a diagnostic
// error mentioning the draft status.
func TestMerge_405Draft(t *testing.T) {
	mrDraft := `{"id":100,"iid":1,"title":"Draft: Test MR","state":"opened","detailed_merge_status":"draft_status","has_conflicts":false,"blocking_discussions_resolved":true,"draft":true}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrDraft)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "draft") {
		t.Errorf("expected draft diagnostic, got: %v", err)
	}
}

// TestMerge_405Conflicts verifies that merging an MR with conflicts returns
// a diagnostic error mentioning conflicts.
func TestMerge_405Conflicts(t *testing.T) {
	mrConflict := `{"id":100,"iid":1,"title":"Test MR","state":"opened","detailed_merge_status":"conflict","has_conflicts":true,"blocking_discussions_resolved":true,"draft":false}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrConflict)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("expected conflict diagnostic, got: %v", err)
	}
}

// TestMerge_405UnresolvedDiscussions verifies that merging an MR with unresolved
// discussions returns a diagnostic error mentioning discussions.
func TestMerge_405UnresolvedDiscussions(t *testing.T) {
	mrDiscussions := `{"id":100,"iid":1,"title":"Test MR","state":"opened","detailed_merge_status":"discussions_not_resolved","has_conflicts":false,"blocking_discussions_resolved":false,"draft":false}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrDiscussions)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "discussion") {
		t.Errorf("expected discussions diagnostic, got: %v", err)
	}
}

// TestMerge_405NotApproved verifies that merging an unapproved MR returns
// a diagnostic error about missing approvals.
func TestMerge_405NotApproved(t *testing.T) {
	mrNotApproved := `{"id":100,"iid":1,"title":"Test MR","state":"opened","detailed_merge_status":"not_approved","has_conflicts":false,"blocking_discussions_resolved":true,"draft":false}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrNotApproved)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "approv") {
		t.Errorf("expected approval diagnostic, got: %v", err)
	}
}

// TestMerge_405FallbackWhenPrefetchFails verifies that when both the pre-fetch
// GET and merge PUT fail, the error falls back to the generic WrapErr message
// without diagnostics.
func TestMerge_405FallbackWhenPrefetchFails(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	// Should fall back to generic WrapErr since pre-fetch failed
	if !strings.Contains(err.Error(), "mrMerge") {
		t.Errorf("expected mrMerge prefix, got: %v", err)
	}
}

// TestMerge_405MultipleBlockers verifies that when multiple blockers exist,
// they are all listed in the error message.
func TestMerge_405MultipleBlockers(t *testing.T) {
	mrMultiple := `{"id":100,"iid":1,"title":"Draft: Test","state":"opened","detailed_merge_status":"ci_still_running","has_conflicts":true,"blocking_discussions_resolved":false,"draft":true}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathMR1:
			testutil.RespondJSON(w, http.StatusOK, mrMultiple)
		case r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge":
			testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID: testProjectID,
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("Merge() expected error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "pipeline") {
		t.Errorf("expected pipeline blocker, got: %v", err)
	}
	if !strings.Contains(errMsg, "draft") {
		t.Errorf("expected draft blocker, got: %v", err)
	}
	if !strings.Contains(errMsg, "conflict") {
		t.Errorf("expected conflict blocker, got: %v", err)
	}
	if !strings.Contains(errMsg, "discussion") {
		t.Errorf("expected discussion blocker, got: %v", err)
	}
}

// TestFormatMarkdown_MergedState covers merged-by, closed-by, ProjectPath,
// Milestone, PipelineID with web URL, ChangesCount, and Description branches.
func TestFormatMarkdown_MergedState(t *testing.T) {
	md := FormatMarkdown(Output{
		IID: 5, Title: "merged MR", State: "merged",
		SourceBranch: "feat", TargetBranch: "main",
		MergeStatus: "merged", ProjectPath: "group/project",
		Milestone: "v1.0", PipelineID: 42, PipelineWebURL: "https://pipeline",
		ChangesCount: "3", MergedBy: "alice", MergedAt: testCreatedAt,
	})
	for _, want := range []string{"group/project", "v1.0", "[#42](https://pipeline)", "3 files", "@alice"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in merged MR markdown", want)
		}
	}
}

func TestFormatMarkdown_ClosedState(t *testing.T) {
	md := FormatMarkdown(Output{
		IID: 6, Title: "closed MR", State: "closed",
		SourceBranch: "fix", TargetBranch: "main", MergeStatus: "cannot_be_merged",
		ClosedBy: "bob", ClosedAt: testCreatedAt,
		PipelineID: 99,
	})
	if !strings.Contains(md, "@bob") {
		t.Error("expected closed-by in output")
	}
	if !strings.Contains(md, "#99") {
		t.Error("expected pipeline ID without URL")
	}
}

// TestApprove_Forbidden covers the 403 auth/permissions hint branch.
func TestApprove_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Approve(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "self-approval") {
		t.Errorf("expected self-approval hint, got: %v", err)
	}
}

func TestApprove_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := Approve(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Premium") {
		t.Errorf("expected Premium hint, got: %v", err)
	}
}

func TestApprove_GenericError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Approve(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestSubscribe_EOF covers the 304/EOF fallback to Get.
func TestSubscribe_EOF(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/subscribe") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, mrJSONMinimalMR)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := Subscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IID != 3 {
		t.Errorf("IID = %d, want 3", out.IID)
	}
}

func TestSubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Subscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnsubscribe_EOF(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/unsubscribe") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, mrJSONMinimalMR)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := Unsubscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IID != 3 {
		t.Errorf("IID = %d, want 3", out.IID)
	}
}

func TestUnsubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Unsubscribe(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gitlab_mr_list") {
		t.Errorf("expected hint, got: %v", err)
	}
}

func TestUpdate_GenericError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRebase_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Rebase(context.Background(), client, RebaseInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCommits_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Commits(context.Background(), client, CommitsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPipelines_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Pipelines(context.Background(), client, PipelinesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnapprove_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	err := Unapprove(context.Background(), client, ApproveInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCancelAutoMerge_MethodNotAllowed(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusMethodNotAllowed, `{"message":"405"}`)
	}))
	_, err := CancelAutoMerge(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auto_merge_enabled") {
		t.Errorf("expected auto_merge hint, got: %v", err)
	}
}

func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID, SourceBranch: "feat", TargetBranch: "main", Title: "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetTimeEstimate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{
		ProjectID: testProjectID, MRIID: 1, Duration: "1h",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResetTimeEstimate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ResetTimeEstimate(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddSpentTime_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{
		ProjectID: testProjectID, MRIID: 1, Duration: "2h",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResetSpentTime_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ResetSpentTime(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTimeStats_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := GetTimeStats(context.Background(), client, GetInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParticipants_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Participants(context.Background(), client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReviewers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Reviewers(context.Background(), client, ParticipantsInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreatePipeline_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := CreatePipeline(context.Background(), client, CreatePipelineInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIssuesClosed_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := IssuesClosed(context.Background(), client, IssuesClosedInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRelatedIssues_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := RelatedIssues(context.Background(), client, RelatedIssuesInput{ProjectID: testProjectID, MRIID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

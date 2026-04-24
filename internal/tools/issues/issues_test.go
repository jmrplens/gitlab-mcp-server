// issues_test.go contains unit tests for GitLab issue operations (create,
// get, list, update, delete). Tests use httptest to mock the GitLab API
// and verify success paths, error handling, filtering, pagination, and
// context cancellation.

package issues

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Test endpoint paths and JSON response fixtures for issue operation tests.
const (
	errExpMissingProjectID = "expected error for missing project_id"
	pathIssues             = "/api/v4/projects/42/issues"
	issueJSONMinimal       = `{"id":1,"iid":10,"title":"Test issue","description":"","state":"opened","labels":[],"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`
	issueJSONFull          = `{"id":1,"iid":10,"title":"Bug: login fails","description":"Login page returns an error","state":"opened","labels":["bug","critical"],"assignees":[{"username":"alice"},{"username":"bob"}],"milestone":{"title":"v1.0"},"author":{"username":"charlie"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-16T12:00:00Z","due_date":"2026-02-01"}`
	issueJSONClosed        = `{"id":1,"iid":10,"title":"Bug: login fails","description":"Login page returns an error","state":"closed","labels":["bug"],"assignees":[],"author":{"username":"charlie"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-20T09:00:00Z","closed_at":"2026-01-20T09:00:00Z"}`
	fmtIssueStateWant      = "out.State = %q, want %q"
	pathIssue10            = "/api/v4/projects/42/issues/10"
	testDueDate            = "2026-02-01"
	fmtIssueListErr        = "List() unexpected error: %v"
	fmtIssueGetErr         = "Get() unexpected error: %v"
	fmtIssueUpdateErr      = "Update() unexpected error: %v"
	msgConfidentialWant    = "out.Confidential = false, want true"
	fmtTaskTotalWant       = "out.TaskCompletionTotal = %d, want 5"
	testIssueTitle         = "Test issue"
	fmtCreateErr           = "Create() unexpected error: %v"
	fmtIIDWant10           = "out.IID = %d, want 10"
	fmtIssueCountWant1     = "len(out.Issues) = %d, want 1"

	// issueJSONEnriched includes all enriched fields: confidential, task_completion_status, user_notes_count.
	issueJSONEnriched = `{
		"id":1,"iid":10,
		"title":"Secure login implementation",
		"description":"Implement OAuth2 login with PKCE",
		"state":"opened",
		"labels":["security","feature"],
		"assignees":[{"username":"alice"},{"username":"bob"}],
		"milestone":{"title":"v2.0"},
		"author":{"username":"charlie"},
		"web_url":"https://gitlab.example.com/project/issues/10",
		"created_at":"2026-02-01T10:00:00Z",
		"updated_at":"2026-02-15T14:30:00Z",
		"due_date":"2026-03-01",
		"confidential":true,
		"user_notes_count":8,
		"task_completion_status":{"count":5,"completed_count":3}
	}`

	// issueJSONNoTasks has no task_completion_status and confidential=false.
	issueJSONNoTasks = `{
		"id":2,"iid":20,
		"title":"Simple bug",
		"description":"A minor issue",
		"state":"opened",
		"labels":[],
		"assignees":[],
		"author":{"username":"dave"},
		"web_url":"https://gitlab.example.com/project/issues/20",
		"created_at":"2026-02-10T08:00:00Z",
		"updated_at":"2026-02-10T08:00:00Z",
		"confidential":false,
		"user_notes_count":0
	}`

	// issueJSONClosedEnriched includes task completion in a closed issue.
	issueJSONClosedEnriched = `{
		"id":3,"iid":30,
		"title":"Completed task",
		"description":"All subtasks done",
		"state":"closed",
		"labels":["done"],
		"assignees":[{"username":"eve"}],
		"author":{"username":"frank"},
		"web_url":"https://gitlab.example.com/project/issues/30",
		"created_at":"2026-01-01T10:00:00Z",
		"updated_at":"2026-02-20T16:00:00Z",
		"closed_at":"2026-02-20T16:00:00Z",
		"confidential":false,
		"user_notes_count":15,
		"task_completion_status":{"count":3,"completed_count":3}
	}`
)

// TestCreate_Success verifies that Create correctly creates an issue
// with all optional fields (labels, assignees, due date, milestone). The mock
// returns a full issue JSON and the test asserts all output fields match.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			testutil.RespondJSON(w, http.StatusCreated, issueJSONFull)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   testProjectID,
		Title:       "Bug: login fails",
		Description: "Login page returns an error",
		Labels:      "bug,critical",
		AssigneeIDs: []int64{1, 2},
		DueDate:     testDueDate,
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
	if out.State != "opened" {
		t.Errorf(fmtIssueStateWant, out.State, "opened")
	}
	if len(out.Labels) != 2 {
		t.Errorf("len(out.Labels) = %d, want 2", len(out.Labels))
	}
	if len(out.Assignees) != 2 {
		t.Errorf("len(out.Assignees) = %d, want 2", len(out.Assignees))
	}
	if out.Milestone != "v1.0" {
		t.Errorf("out.Milestone = %q, want %q", out.Milestone, "v1.0")
	}
	if out.DueDate != testDueDate {
		t.Errorf("out.DueDate = %q, want %q", out.DueDate, testDueDate)
	}
}

// TestCreate_InvalidDueDate verifies that Create returns an error
// when the due_date field has an invalid format. The mock should never be
// called because validation occurs before the API request.
func TestCreate_InvalidDueDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called for invalid due_date")
		http.NotFound(w, nil)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		Title:     "test",
		DueDate:   "not-a-date",
	})
	if err == nil {
		t.Fatal("Create() expected error for invalid due_date, got nil")
	}
	if !strings.Contains(err.Error(), "invalid due_date") {
		t.Errorf("error = %q, want it to contain 'invalid due_date'", err.Error())
	}
}

// TestCreate_APIError verifies that Create propagates a 403 Forbidden
// error returned by the GitLab API.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		Title:     "test",
	})
	if err == nil {
		t.Fatal("Create() expected error for 403, got nil")
	}
}

// TestCreate_CancelledContext verifies that Create returns an error
// immediately when called with an already-canceled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, Title: "test"})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// TestGet_Success verifies that Get retrieves a single issue by IID
// and correctly maps author, web URL, and other fields to the output struct.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, issueJSONFull)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
	if out.Author != "charlie" {
		t.Errorf("out.Author = %q, want %q", out.Author, "charlie")
	}
	if out.WebURL != "https://gitlab.example.com/project/issues/10" {
		t.Errorf("out.WebURL = %q, want expected URL", out.WebURL)
	}
}

// TestGet_NotFound verifies that Get returns an error when the
// GitLab API responds with 404 for a non-existent issue.
func TestGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 9999})
	if err == nil {
		t.Fatal("Get() expected error for non-existent issue, got nil")
	}
}

// TestList_WithFilters verifies that List correctly passes state and
// search query parameters to the GitLab API and returns matching issues with
// pagination metadata.
func TestList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			q := r.URL.Query()
			if q.Get("state") != "opened" {
				t.Errorf("expected state=opened, got %q", q.Get("state"))
			}
			if q.Get("search") != "login" {
				t.Errorf("expected search=login, got %q", q.Get("search"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+issueJSONMinimal+`]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "1", TotalPages: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		State:     "opened",
		Search:    "login",
	})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtIssueCountWant1, len(out.Issues))
	}
	if out.Issues[0].IID != 10 {
		t.Errorf("out.Issues[0].IID = %d, want 10", out.Issues[0].IID)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("out.Pagination.TotalItems = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestList_ByLabels verifies that List forwards comma-separated
// label filters to the GitLab API query parameters.
func TestList_ByLabels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			q := r.URL.Query()
			if q.Get("labels") != "bug,critical" {
				t.Errorf("expected labels=bug,critical, got %q", q.Get("labels"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+issueJSONFull+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		Labels:    "bug,critical",
	})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtIssueCountWant1, len(out.Issues))
	}
}

// TestList_Empty verifies that List returns an empty slice when
// the GitLab API returns no issues.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 0 {
		t.Errorf("len(out.Issues) = %d, want 0", len(out.Issues))
	}
}

// TestList_Pagination verifies that List correctly forwards page
// and per_page parameters to the API and parses pagination response headers.
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			q := r.URL.Query()
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %q", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", q.Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+issueJSONMinimal+`]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "5", Total: "6", TotalPages: "2", PrevPage: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("out.Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("out.Pagination.TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// TestUpdate_StateClose verifies that Update transitions an issue
// to the closed state and that the ClosedAt timestamp is populated.
func TestUpdate_StateClose(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, issueJSONClosed)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  testProjectID,
		IssueIID:   10,
		StateEvent: "close",
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if out.State != "closed" {
		t.Errorf(fmtIssueStateWant, out.State, "closed")
	}
	if out.ClosedAt == "" {
		t.Error("out.ClosedAt should not be empty for closed issue")
	}
}

// TestUpdate_Labels verifies that Update supports adding labels
// without removing existing ones via the AddLabels field.
func TestUpdate_Labels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":10,"title":"Bug: login fails","description":"","state":"opened","labels":["bug","critical","urgent"],"assignees":[],"author":{"username":"charlie"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-16T12:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		AddLabels: "urgent",
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if len(out.Labels) != 3 {
		t.Errorf("len(out.Labels) = %d, want 3", len(out.Labels))
	}
}

// TestUpdate_APIError verifies that Update propagates a 403 Forbidden
// error returned by the GitLab API.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  testProjectID,
		IssueIID:   10,
		StateEvent: "close",
	})
	if err == nil {
		t.Fatal("Update() expected error for 403, got nil")
	}
}

// TestDelete_Success verifies that Delete completes without error
// when the GitLab API responds with 204 No Content.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathIssue10 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_NotFound verifies that Delete returns an error when
// the GitLab API responds with 404 for a non-existent issue.
func TestDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueIID: 9999})
	if err == nil {
		t.Fatal("Delete() expected error for non-existent issue, got nil")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error
// immediately when called with an already-canceled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("Delete() expected error for canceled context, got nil")
	}
}

// TestToOutput_EmptyLabels verifies that ToOutput normalizes null
// labels from the API response to an empty slice instead of nil.
func TestToOutput_EmptyLabels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":10,"title":"no labels","description":"","state":"opened","labels":null,"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}
	if out.Labels == nil {
		t.Error("out.Labels should be empty slice, not nil")
	}
	if len(out.Labels) != 0 {
		t.Errorf("len(out.Labels) = %d, want 0", len(out.Labels))
	}
}

// TestListGroup_Success verifies that ListGroup returns issues for a group.
func TestListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10/issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"iid":5,"title":"group issue","description":"desc","state":"opened","labels":["bug"],"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/group/project/issues/5","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: "10",
	})
	if err != nil {
		t.Fatalf("ListGroup() unexpected error: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
	}
	if out.Issues[0].Title != "group issue" {
		t.Errorf("Issues[0].Title = %q, want %q", out.Issues[0].Title, "group issue")
	}
}

// TestListGroup_EmptyGroupID verifies ListGroup returns error for empty group_id.
func TestListGroup_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListGroup_CancelledContext verifies ListGroup handles canceled context.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestGet_EnrichedFields verifies that Get populates all enriched
// fields: confidential, task_completion_status, and user_notes_count.
func TestGet_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, issueJSONEnriched)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}

	if !out.Confidential {
		t.Error(msgConfidentialWant)
	}
	if out.TaskCompletionTotal != 5 {
		t.Errorf(fmtTaskTotalWant, out.TaskCompletionTotal)
	}
	if out.TaskCompletionCount != 3 {
		t.Errorf("out.TaskCompletionCount = %d, want 3", out.TaskCompletionCount)
	}
	if out.UserNotesCount != 8 {
		t.Errorf("out.UserNotesCount = %d, want 8", out.UserNotesCount)
	}
	if len(out.Labels) != 2 {
		t.Errorf("len(out.Labels) = %d, want 2", len(out.Labels))
	}
	if len(out.Assignees) != 2 {
		t.Errorf("len(out.Assignees) = %d, want 2", len(out.Assignees))
	}
	if out.Milestone != "v2.0" {
		t.Errorf("out.Milestone = %q, want %q", out.Milestone, "v2.0")
	}
	if out.DueDate != "2026-03-01" {
		t.Errorf("out.DueDate = %q, want %q", out.DueDate, "2026-03-01")
	}
}

// TestGet_NoTaskCompletion verifies that task completion fields default to
// zero when the API does not return task_completion_status.
func TestGet_NoTaskCompletion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues/20" {
			testutil.RespondJSON(w, http.StatusOK, issueJSONNoTasks)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 20})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}
	if out.Confidential {
		t.Error("out.Confidential = true, want false")
	}
	if out.TaskCompletionTotal != 0 {
		t.Errorf("out.TaskCompletionTotal = %d, want 0", out.TaskCompletionTotal)
	}
	if out.TaskCompletionCount != 0 {
		t.Errorf("out.TaskCompletionCount = %d, want 0", out.TaskCompletionCount)
	}
	if out.UserNotesCount != 0 {
		t.Errorf("out.UserNotesCount = %d, want 0", out.UserNotesCount)
	}
}

// TestGet_ClosedWithTasks verifies enriched fields on a closed issue with
// completed tasks.
func TestGet_ClosedWithTasks(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues/30" {
			testutil.RespondJSON(w, http.StatusOK, issueJSONClosedEnriched)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 30})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}
	if out.State != "closed" {
		t.Errorf(fmtIssueStateWant, out.State, "closed")
	}
	if out.ClosedAt == "" {
		t.Error("out.ClosedAt should not be empty for closed issue")
	}
	if out.TaskCompletionTotal != 3 {
		t.Errorf("out.TaskCompletionTotal = %d, want 3", out.TaskCompletionTotal)
	}
	if out.TaskCompletionCount != 3 {
		t.Errorf("out.TaskCompletionCount = %d, want 3 (all completed)", out.TaskCompletionCount)
	}
	if out.UserNotesCount != 15 {
		t.Errorf("out.UserNotesCount = %d, want 15", out.UserNotesCount)
	}
}

// TestCreate_EnrichedFields verifies that Create returns enriched
// fields from the API response including confidential and notes count.
func TestCreate_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			testutil.RespondJSON(w, http.StatusCreated, issueJSONEnriched)
			return
		}
		http.NotFound(w, r)
	}))

	conf := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		Title:        "Secure login implementation",
		Confidential: &conf,
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if !out.Confidential {
		t.Error(msgConfidentialWant)
	}
	if out.Author != "charlie" {
		t.Errorf("out.Author = %q, want %q", out.Author, "charlie")
	}
	if out.TaskCompletionTotal != 5 {
		t.Errorf(fmtTaskTotalWant, out.TaskCompletionTotal)
	}
	if out.UserNotesCount != 8 {
		t.Errorf("out.UserNotesCount = %d, want 8", out.UserNotesCount)
	}
}

// TestList_EnrichedFields verifies that List populates enriched fields
// across multiple issues, including mixed confidential and task states.
func TestList_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			testutil.RespondJSON(w, http.StatusOK, `[`+issueJSONEnriched+`,`+issueJSONNoTasks+`,`+issueJSONClosedEnriched+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 3 {
		t.Fatalf("len(out.Issues) = %d, want 3", len(out.Issues))
	}

	if !out.Issues[0].Confidential {
		t.Error("Issues[0].Confidential = false, want true")
	}
	if out.Issues[0].TaskCompletionTotal != 5 {
		t.Errorf("Issues[0].TaskCompletionTotal = %d, want 5", out.Issues[0].TaskCompletionTotal)
	}

	if out.Issues[1].Confidential {
		t.Error("Issues[1].Confidential = true, want false")
	}
	if out.Issues[1].TaskCompletionTotal != 0 {
		t.Errorf("Issues[1].TaskCompletionTotal = %d, want 0", out.Issues[1].TaskCompletionTotal)
	}

	if out.Issues[2].State != "closed" {
		t.Errorf("Issues[2].State = %q, want %q", out.Issues[2].State, "closed")
	}
	if out.Issues[2].TaskCompletionCount != 3 {
		t.Errorf("Issues[2].TaskCompletionCount = %d, want 3", out.Issues[2].TaskCompletionCount)
	}
	if out.Issues[2].UserNotesCount != 15 {
		t.Errorf("Issues[2].UserNotesCount = %d, want 15", out.Issues[2].UserNotesCount)
	}
}

// TestUpdate_EnrichedFields verifies that Update returns enriched
// fields in the response after updating an issue.
func TestUpdate_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, issueJSONEnriched)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    testProjectID,
		IssueIID:     10,
		Confidential: new(true),
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if !out.Confidential {
		t.Error(msgConfidentialWant)
	}
	if out.TaskCompletionTotal != 5 {
		t.Errorf(fmtTaskTotalWant, out.TaskCompletionTotal)
	}
}

// assertEnrichedInputBody decodes the request body and verifies the enriched
// input fields (created_at, merge_request_to_resolve_discussions_of,
// discussion_to_resolve) are correctly passed to the GitLab API.
func assertEnrichedInputBody(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}
	if v, ok := body["created_at"].(string); !ok || !strings.Contains(v, "2026") {
		t.Errorf("created_at = %v, want value containing '2026'", body["created_at"])
	}
	if v, ok := body["merge_request_to_resolve_discussions_of"].(float64); !ok || int64(v) != 5 {
		t.Errorf("merge_request_to_resolve_discussions_of = %v, want 5", body["merge_request_to_resolve_discussions_of"])
	}
	if v, ok := body["discussion_to_resolve"].(string); !ok || v != "abc123" {
		t.Errorf("discussion_to_resolve = %v, want %q", body["discussion_to_resolve"], "abc123")
	}
}

// TestCreate_EnrichedInputFields verifies that Create passes the new
// input fields (CreatedAt, MergeRequestToResolveDiscussionsOf, DiscussionToResolve)
// to the GitLab API request body.
func TestCreate_EnrichedInputFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			assertEnrichedInputBody(t, r)
			testutil.RespondJSON(w, http.StatusCreated, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:                          testProjectID,
		Title:                              "Resolve discussion issue",
		CreatedAt:                          "2026-03-01T10:00:00Z",
		MergeRequestToResolveDiscussionsOf: 5,
		DiscussionToResolve:                "abc123",
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
}

// TestGet_EpicIssueID verifies that Get maps the epic_issue_id field.
func TestGet_EpicIssueID(t *testing.T) {
	issueWithEpic := `{
		"id":1,"iid":10,
		"title":"Epic linked issue",
		"description":"","state":"opened",
		"labels":[],"assignees":[],
		"author":{"username":"alice"},
		"web_url":"https://gitlab.example.com/project/issues/10",
		"created_at":"2026-01-15T10:00:00Z",
		"updated_at":"2026-01-15T10:00:00Z",
		"epic_issue_id":42
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssue10 {
			testutil.RespondJSON(w, http.StatusOK, issueWithEpic)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf(fmtIssueGetErr, err)
	}
	if out.EpicIssueID != 42 {
		t.Errorf("out.EpicIssueID = %d, want 42", out.EpicIssueID)
	}
}

// TASK-021: ListAll, GetByID, Reorder, Move, Subscribe, Unsubscribe, CreateTodo.

const (
	pathGlobalIssues = "/api/v4/issues"
	pathIssueByID    = "/api/v4/issues/99"
	pathReorder      = "/api/v4/projects/42/issues/10/reorder"
	pathMove         = "/api/v4/projects/42/issues/10/move"
	pathSubscribe    = "/api/v4/projects/42/issues/10/subscribe"
	pathUnsubscribe  = "/api/v4/projects/42/issues/10/unsubscribe"
	pathCreateTodo   = "/api/v4/projects/42/issues/10/todo"

	todoJSON = `{"id":501,"action_name":"marked","target_type":"Issue","target":{"title":"Test issue","web_url":"https://gitlab.example.com/project/issues/10"},"body":"marked todo","state":"pending","created_at":"2026-03-01T10:00:00Z"}`
)

// TestListAll_Success verifies the behavior of list all success.
func TestListAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGlobalIssues && r.Method == http.MethodGet {
			testutil.AssertQueryParam(t, r, "state", "opened")
			testutil.RespondJSON(w, http.StatusOK, "["+issueJSONMinimal+"]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListAll(context.Background(), client, ListAllInput{State: "opened"})
	if err != nil {
		t.Fatalf("ListAll() unexpected error: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtIssueCountWant1, len(out.Issues))
	}
	if out.Issues[0].Title != testIssueTitle {
		t.Errorf("out.Issues[0].Title = %q, want %q", out.Issues[0].Title, testIssueTitle)
	}
}

// TestListAll_CancelledContext verifies the behavior of list all cancelled context.
func TestListAll_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListAll(ctx, client, ListAllInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestGetByID_Success verifies the behavior of get by i d success.
func TestGetByID_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssueByID && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetByID(context.Background(), client, GetByIDInput{IssueID: 99})
	if err != nil {
		t.Fatalf("GetByID() unexpected error: %v", err)
	}
	if out.Title != testIssueTitle {
		t.Errorf("out.Title = %q, want %q", out.Title, testIssueTitle)
	}
}

// TestGetByID_MissingID verifies the behavior of get by i d missing i d.
func TestGetByID_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetByID(context.Background(), client, GetByIDInput{})
	if err == nil {
		t.Fatal("expected error for missing issue_id")
	}
	if !strings.Contains(err.Error(), "issue_id is required") {
		t.Errorf("error = %q, want issue_id is required", err.Error())
	}
}

// TestReorder_Success verifies the behavior of reorder success.
func TestReorder_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReorder && r.Method == http.MethodPut {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	afterID := int64(5)
	out, err := Reorder(context.Background(), client, ReorderInput{ProjectID: testProjectID, IssueIID: 10, MoveAfterID: &afterID})
	if err != nil {
		t.Fatalf("Reorder() unexpected error: %v", err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
}

// TestReorder_MissingProjectID verifies the behavior of reorder missing project i d.
func TestReorder_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Reorder(context.Background(), client, ReorderInput{IssueIID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestMove_Success verifies the behavior of move success.
func TestMove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMove && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Move(context.Background(), client, MoveInput{ProjectID: testProjectID, IssueIID: 10, ToProjectID: 99})
	if err != nil {
		t.Fatalf("Move() unexpected error: %v", err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
}

// TestMove_MissingToProject verifies the behavior of move missing to project.
func TestMove_MissingToProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Move(context.Background(), client, MoveInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("expected error for missing to_project_id")
	}
	if !strings.Contains(err.Error(), "to_project_id is required") {
		t.Errorf("error = %q, want to_project_id is required", err.Error())
	}
}

// TestSubscribe_Success verifies the behavior of subscribe success.
func TestSubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathSubscribe && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":10,"title":"Test issue","state":"opened","labels":[],"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z","subscribed":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Subscribe(context.Background(), client, SubscribeInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	if !out.Subscribed {
		t.Error("out.Subscribed = false, want true")
	}
}

// TestSubscribe_MissingProjectID verifies the behavior of subscribe missing project i d.
func TestSubscribe_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Subscribe(context.Background(), client, SubscribeInput{IssueIID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestUnsubscribe_Success verifies the behavior of unsubscribe success.
func TestUnsubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathUnsubscribe && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":10,"title":"Test issue","state":"opened","labels":[],"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z","subscribed":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Unsubscribe(context.Background(), client, UnsubscribeInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Unsubscribe() unexpected error: %v", err)
	}
	if out.Subscribed {
		t.Error("out.Subscribed = true, want false")
	}
}

// TestUnsubscribe_MissingProjectID verifies the behavior of unsubscribe missing project i d.
func TestUnsubscribe_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Unsubscribe(context.Background(), client, UnsubscribeInput{IssueIID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestCreateTodo_Success verifies the behavior of create todo success.
func TestCreateTodo_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathCreateTodo && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, todoJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateTodo(context.Background(), client, CreateTodoInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("CreateTodo() unexpected error: %v", err)
	}
	if out.ID != 501 {
		t.Errorf("out.ID = %d, want 501", out.ID)
	}
	if out.ActionName != "marked" {
		t.Errorf("out.ActionName = %q, want %q", out.ActionName, "marked")
	}
	if out.TargetTitle != testIssueTitle {
		t.Errorf("out.TargetTitle = %q, want %q", out.TargetTitle, testIssueTitle)
	}
	if out.State != "pending" {
		t.Errorf("out.State = %q, want %q", out.State, "pending")
	}
}

// TestCreateTodo_MissingProjectID verifies the behavior of create todo missing project i d.
func TestCreateTodo_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateTodo(context.Background(), client, CreateTodoInput{IssueIID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TASK-022: Time Tracking, Participants, Closing/Related MRs tests.

const (
	timeStatsIssueResponse = `{"human_time_estimate":"3h","human_total_time_spent":"1h30m","time_estimate":10800,"total_time_spent":5400}`
	participantsResponse   = `[{"id":1,"username":"alice","name":"Alice Dev","web_url":"https://gitlab.example.com/alice"},{"id":2,"username":"bob","name":"Bob QA","web_url":"https://gitlab.example.com/bob"}]`
	closingMRsResponse     = `[{"id":100,"iid":5,"title":"Fix login","state":"merged","source_branch":"fix-login","target_branch":"main","author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/-/merge_requests/5"}]`
	relatedMRsResponse     = `[{"id":200,"iid":8,"title":"Refactor auth","state":"opened","source_branch":"refactor-auth","target_branch":"main","author":{"username":"bob"},"web_url":"https://gitlab.example.com/project/-/merge_requests/8"}]`
)

// TestSetTimeEstimate_Success verifies the behavior of set time estimate success.
func TestSetTimeEstimate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssue10+"/time_estimate" {
			testutil.RespondJSON(w, http.StatusOK, timeStatsIssueResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, IssueIID: 10, Duration: "3h"})
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

// TestSetTimeEstimate_MissingDuration verifies the behavior of set time estimate missing duration.
func TestSetTimeEstimate_MissingDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("SetTimeEstimate() expected error for missing duration")
	}
}

// TestResetTimeEstimate_Success verifies the behavior of reset time estimate success.
func TestResetTimeEstimate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssue10+"/reset_time_estimate" {
			testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate":"","human_total_time_spent":"1h30m","time_estimate":0,"total_time_spent":5400}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ResetTimeEstimate(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ResetTimeEstimate() unexpected error: %v", err)
	}
	if out.TimeEstimate != 0 {
		t.Errorf("TimeEstimate = %d, want 0", out.TimeEstimate)
	}
}

// TestAddSpentTime_Success verifies the behavior of add spent time success.
func TestAddSpentTime_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssue10+"/add_spent_time" {
			testutil.RespondJSON(w, http.StatusCreated, timeStatsIssueResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10, Duration: "1h30m", Summary: "code review"})
	if err != nil {
		t.Fatalf("AddSpentTime() unexpected error: %v", err)
	}
	if out.TotalTimeSpent != 5400 {
		t.Errorf("TotalTimeSpent = %d, want 5400", out.TotalTimeSpent)
	}
}

// TestAddSpentTime_MissingDuration verifies the behavior of add spent time missing duration.
func TestAddSpentTime_MissingDuration(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("AddSpentTime() expected error for missing duration")
	}
}

// TestResetSpentTime_Success verifies the behavior of reset spent time success.
func TestResetSpentTime_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssue10+"/reset_spent_time" {
			testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate":"3h","human_total_time_spent":"","time_estimate":10800,"total_time_spent":0}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ResetSpentTime(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ResetSpentTime() unexpected error: %v", err)
	}
	if out.TotalTimeSpent != 0 {
		t.Errorf("TotalTimeSpent = %d, want 0", out.TotalTimeSpent)
	}
}

// TestGetTimeStats_Success verifies the behavior of get time stats success.
func TestGetTimeStats_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathIssue10+"/time_stats" {
			testutil.RespondJSON(w, http.StatusOK, timeStatsIssueResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetTimeStats(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
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

// TestGetTimeStats_MissingProject verifies the behavior of get time stats missing project.
func TestGetTimeStats_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetTimeStats(context.Background(), client, GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("GetTimeStats() expected error for missing project_id")
	}
}

// TestGetParticipants_Success verifies the behavior of get participants success.
func TestGetParticipants_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathIssue10+"/participants" {
			testutil.RespondJSON(w, http.StatusOK, participantsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetParticipants(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("GetParticipants() unexpected error: %v", err)
	}
	if len(out.Participants) != 2 {
		t.Fatalf("len(Participants) = %d, want 2", len(out.Participants))
	}
	if out.Participants[0].Username != "alice" {
		t.Errorf("Participants[0].Username = %q, want %q", out.Participants[0].Username, "alice")
	}
	if out.Participants[1].Username != "bob" {
		t.Errorf("Participants[1].Username = %q, want %q", out.Participants[1].Username, "bob")
	}
}

// TestGetParticipants_MissingProject verifies the behavior of get participants missing project.
func TestGetParticipants_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetParticipants(context.Background(), client, GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("GetParticipants() expected error for missing project_id")
	}
}

// TestListMRsClosing_Success verifies the behavior of list m rs closing success.
func TestListMRsClosing_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathIssue10+"/closed_by" {
			testutil.RespondJSON(w, http.StatusOK, closingMRsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListMRsClosing(context.Background(), client, ListMRsClosingInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ListMRsClosing() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "Fix login" {
		t.Errorf("MergeRequests[0].Title = %q, want %q", out.MergeRequests[0].Title, "Fix login")
	}
	if out.MergeRequests[0].State != "merged" {
		t.Errorf("MergeRequests[0].State = %q, want %q", out.MergeRequests[0].State, "merged")
	}
}

// TestListMRsClosing_MissingProject verifies the behavior of list m rs closing missing project.
func TestListMRsClosing_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListMRsClosing(context.Background(), client, ListMRsClosingInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ListMRsClosing() expected error for missing project_id")
	}
}

// TestListMRsRelated_Success verifies the behavior of list m rs related success.
func TestListMRsRelated_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathIssue10+"/related_merge_requests" {
			testutil.RespondJSON(w, http.StatusOK, relatedMRsResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListMRsRelated(context.Background(), client, ListMRsRelatedInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ListMRsRelated() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "Refactor auth" {
		t.Errorf("MergeRequests[0].Title = %q, want %q", out.MergeRequests[0].Title, "Refactor auth")
	}
	if out.MergeRequests[0].SourceBranch != "refactor-auth" {
		t.Errorf("MergeRequests[0].SourceBranch = %q, want %q", out.MergeRequests[0].SourceBranch, "refactor-auth")
	}
}

// TestListMRsRelated_MissingProject verifies the behavior of list m rs related missing project.
func TestListMRsRelated_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListMRsRelated(context.Background(), client, ListMRsRelatedInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ListMRsRelated() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestIssueIIDRequired_Validation ensures all handlers that accept issue_iid
// reject zero/negative values with ErrRequiredInt64 before making any API call.
func TestIssueIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_iid is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Update", func() error { _, e := Update(ctx, client, UpdateInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Delete", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, IssueIID: 0}) }},
		{"Reorder", func() error { _, e := Reorder(ctx, client, ReorderInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Move", func() error {
			_, e := Move(ctx, client, MoveInput{ProjectID: pid, IssueIID: 0, ToProjectID: 99})
			return e
		}},
		{"Subscribe", func() error { _, e := Subscribe(ctx, client, SubscribeInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"Unsubscribe", func() error {
			_, e := Unsubscribe(ctx, client, UnsubscribeInput{ProjectID: pid, IssueIID: 0})
			return e
		}},
		{"CreateTodo", func() error { _, e := CreateTodo(ctx, client, CreateTodoInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"SetTimeEstimate", func() error {
			_, e := SetTimeEstimate(ctx, client, SetTimeEstimateInput{ProjectID: pid, IssueIID: 0, Duration: "1h"})
			return e
		}},
		{"ResetTimeEstimate", func() error { _, e := ResetTimeEstimate(ctx, client, GetInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"AddSpentTime", func() error {
			_, e := AddSpentTime(ctx, client, AddSpentTimeInput{ProjectID: pid, IssueIID: 0, Duration: "1h"})
			return e
		}},
		{"ResetSpentTime", func() error { _, e := ResetSpentTime(ctx, client, GetInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"GetTimeStats", func() error { _, e := GetTimeStats(ctx, client, GetInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"GetParticipants", func() error { _, e := GetParticipants(ctx, client, GetInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"ListMRsClosing", func() error {
			_, e := ListMRsClosing(ctx, client, ListMRsClosingInput{ProjectID: pid, IssueIID: 0})
			return e
		}},
		{"ListMRsRelated", func() error {
			_, e := ListMRsRelated(ctx, client, ListMRsRelatedInput{ProjectID: pid, IssueIID: 0})
			return e
		}},
		{"Get_negative", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: -1}); return e }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "issue_iid")
		})
	}
}

// TestIssueIDRequired_Validation ensures GetByID rejects zero/negative issue_id.
func TestIssueIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_id is invalid")
	}))
	tests := []struct {
		name string
		id   int64
	}{
		{"zero", 0},
		{"negative", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetByID(context.Background(), client, GetByIDInput{IssueID: tt.id})
			assertContains(t, err, "issue_id")
		})
	}
}

// TestToProjectIDRequired_Validation ensures Move rejects zero/negative to_project_id.
func TestToProjectIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when to_project_id is invalid")
	}))
	tests := []struct {
		name string
		id   int64
	}{
		{"zero", 0},
		{"negative", -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Move(context.Background(), client, MoveInput{ProjectID: "my/project", IssueIID: 10, ToProjectID: tt.id})
			assertContains(t, err, "to_project_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const testProjectID = "42"

const (
	testDueDateCov       = "2026-06-01"
	testCreatedAtCov     = "2026-01-01T00:00:00Z"
	testNoIssuesFound    = "No issues found"
	testCreatedAfterCov  = "2026-01-01T00:00:00Z"
	testCreatedBeforeCov = "2026-12-31T23:59:59Z"
)

// ---------------------------------------------------------------------------
// Format*Markdown tests
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Populated verifies the behavior of format markdown populated.
func TestFormatMarkdown_Populated(t *testing.T) {
	md := FormatMarkdown(Output{
		IID: 10, Title: "Big Bug", State: "opened",
		Author: "alice", Assignees: []string{"bob", "carol"},
		Labels: []string{"bug", "critical"}, Milestone: "v1.0",
		DueDate: testDueDateCov, Confidential: true,
		CreatedAt: testCreatedAtCov, Description: "Details here",
		WebURL:              "https://gitlab.example.com/issue/10",
		TaskCompletionCount: 3, TaskCompletionTotal: 5,
		UserNotesCount: 7,
	})
	for _, want := range []string{
		"Big Bug", "opened", "@alice", "@bob", "@carol",
		"bug", "critical", "v1.0", "1 Jun 2026", "Confidential",
		"Details here", "https://gitlab.example.com/issue/10",
		"Tasks", "3/5", "Comments", "7",
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
		Issues: []Output{
			{IID: 1, Title: "Issue1", State: "opened", Author: "alice", Labels: []string{"bug"}},
			{IID: 2, Title: "Issue2", State: "closed", Author: "bob", Labels: []string{}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})
	for _, want := range []string{"Issue1", "Issue2", "alice", "bob", "#1", "#2"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown missing %q", want)
		}
	}
}

// TestFormatListMarkdown_ClickableIssueLinks verifies that issue IIDs appear
// as clickable Markdown links when WebURL is present.
func TestFormatListMarkdown_ClickableIssueLinks(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Issues: []Output{
			{IID: 42, Title: "Bug", State: "opened", Author: "alice",
				WebURL: "https://gitlab.example.com/issues/42"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "[#42](https://gitlab.example.com/issues/42)") {
		t.Errorf("expected clickable issue link, got:\n%s", md)
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, testNoIssuesFound) {
		t.Error("FormatListMarkdown should say no issues found for empty list")
	}
}

// TestFormatListGroupMarkdown_Populated verifies the behavior of format list group markdown populated.
func TestFormatListGroupMarkdown_Populated(t *testing.T) {
	md := FormatListGroupMarkdown(ListGroupOutput{
		Issues: []Output{
			{IID: 5, Title: "GroupIssue", State: "opened", Author: "carol", Labels: []string{"feat"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"GroupIssue", "carol", "#5", "feat", "Group Issues"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListGroupMarkdown missing %q", want)
		}
	}
}

// TestFormatListGroupMarkdown_ClickableLinks verifies that group issue list
// renders IIDs as clickable Markdown links.
func TestFormatListGroupMarkdown_ClickableLinks(t *testing.T) {
	md := FormatListGroupMarkdown(ListGroupOutput{
		Issues: []Output{
			{IID: 5, Title: "GroupIssue", State: "opened", Author: "carol",
				WebURL: "https://gitlab.example.com/issues/5"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "[#5](https://gitlab.example.com/issues/5)") {
		t.Errorf("expected clickable issue link in group list, got:\n%s", md)
	}
}

// TestFormatListGroupMarkdown_Empty verifies the behavior of format list group markdown empty.
func TestFormatListGroupMarkdown_Empty(t *testing.T) {
	md := FormatListGroupMarkdown(ListGroupOutput{})
	if !strings.Contains(md, testNoIssuesFound) {
		t.Error("FormatListGroupMarkdown should say no issues found for empty list")
	}
}

// TestFormatListAllMarkdown_Populated verifies the behavior of format list all markdown populated.
func TestFormatListAllMarkdown_Populated(t *testing.T) {
	md := FormatListAllMarkdown(ListOutput{
		Issues: []Output{
			{IID: 100, Title: "AllIssue", State: "closed", Author: "dave", Labels: []string{"doc"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"AllIssue", "dave", "#100", "doc", "All Issues"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListAllMarkdown missing %q", want)
		}
	}
}

// TestFormatListAllMarkdown_Empty verifies the behavior of format list all markdown empty.
func TestFormatListAllMarkdown_Empty(t *testing.T) {
	md := FormatListAllMarkdown(ListOutput{})
	if !strings.Contains(md, testNoIssuesFound) {
		t.Error("FormatListAllMarkdown should say no issues found for empty list")
	}
}

// TestFormatListAllMarkdown_ClickableLinks verifies that all-issues list
// renders IIDs as clickable Markdown links.
func TestFormatListAllMarkdown_ClickableLinks(t *testing.T) {
	md := FormatListAllMarkdown(ListOutput{
		Issues: []Output{
			{IID: 100, Title: "AllIssue", State: "closed", Author: "dave",
				WebURL: "https://gitlab.example.com/issues/100"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "[#100](https://gitlab.example.com/issues/100)") {
		t.Errorf("expected clickable issue link in all-issues list, got:\n%s", md)
	}
}

// TestFormatTodoMarkdown_Populated verifies the behavior of format todo markdown populated.
func TestFormatTodoMarkdown_Populated(t *testing.T) {
	md := FormatTodoMarkdown(TodoOutput{
		ID: 1, ActionName: "marked", TargetType: "Issue",
		TargetTitle: "Bug fix", TargetURL: "https://gitlab.example.com/todo/1",
		State: "pending", CreatedAt: testCreatedAtCov,
	})
	for _, want := range []string{"Todo #1", "marked", "Issue", "Bug fix", "pending", "1 Jan 2026", "https://gitlab.example.com/todo/1"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatTodoMarkdown missing %q", want)
		}
	}
}

// TestFormatTodoMarkdown_Empty verifies the behavior of format todo markdown empty.
func TestFormatTodoMarkdown_Empty(t *testing.T) {
	md := FormatTodoMarkdown(TodoOutput{})
	if md == "" {
		t.Error("FormatTodoMarkdown returned empty string for zero value")
	}
}

// TestFormatTimeStatsMarkdown_Populated verifies the behavior of format time stats markdown populated.
func TestFormatTimeStatsMarkdown_Populated(t *testing.T) {
	md := FormatTimeStatsMarkdown(TimeStatsOutput{
		HumanTimeEstimate:   "3h",
		HumanTotalTimeSpent: "1h",
		TimeEstimate:        10800,
		TotalTimeSpent:      3600,
	})
	for _, want := range []string{"Time Tracking", "3h", "1h", "10800", "3600"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatTimeStatsMarkdown missing %q", want)
		}
	}
}

// TestFormatTimeStatsMarkdown_Empty verifies the behavior of format time stats markdown empty.
func TestFormatTimeStatsMarkdown_Empty(t *testing.T) {
	md := FormatTimeStatsMarkdown(TimeStatsOutput{})
	if !strings.Contains(md, "Time Tracking") {
		t.Error("FormatTimeStatsMarkdown should contain heading even for zero value")
	}
}

// TestFormatParticipantsMarkdown_Populated verifies the behavior of format participants markdown populated.
func TestFormatParticipantsMarkdown_Populated(t *testing.T) {
	md := FormatParticipantsMarkdown(ParticipantsOutput{
		Participants: []ParticipantOutput{
			{ID: 1, Username: "alice", Name: "Alice A"},
			{ID: 2, Username: "bob", Name: "Bob B"},
		},
	})
	for _, want := range []string{"Participants (2)", "alice", "bob", "Alice A", "Bob B"} {
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

// TestFormatRelatedMRsMarkdown_Populated verifies the behavior of format related m rs markdown populated.
func TestFormatRelatedMRsMarkdown_Populated(t *testing.T) {
	md := FormatRelatedMRsMarkdown(RelatedMRsOutput{
		MergeRequests: []RelatedMROutput{
			{IID: 3, Title: "Fix MR", State: "merged", Author: "carol", SourceBranch: "fix", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}, "Related MRs")
	for _, want := range []string{"Related MRs", "Fix MR", "merged", "@carol", "fix", "main", "!3"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatRelatedMRsMarkdown missing %q", want)
		}
	}
}

// TestFormatRelatedMRsMarkdown_Empty verifies the behavior of format related m rs markdown empty.
func TestFormatRelatedMRsMarkdown_Empty(t *testing.T) {
	md := FormatRelatedMRsMarkdown(RelatedMRsOutput{}, "Closing MRs")
	if !strings.Contains(md, "No merge requests found") {
		t.Error("FormatRelatedMRsMarkdown should say no MRs found for empty output")
	}
}

// ---------------------------------------------------------------------------
// prefixAt helper
// ---------------------------------------------------------------------------.

// TestPrefixAt verifies the behavior of prefix at.
func TestPrefixAt(t *testing.T) {
	result := prefixAt([]string{"alice", "bob"})
	if len(result) != 2 || result[0] != "@alice" || result[1] != "@bob" {
		t.Errorf("prefixAt got %v, want [@alice @bob]", result)
	}
	empty := prefixAt([]string{})
	if len(empty) != 0 {
		t.Errorf("prefixAt empty got %v, want []", empty)
	}
}

// ---------------------------------------------------------------------------
// parseDueDate
// ---------------------------------------------------------------------------.

// TestParseDueDate_Valid verifies the behavior of parse due date valid.
func TestParseDueDate_Valid(t *testing.T) {
	d, err := parseDueDate("2026-06-15")
	if err != nil {
		t.Fatalf("parseDueDate valid: unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("parseDueDate valid: got nil")
	}
}

// TestParseDueDate_Invalid verifies the behavior of parse due date invalid.
func TestParseDueDate_Invalid(t *testing.T) {
	_, err := parseDueDate("not-a-date")
	if err == nil {
		t.Fatal("parseDueDate invalid: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid due_date") {
		t.Errorf("parseDueDate error = %q, want 'invalid due_date' substring", err.Error())
	}
}

// TestParseDueDate_RFC3339Rejected verifies that parseDueDate rejects RFC 3339
// timestamps (a common LLM mistake). Only YYYY-MM-DD format is accepted.
func TestParseDueDate_RFC3339Rejected(t *testing.T) {
	cases := []string{
		"2026-01-15T10:00:00Z",
		"2026-01-15T10:00:00+02:00",
		"2026-01-15 10:00:00",
		"01/15/2026",
		"15-01-2026",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := parseDueDate(tc)
			if err == nil {
				t.Errorf("parseDueDate(%q): expected error for non-YYYY-MM-DD format", tc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildUpdateOpts
// ---------------------------------------------------------------------------.

// TestBuildUpdateOpts_AllFields verifies the behavior of build update opts all fields.
func TestBuildUpdateOpts_AllFields(t *testing.T) {
	conf := true
	locked := false
	opts, err := buildUpdateOpts(UpdateInput{
		Title:            "New Title",
		Description:      "New Description",
		StateEvent:       "close",
		AssigneeIDs:      []int64{1, 2},
		Labels:           "a",
		AddLabels:        "b",
		RemoveLabels:     "c",
		MilestoneID:      5,
		DueDate:          "2026-12-31",
		Confidential:     &conf,
		IssueType:        "incident",
		Weight:           3,
		DiscussionLocked: &locked,
	})
	if err != nil {
		t.Fatalf("buildUpdateOpts: unexpected error: %v", err)
	}
	assertUpdateOptsFields(t, opts)
	assertUpdateOptsMetadata(t, opts, &conf, &locked)
}

// assertUpdateOptsFields is an internal helper for the issues package.
func assertUpdateOptsFields(t *testing.T, opts *gl.UpdateIssueOptions) {
	t.Helper()
	if opts.Title == nil || *opts.Title != "New Title" {
		t.Error("buildUpdateOpts: Title not set")
	}
	if opts.Description == nil {
		t.Error("buildUpdateOpts: Description not set")
	}
	if opts.StateEvent == nil || *opts.StateEvent != "close" {
		t.Error("buildUpdateOpts: StateEvent not set")
	}
	if opts.AssigneeIDs == nil || len(*opts.AssigneeIDs) != 2 {
		t.Error("buildUpdateOpts: AssigneeIDs not set")
	}
	if opts.Labels == nil {
		t.Error("buildUpdateOpts: Labels not set")
	}
	if opts.AddLabels == nil {
		t.Error("buildUpdateOpts: AddLabels not set")
	}
	if opts.RemoveLabels == nil {
		t.Error("buildUpdateOpts: RemoveLabels not set")
	}
}

// assertUpdateOptsMetadata is an internal helper for the issues package.
func assertUpdateOptsMetadata(t *testing.T, opts *gl.UpdateIssueOptions, wantConf, wantLocked *bool) {
	t.Helper()
	if opts.MilestoneID == nil || *opts.MilestoneID != 5 {
		t.Error("buildUpdateOpts: MilestoneID not set")
	}
	if opts.DueDate == nil {
		t.Error("buildUpdateOpts: DueDate not set")
	}
	if opts.Confidential == nil || *opts.Confidential != *wantConf {
		t.Error("buildUpdateOpts: Confidential not set")
	}
	if opts.IssueType == nil || *opts.IssueType != "incident" {
		t.Error("buildUpdateOpts: IssueType not set")
	}
	if opts.Weight == nil || *opts.Weight != 3 {
		t.Error("buildUpdateOpts: Weight not set")
	}
	if opts.DiscussionLocked == nil || *opts.DiscussionLocked != *wantLocked {
		t.Error("buildUpdateOpts: DiscussionLocked not set")
	}
}

// TestBuildUpdateOpts_InvalidDueDate verifies the behavior of build update opts invalid due date.
func TestBuildUpdateOpts_InvalidDueDate(t *testing.T) {
	_, err := buildUpdateOpts(UpdateInput{DueDate: "bad-date"})
	if err == nil {
		t.Fatal("buildUpdateOpts: expected error for invalid due date, got nil")
	}
}

// TestBuildUpdateOpts_Empty verifies the behavior of build update opts empty.
func TestBuildUpdateOpts_Empty(t *testing.T) {
	opts, err := buildUpdateOpts(UpdateInput{})
	if err != nil {
		t.Fatalf("buildUpdateOpts empty: %v", err)
	}
	if opts == nil {
		t.Fatal("buildUpdateOpts empty: got nil opts")
	}
}

// ---------------------------------------------------------------------------
// ToOutput edge cases
// ---------------------------------------------------------------------------.

// TestToOutput_Populated verifies the behavior of to output populated.
func TestToOutput_Populated(t *testing.T) {
	now := new(gl.ISOTime)
	issue := &gl.Issue{
		ID: 1, IID: 10, Title: "Test", Description: "Desc", State: "opened",
		Labels:               gl.Labels{"a", "b"},
		Author:               &gl.IssueAuthor{Username: "alice"},
		Milestone:            &gl.Milestone{Title: "v1"},
		Assignees:            []*gl.IssueAssignee{{Username: "bob"}, {Username: "carol"}},
		WebURL:               "https://example.com",
		Confidential:         true,
		DiscussionLocked:     true,
		ProjectID:            42,
		Weight:               5,
		HealthStatus:         "on_track",
		MergeRequestCount:    2,
		UserNotesCount:       10,
		Upvotes:              3,
		Downvotes:            1,
		MovedToID:            99,
		EpicIssueID:          7,
		DueDate:              now,
		ClosedBy:             &gl.IssueCloser{Username: "dave"},
		References:           &gl.IssueReferences{Full: "proj#10"},
		TaskCompletionStatus: &gl.TasksCompletionStatus{CompletedCount: 2, Count: 5},
		Subscribed:           true,
		TimeStats:            &gl.TimeStats{TimeEstimate: 3600, TotalTimeSpent: 1800},
	}
	out := ToOutput(issue)
	if out.Author != "alice" {
		t.Errorf("Author = %q, want alice", out.Author)
	}
	if out.Milestone != "v1" {
		t.Errorf("Milestone = %q, want v1", out.Milestone)
	}
	if len(out.Assignees) != 2 {
		t.Errorf("Assignees len = %d, want 2", len(out.Assignees))
	}
	if out.ClosedBy != "dave" {
		t.Errorf("ClosedBy = %q, want dave", out.ClosedBy)
	}
	if out.References != "proj#10" {
		t.Errorf("References = %q, want proj#10", out.References)
	}
	if out.TaskCompletionCount != 2 || out.TaskCompletionTotal != 5 {
		t.Errorf("TaskCompletion = %d/%d, want 2/5", out.TaskCompletionCount, out.TaskCompletionTotal)
	}
	if !out.Subscribed {
		t.Error("Subscribed = false, want true")
	}
	if out.TimeEstimate != 3600 || out.TotalTimeSpent != 1800 {
		t.Errorf("TimeStats = %d/%d, want 3600/1800", out.TimeEstimate, out.TotalTimeSpent)
	}
	if out.EpicIssueID != 7 {
		t.Errorf("EpicIssueID = %d, want 7", out.EpicIssueID)
	}
	if !out.Confidential {
		t.Error("Confidential = false, want true")
	}
	if !out.DiscussionLocked {
		t.Error("DiscussionLocked = false, want true")
	}
	if out.Weight != 5 {
		t.Errorf("Weight = %d, want 5", out.Weight)
	}
}

// TestToOutput_NilOptionalFields verifies the behavior of to output nil optional fields.
func TestToOutput_NilOptionalFields(t *testing.T) {
	issue := &gl.Issue{
		ID: 2, IID: 20, Title: "Minimal", State: "opened",
	}
	out := ToOutput(issue)
	if out.Author != "" {
		t.Errorf("Author = %q, want empty for nil author", out.Author)
	}
	if out.Milestone != "" {
		t.Errorf("Milestone = %q, want empty for nil milestone", out.Milestone)
	}
	if out.ClosedBy != "" {
		t.Errorf("ClosedBy = %q, want empty for nil ClosedBy", out.ClosedBy)
	}
	if out.References != "" {
		t.Errorf("References = %q, want empty for nil references", out.References)
	}
	if len(out.Labels) != 0 {
		t.Errorf("Labels = %v, want empty slice", out.Labels)
	}
	if len(out.Assignees) != 0 {
		t.Errorf("Assignees = %v, want empty slice", out.Assignees)
	}
}

// TestToOutput_IssueType verifies the behavior of to output issue type.
func TestToOutput_IssueType(t *testing.T) {
	issue := &gl.Issue{ID: 3, IssueType: new("task")}
	out := ToOutput(issue)
	if out.IssueType != "task" {
		t.Errorf("IssueType = %q, want task", out.IssueType)
	}
}

// ---------------------------------------------------------------------------
// timeStatsToOutput
// ---------------------------------------------------------------------------.

// TestTimeStatsToOutput_Nil verifies the behavior of time stats to output nil.
func TestTimeStatsToOutput_Nil(t *testing.T) {
	out := timeStatsToOutput(nil)
	if out.TimeEstimate != 0 || out.TotalTimeSpent != 0 {
		t.Errorf("timeStatsToOutput(nil) = %+v, want zero", out)
	}
}

// TestTimeStatsToOutput_Populated verifies the behavior of time stats to output populated.
func TestTimeStatsToOutput_Populated(t *testing.T) {
	ts := &gl.TimeStats{
		HumanTimeEstimate:   "2h",
		HumanTotalTimeSpent: "30m",
		TimeEstimate:        7200,
		TotalTimeSpent:      1800,
	}
	out := timeStatsToOutput(ts)
	if out.HumanTimeEstimate != "2h" {
		t.Errorf("HumanTimeEstimate = %q, want 2h", out.HumanTimeEstimate)
	}
	if out.TotalTimeSpent != 1800 {
		t.Errorf("TotalTimeSpent = %d, want 1800", out.TotalTimeSpent)
	}
}

// ---------------------------------------------------------------------------
// basicMRToOutput
// ---------------------------------------------------------------------------.

// TestBasicMRToOutput verifies the behavior of basic m r to output.
func TestBasicMRToOutput(t *testing.T) {
	mr := &gl.BasicMergeRequest{
		ID: 1, IID: 2, Title: "MR1", State: "merged",
		SourceBranch: "feat", TargetBranch: "main",
		Author: &gl.BasicUser{Username: "alice"},
		WebURL: "https://gitlab.example.com/mr/1",
	}
	out := basicMRToOutput(mr)
	if out.Author != "alice" {
		t.Errorf("Author = %q, want alice", out.Author)
	}
	if out.WebURL != "https://gitlab.example.com/mr/1" {
		t.Errorf("WebURL = %q, want correct URL", out.WebURL)
	}
}

// TestBasicMRToOutput_NilAuthor verifies the behavior of basic m r to output nil author.
func TestBasicMRToOutput_NilAuthor(t *testing.T) {
	mr := &gl.BasicMergeRequest{ID: 1, IID: 2}
	out := basicMRToOutput(mr)
	if out.Author != "" {
		t.Errorf("Author = %q, want empty for nil author", out.Author)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation tests for ALL 21 handlers
// ---------------------------------------------------------------------------.

// nopClient is an internal helper for the issues package.
func nopClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	if _, err := Get(testutil.CancelledCtx(t), nopClient(t), GetInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("Get: expected error for canceled context")
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	if _, err := List(testutil.CancelledCtx(t), nopClient(t), ListInput{ProjectID: testProjectID}); err == nil {
		t.Fatal("List: expected error for canceled context")
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	if _, err := Update(testutil.CancelledCtx(t), nopClient(t), UpdateInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("Update: expected error for canceled context")
	}
}

// TestGetByID_CancelledContext verifies the behavior of get by i d cancelled context.
func TestGetByID_CancelledContext(t *testing.T) {
	if _, err := GetByID(testutil.CancelledCtx(t), nopClient(t), GetByIDInput{IssueID: 10}); err == nil {
		t.Fatal("GetByID: expected error for canceled context")
	}
}

// TestReorder_CancelledContext verifies the behavior of reorder cancelled context.
func TestReorder_CancelledContext(t *testing.T) {
	if _, err := Reorder(testutil.CancelledCtx(t), nopClient(t), ReorderInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("Reorder: expected error for canceled context")
	}
}

// TestMove_CancelledContext verifies the behavior of move cancelled context.
func TestMove_CancelledContext(t *testing.T) {
	if _, err := Move(testutil.CancelledCtx(t), nopClient(t), MoveInput{ProjectID: testProjectID, IssueIID: 10, ToProjectID: 99}); err == nil {
		t.Fatal("Move: expected error for canceled context")
	}
}

// TestSubscribe_CancelledContext verifies the behavior of subscribe cancelled context.
func TestSubscribe_CancelledContext(t *testing.T) {
	if _, err := Subscribe(testutil.CancelledCtx(t), nopClient(t), SubscribeInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("Subscribe: expected error for canceled context")
	}
}

// TestUnsubscribe_CancelledContext verifies the behavior of unsubscribe cancelled context.
func TestUnsubscribe_CancelledContext(t *testing.T) {
	if _, err := Unsubscribe(testutil.CancelledCtx(t), nopClient(t), UnsubscribeInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("Unsubscribe: expected error for canceled context")
	}
}

// TestCreateTodo_CancelledContext verifies the behavior of create todo cancelled context.
func TestCreateTodo_CancelledContext(t *testing.T) {
	if _, err := CreateTodo(testutil.CancelledCtx(t), nopClient(t), CreateTodoInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("CreateTodo: expected error for canceled context")
	}
}

// TestSetTimeEstimate_CancelledContext verifies the behavior of set time estimate cancelled context.
func TestSetTimeEstimate_CancelledContext(t *testing.T) {
	if _, err := SetTimeEstimate(testutil.CancelledCtx(t), nopClient(t), SetTimeEstimateInput{ProjectID: testProjectID, IssueIID: 10, Duration: "3h"}); err == nil {
		t.Fatal("SetTimeEstimate: expected error for canceled context")
	}
}

// TestResetTimeEstimate_CancelledContext verifies the behavior of reset time estimate cancelled context.
func TestResetTimeEstimate_CancelledContext(t *testing.T) {
	if _, err := ResetTimeEstimate(testutil.CancelledCtx(t), nopClient(t), GetInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("ResetTimeEstimate: expected error for canceled context")
	}
}

// TestAddSpentTime_CancelledContext verifies the behavior of add spent time cancelled context.
func TestAddSpentTime_CancelledContext(t *testing.T) {
	if _, err := AddSpentTime(testutil.CancelledCtx(t), nopClient(t), AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10, Duration: "1h"}); err == nil {
		t.Fatal("AddSpentTime: expected error for canceled context")
	}
}

// TestResetSpentTime_CancelledContext verifies the behavior of reset spent time cancelled context.
func TestResetSpentTime_CancelledContext(t *testing.T) {
	if _, err := ResetSpentTime(testutil.CancelledCtx(t), nopClient(t), GetInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("ResetSpentTime: expected error for canceled context")
	}
}

// TestGetTimeStats_CancelledContext verifies the behavior of get time stats cancelled context.
func TestGetTimeStats_CancelledContext(t *testing.T) {
	if _, err := GetTimeStats(testutil.CancelledCtx(t), nopClient(t), GetInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("GetTimeStats: expected error for canceled context")
	}
}

// TestGetParticipants_CancelledContext verifies the behavior of get participants cancelled context.
func TestGetParticipants_CancelledContext(t *testing.T) {
	if _, err := GetParticipants(testutil.CancelledCtx(t), nopClient(t), GetInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("GetParticipants: expected error for canceled context")
	}
}

// TestListMRsClosing_CancelledContext verifies the behavior of list m rs closing cancelled context.
func TestListMRsClosing_CancelledContext(t *testing.T) {
	if _, err := ListMRsClosing(testutil.CancelledCtx(t), nopClient(t), ListMRsClosingInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("ListMRsClosing: expected error for canceled context")
	}
}

// TestListMRsRelated_CancelledContext verifies the behavior of list m rs related cancelled context.
func TestListMRsRelated_CancelledContext(t *testing.T) {
	if _, err := ListMRsRelated(testutil.CancelledCtx(t), nopClient(t), ListMRsRelatedInput{ProjectID: testProjectID, IssueIID: 10}); err == nil {
		t.Fatal("ListMRsRelated: expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// Missing project_id validation tests
// ---------------------------------------------------------------------------.

// TestMove_MissingProjectID verifies the behavior of move missing project i d.
func TestMove_MissingProjectID(t *testing.T) {
	client := nopClient(t)
	_, err := Move(context.Background(), client, MoveInput{IssueIID: 10, ToProjectID: 99})
	if err == nil {
		t.Fatal("Move: expected error for empty project_id")
	}
}

// TestSetTimeEstimate_MissingProject verifies the behavior of set time estimate missing project.
func TestSetTimeEstimate_MissingProject(t *testing.T) {
	_, err := SetTimeEstimate(context.Background(), nopClient(t), SetTimeEstimateInput{IssueIID: 10, Duration: "3h"})
	if err == nil {
		t.Fatal("SetTimeEstimate: expected error for empty project_id")
	}
}

// TestSetTimeEstimate_MissingDuration2 verifies the behavior of set time estimate missing duration2.
func TestSetTimeEstimate_MissingDuration2(t *testing.T) {
	_, err := SetTimeEstimate(context.Background(), nopClient(t), SetTimeEstimateInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("SetTimeEstimate: expected error for empty duration")
	}
}

// TestResetTimeEstimate_MissingProject verifies the behavior of reset time estimate missing project.
func TestResetTimeEstimate_MissingProject(t *testing.T) {
	_, err := ResetTimeEstimate(context.Background(), nopClient(t), GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ResetTimeEstimate: expected error for empty project_id")
	}
}

// TestAddSpentTime_MissingProject verifies the behavior of add spent time missing project.
func TestAddSpentTime_MissingProject(t *testing.T) {
	_, err := AddSpentTime(context.Background(), nopClient(t), AddSpentTimeInput{IssueIID: 10, Duration: "1h"})
	if err == nil {
		t.Fatal("AddSpentTime: expected error for empty project_id")
	}
}

// TestAddSpentTime_MissingDuration2 verifies the behavior of add spent time missing duration2.
func TestAddSpentTime_MissingDuration2(t *testing.T) {
	_, err := AddSpentTime(context.Background(), nopClient(t), AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("AddSpentTime: expected error for empty duration")
	}
}

// TestResetSpentTime_MissingProject verifies the behavior of reset spent time missing project.
func TestResetSpentTime_MissingProject(t *testing.T) {
	_, err := ResetSpentTime(context.Background(), nopClient(t), GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ResetSpentTime: expected error for empty project_id")
	}
}

// TestGetParticipants_MissingProject2 verifies the behavior of get participants missing project2.
func TestGetParticipants_MissingProject2(t *testing.T) {
	_, err := GetParticipants(context.Background(), nopClient(t), GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("GetParticipants: expected error for empty project_id")
	}
}

// TestListMRsClosing_MissingProject2 verifies the behavior of list m rs closing missing project2.
func TestListMRsClosing_MissingProject2(t *testing.T) {
	_, err := ListMRsClosing(context.Background(), nopClient(t), ListMRsClosingInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ListMRsClosing: expected error for empty project_id")
	}
}

// TestListMRsRelated_MissingProject2 verifies the behavior of list m rs related missing project2.
func TestListMRsRelated_MissingProject2(t *testing.T) {
	_, err := ListMRsRelated(context.Background(), nopClient(t), ListMRsRelatedInput{IssueIID: 10})
	if err == nil {
		t.Fatal("ListMRsRelated: expected error for empty project_id")
	}
}

// TestCreate_MissingProject verifies the behavior of create missing project.
func TestCreate_MissingProject(t *testing.T) {
	_, err := Create(context.Background(), nopClient(t), CreateInput{Title: "t"})
	if err == nil {
		t.Fatal("Create: expected error for empty project_id")
	}
}

// TestGet_MissingProject verifies the behavior of get missing project.
func TestGet_MissingProject(t *testing.T) {
	_, err := Get(context.Background(), nopClient(t), GetInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Get: expected error for empty project_id")
	}
}

// TestList_MissingProject verifies the behavior of list missing project.
func TestList_MissingProject(t *testing.T) {
	_, err := List(context.Background(), nopClient(t), ListInput{})
	if err == nil {
		t.Fatal("List: expected error for empty project_id")
	}
}

// TestUpdate_MissingProject verifies the behavior of update missing project.
func TestUpdate_MissingProject(t *testing.T) {
	_, err := Update(context.Background(), nopClient(t), UpdateInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Update: expected error for empty project_id")
	}
}

// TestDelete_MissingProject verifies the behavior of delete missing project.
func TestDelete_MissingProject(t *testing.T) {
	err := Delete(context.Background(), nopClient(t), DeleteInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Delete: expected error for empty project_id")
	}
}

// TestListGroup_MissingGroupID verifies the behavior of list group missing group i d.
func TestListGroup_MissingGroupID(t *testing.T) {
	_, err := ListGroup(context.Background(), nopClient(t), ListGroupInput{})
	if err == nil {
		t.Fatal("ListGroup: expected error for empty group_id")
	}
}

// TestGetByID_MissingIssueID verifies the behavior of get by i d missing issue i d.
func TestGetByID_MissingIssueID(t *testing.T) {
	_, err := GetByID(context.Background(), nopClient(t), GetByIDInput{})
	if err == nil {
		t.Fatal("GetByID: expected error for zero issue_id")
	}
}

// TestReorder_MissingProject verifies the behavior of reorder missing project.
func TestReorder_MissingProject(t *testing.T) {
	_, err := Reorder(context.Background(), nopClient(t), ReorderInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Reorder: expected error for empty project_id")
	}
}

// TestSubscribe_MissingProject verifies the behavior of subscribe missing project.
func TestSubscribe_MissingProject(t *testing.T) {
	_, err := Subscribe(context.Background(), nopClient(t), SubscribeInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Subscribe: expected error for empty project_id")
	}
}

// TestUnsubscribe_MissingProject verifies the behavior of unsubscribe missing project.
func TestUnsubscribe_MissingProject(t *testing.T) {
	_, err := Unsubscribe(context.Background(), nopClient(t), UnsubscribeInput{IssueIID: 10})
	if err == nil {
		t.Fatal("Unsubscribe: expected error for empty project_id")
	}
}

// TestCreateTodo_MissingProject verifies the behavior of create todo missing project.
func TestCreateTodo_MissingProject(t *testing.T) {
	_, err := CreateTodo(context.Background(), nopClient(t), CreateTodoInput{IssueIID: 10})
	if err == nil {
		t.Fatal("CreateTodo: expected error for empty project_id")
	}
}

// TestMove_MissingToProjectCov verifies the behavior of move missing to project cov.
func TestMove_MissingToProjectCov(t *testing.T) {
	_, err := Move(context.Background(), nopClient(t), MoveInput{ProjectID: testProjectID, IssueIID: 10})
	if err == nil {
		t.Fatal("Move: expected error for zero to_project_id")
	}
}

// ---------------------------------------------------------------------------
// Success paths for all handlers via mock API
// ---------------------------------------------------------------------------.

const (
	issueJSONCov       = `{"id":1,"iid":10,"title":"Test Issue","state":"opened","labels":["bug"],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/issue/10","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","project_id":42}`
	issueListJSONCov   = `[` + issueJSONCov + `]`
	timeStatsJSONCov   = `{"human_time_estimate":"3h","human_total_time_spent":"1h","time_estimate":10800,"total_time_spent":3600}`
	participantJSONCov = `[{"id":1,"username":"alice","name":"Alice","web_url":"https://example.com/alice"}]`
	todoJSONCov        = `{"id":1,"action_name":"marked","target_type":"Issue","target":{"title":"Test","web_url":"https://example.com"},"state":"pending","created_at":"2026-01-01T00:00:00Z"}`
	closingMRJSONCov   = `[{"id":1,"iid":5,"title":"Fix","state":"merged","source_branch":"fix","target_branch":"main","author":{"username":"bob"},"web_url":"https://example.com/mr/5"}]`
)

// issueMockResp holds a canned response for a mock issue endpoint.
type issueMockResp struct {
	status int
	body   string
	pgHdr  *testutil.PaginationHeaders
}

// issueMockHandler is an internal helper for the issues package.
func issueMockHandler(w http.ResponseWriter, r *http.Request) {
	pgDefault := &testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}
	issue10 := "/api/v4/projects/42/issues/10"

	routes := map[string]issueMockResp{
		"POST /api/v4/projects/42/issues":            {http.StatusCreated, issueJSONCov, nil},
		"GET " + issue10:                             {http.StatusOK, issueJSONCov, nil},
		"PUT " + issue10:                             {http.StatusOK, issueJSONCov, nil},
		"DELETE " + issue10:                          {http.StatusNoContent, "", nil},
		"GET /api/v4/projects/42/issues":             {http.StatusOK, issueListJSONCov, pgDefault},
		"GET /api/v4/groups/99/issues":               {http.StatusOK, issueListJSONCov, pgDefault},
		"GET /api/v4/issues":                         {http.StatusOK, issueListJSONCov, pgDefault},
		"GET /api/v4/issues/10":                      {http.StatusOK, issueJSONCov, nil},
		"PUT " + issue10 + "/reorder":                {http.StatusOK, issueJSONCov, nil},
		"POST " + issue10 + "/move":                  {http.StatusOK, issueJSONCov, nil},
		"POST " + issue10 + "/subscribe":             {http.StatusOK, issueJSONCov, nil},
		"POST " + issue10 + "/unsubscribe":           {http.StatusOK, issueJSONCov, nil},
		"POST " + issue10 + "/todo":                  {http.StatusCreated, todoJSONCov, nil},
		"POST " + issue10 + "/time_estimate":         {http.StatusOK, timeStatsJSONCov, nil},
		"POST " + issue10 + "/reset_time_estimate":   {http.StatusOK, timeStatsJSONCov, nil},
		"POST " + issue10 + "/add_spent_time":        {http.StatusCreated, timeStatsJSONCov, nil},
		"POST " + issue10 + "/reset_spent_time":      {http.StatusOK, timeStatsJSONCov, nil},
		"GET " + issue10 + "/time_stats":             {http.StatusOK, timeStatsJSONCov, nil},
		"GET " + issue10 + "/participants":           {http.StatusOK, participantJSONCov, nil},
		"GET " + issue10 + "/closed_by":              {http.StatusOK, closingMRJSONCov, pgDefault},
		"GET " + issue10 + "/related_merge_requests": {http.StatusOK, closingMRJSONCov, pgDefault},
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
}

// TestCreate_SuccessCov verifies the behavior of create success cov.
func TestCreate_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	conf := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID, Title: "Test Issue",
		Description: "desc", Labels: "bug",
		AssigneeIDs: []int64{1}, MilestoneID: 5,
		DueDate: testDueDateCov, Confidential: &conf,
		IssueType: "issue", Weight: 3, EpicID: 10,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Create IID = %d, want 10", out.IID)
	}
}

// TestCreate_WithCreatedAt verifies the behavior of create with created at.
func TestCreate_WithCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID, Title: "Test", CreatedAt: testCreatedAtCov,
	})
	if err != nil {
		t.Fatalf("Create with created_at: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("IID = %d, want 10", out.IID)
	}
}

// TestCreate_InvalidCreatedAt verifies the behavior of create invalid created at.
func TestCreate_InvalidCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID, Title: "Test", CreatedAt: "not-a-time",
	})
	if err == nil {
		t.Fatal("Create: expected error for invalid created_at, got nil")
	}
}

// TestCreate_WithMRResolve verifies the behavior of create with m r resolve.
func TestCreate_WithMRResolve(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID, Title: "Test",
		MergeRequestToResolveDiscussionsOf: 99,
		DiscussionToResolve:                "abc123",
	})
	if err != nil {
		t.Fatalf("Create with MR resolve: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("IID = %d, want 10", out.IID)
	}
}

// TestGet_SuccessCov verifies the behavior of get success cov.
func TestGet_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Title != "Test Issue" {
		t.Errorf("Get Title = %q, want Test Issue", out.Title)
	}
}

// TestList_SuccessCov verifies the behavior of list success cov.
func TestList_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("List len = %d, want 1", len(out.Issues))
	}
}

// TestUpdate_SuccessCov verifies the behavior of update success cov.
func TestUpdate_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, IssueIID: 10, Title: "Updated"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Update IID = %d, want 10", out.IID)
	}
}

// TestUpdate_InvalidDueDateCov verifies the behavior of update invalid due date cov.
func TestUpdate_InvalidDueDateCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID, IssueIID: 10, DueDate: "bad"})
	if err == nil {
		t.Fatal("Update: expected error for invalid due_date")
	}
}

// TestDelete_SuccessCov verifies the behavior of delete success cov.
func TestDelete_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestListGroup_SuccessCov verifies the behavior of list group success cov.
func TestListGroup_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("ListGroup: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListGroup len = %d, want 1", len(out.Issues))
	}
}

// TestListAll_SuccessCov verifies the behavior of list all success cov.
func TestListAll_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListAll len = %d, want 1", len(out.Issues))
	}
}

// TestGetByID_SuccessCov verifies the behavior of get by i d success cov.
func TestGetByID_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := GetByID(context.Background(), client, GetByIDInput{IssueID: 10})
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("GetByID IID = %d, want 10", out.IID)
	}
}

// TestReorder_SuccessCov verifies the behavior of reorder success cov.
func TestReorder_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	afterID := int64(5)
	out, err := Reorder(context.Background(), client, ReorderInput{ProjectID: testProjectID, IssueIID: 10, MoveAfterID: &afterID})
	if err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Reorder IID = %d, want 10", out.IID)
	}
}

// TestMove_SuccessCov verifies the behavior of move success cov.
func TestMove_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Move(context.Background(), client, MoveInput{ProjectID: testProjectID, IssueIID: 10, ToProjectID: 99})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Move IID = %d, want 10", out.IID)
	}
}

// TestSubscribe_SuccessCov verifies the behavior of subscribe success cov.
func TestSubscribe_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Subscribe(context.Background(), client, SubscribeInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Subscribe IID = %d, want 10", out.IID)
	}
}

// TestUnsubscribe_SuccessCov verifies the behavior of unsubscribe success cov.
func TestUnsubscribe_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := Unsubscribe(context.Background(), client, UnsubscribeInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("Unsubscribe IID = %d, want 10", out.IID)
	}
}

// TestCreateTodo_SuccessCov verifies the behavior of create todo success cov.
func TestCreateTodo_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := CreateTodo(context.Background(), client, CreateTodoInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("CreateTodo ID = %d, want 1", out.ID)
	}
	if out.TargetTitle != "Test" {
		t.Errorf("CreateTodo TargetTitle = %q, want Test", out.TargetTitle)
	}
}

// TestSetTimeEstimate_SuccessCov verifies the behavior of set time estimate success cov.
func TestSetTimeEstimate_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{ProjectID: testProjectID, IssueIID: 10, Duration: "3h"})
	if err != nil {
		t.Fatalf("SetTimeEstimate: %v", err)
	}
	if out.TimeEstimate != 10800 {
		t.Errorf("SetTimeEstimate TimeEstimate = %d, want 10800", out.TimeEstimate)
	}
}

// TestResetTimeEstimate_SuccessCov verifies the behavior of reset time estimate success cov.
func TestResetTimeEstimate_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ResetTimeEstimate(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ResetTimeEstimate: %v", err)
	}
	if out.TimeEstimate != 10800 {
		t.Errorf("ResetTimeEstimate TimeEstimate = %d, want 10800", out.TimeEstimate)
	}
}

// TestAddSpentTime_SuccessCov verifies the behavior of add spent time success cov.
func TestAddSpentTime_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10, Duration: "1h"})
	if err != nil {
		t.Fatalf("AddSpentTime: %v", err)
	}
	if out.TotalTimeSpent != 3600 {
		t.Errorf("AddSpentTime TotalTimeSpent = %d, want 3600", out.TotalTimeSpent)
	}
}

// TestAddSpentTime_WithSummaryCov verifies the behavior of add spent time with summary cov.
func TestAddSpentTime_WithSummaryCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{ProjectID: testProjectID, IssueIID: 10, Duration: "1h", Summary: "debugging"})
	if err != nil {
		t.Fatalf("AddSpentTime with summary: %v", err)
	}
	if out.TotalTimeSpent != 3600 {
		t.Errorf("TotalTimeSpent = %d, want 3600", out.TotalTimeSpent)
	}
}

// TestResetSpentTime_SuccessCov verifies the behavior of reset spent time success cov.
func TestResetSpentTime_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ResetSpentTime(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ResetSpentTime: %v", err)
	}
	if out.TimeEstimate != 10800 {
		t.Errorf("ResetSpentTime TimeEstimate = %d, want 10800", out.TimeEstimate)
	}
}

// TestGetTimeStats_SuccessCov verifies the behavior of get time stats success cov.
func TestGetTimeStats_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := GetTimeStats(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("GetTimeStats: %v", err)
	}
	if out.HumanTimeEstimate != "3h" {
		t.Errorf("GetTimeStats HumanTimeEstimate = %q, want 3h", out.HumanTimeEstimate)
	}
}

// TestGetParticipants_SuccessCov verifies the behavior of get participants success cov.
func TestGetParticipants_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := GetParticipants(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("GetParticipants: %v", err)
	}
	if len(out.Participants) != 1 {
		t.Fatalf("GetParticipants len = %d, want 1", len(out.Participants))
	}
	if out.Participants[0].Username != "alice" {
		t.Errorf("Participant username = %q, want alice", out.Participants[0].Username)
	}
}

// TestListMRsClosing_SuccessCov verifies the behavior of list m rs closing success cov.
func TestListMRsClosing_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListMRsClosing(context.Background(), client, ListMRsClosingInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ListMRsClosing: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("ListMRsClosing len = %d, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Author != "bob" {
		t.Errorf("MR author = %q, want bob", out.MergeRequests[0].Author)
	}
}

// TestListMRsRelated_SuccessCov verifies the behavior of list m rs related success cov.
func TestListMRsRelated_SuccessCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListMRsRelated(context.Background(), client, ListMRsRelatedInput{ProjectID: testProjectID, IssueIID: 10})
	if err != nil {
		t.Fatalf("ListMRsRelated: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("ListMRsRelated len = %d, want 1", len(out.MergeRequests))
	}
}

// ---------------------------------------------------------------------------
// List with all filter fields (to cover filter branches)
// ---------------------------------------------------------------------------.

// TestListAll_FilterFieldsCov verifies the behavior of list all filter fields cov.
func TestListAll_FilterFieldsCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	conf := true
	out, err := ListAll(context.Background(), client, ListAllInput{
		State: "opened", Labels: "bug,feat", Milestone: "v1",
		Scope: "assigned_to_me", Search: "test",
		AssigneeUsername: "alice", AuthorUsername: "bob",
		OrderBy: "created_at", Sort: "desc",
		CreatedAfter: testCreatedAfterCov, CreatedBefore: testCreatedBeforeCov,
		UpdatedAfter: testCreatedAfterCov, UpdatedBefore: testCreatedBeforeCov,
		Confidential: &conf,
	})
	if err != nil {
		t.Fatalf("ListAll with filters: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListAll with filters len = %d, want 1", len(out.Issues))
	}
}

// TestListAll_WithPagination verifies the behavior of list all with pagination.
func TestListAll_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListAll(context.Background(), client, ListAllInput{
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("ListAll with pagination: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListAll with pagination len = %d, want 1", len(out.Issues))
	}
}

// TestListGroup_AllFilterFieldsCov verifies the behavior of list group all filter fields cov.
func TestListGroup_AllFilterFieldsCov(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: "99", State: "opened", Labels: "bug",
		Milestone: "v1", Search: "test", Scope: "all",
		AuthorUsername: "bob",
		CreatedAfter:   testCreatedAfterCov, CreatedBefore: testCreatedBeforeCov,
		UpdatedAfter: testCreatedAfterCov, UpdatedBefore: testCreatedBeforeCov,
	})
	if err != nil {
		t.Fatalf("ListGroup with filters: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListGroup with filters len = %d, want 1", len(out.Issues))
	}
}

// TestListGroup_WithPagination verifies the behavior of list group with pagination.
func TestListGroup_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:         "99",
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("ListGroup with pagination: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Errorf("ListGroup with pagination len = %d, want 1", len(out.Issues))
	}
}

// TestListMRsClosing_WithPagination verifies the behavior of list m rs closing with pagination.
func TestListMRsClosing_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListMRsClosing(context.Background(), client, ListMRsClosingInput{
		ProjectID: testProjectID, IssueIID: 10,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("ListMRsClosing with pagination: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Errorf("len = %d, want 1", len(out.MergeRequests))
	}
}

// TestListMRsRelated_WithPagination verifies the behavior of list m rs related with pagination.
func TestListMRsRelated_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))
	out, err := ListMRsRelated(context.Background(), client, ListMRsRelatedInput{
		ProjectID: testProjectID, IssueIID: 10,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("ListMRsRelated with pagination: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Errorf("len = %d, want 1", len(out.MergeRequests))
	}
}

// ---------------------------------------------------------------------------
// RegisterTools MCP integration test
// ---------------------------------------------------------------------------.

// newIssueMCPSession is an internal helper for the issues package.
func newIssueMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))

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

// callToolAndVerify is an internal helper for the issues package.
func callToolAndVerify(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
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
	session := newIssueMCPSession(t)
	ctx := context.Background()
	pid := testProjectID

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_create", map[string]any{"project_id": pid, "title": "Test"}},
		{"gitlab_issue_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_list", map[string]any{"project_id": pid}},
		{"gitlab_issue_update", map[string]any{"project_id": pid, "issue_iid": 10, "title": "Updated"}},
		{"gitlab_issue_delete", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_list_group", map[string]any{"group_id": "99"}},
		{"gitlab_issue_list_all", map[string]any{}},
		{"gitlab_issue_get_by_id", map[string]any{"issue_id": 10}},
		{"gitlab_issue_reorder", map[string]any{"project_id": pid, "issue_iid": 10, "move_after_id": 5}},
		{"gitlab_issue_move", map[string]any{"project_id": pid, "issue_iid": 10, "to_project_id": 99}},
		{"gitlab_issue_subscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_unsubscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_create_todo", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_estimate_set", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "3h"}},
		{"gitlab_issue_time_estimate_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_spent_time_add", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "1h"}},
		{"gitlab_issue_spent_time_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_stats_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_participants", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_closing", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_related", map[string]any{"project_id": pid, "issue_iid": 10}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			callToolAndVerify(t, session, ctx, tt.name, tt.args)
		})
	}
}

// ---------------------------------------------------------------------------
// Confidential workflow edge cases
// ---------------------------------------------------------------------------.

// TestCreate_ConfidentialTrue verifies that Create passes the confidential=true
// flag to the API and the output reflects it.
func TestCreate_ConfidentialTrue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v, ok := body["confidential"].(bool); !ok || !v {
				t.Errorf("confidential = %v, want true", body["confidential"])
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"iid":10,"title":"Secret","description":"","state":"opened","labels":[],"assignees":[],"author":{"username":"alice"},"web_url":"https://gitlab.example.com/project/issues/10","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z","confidential":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	confidential := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		Title:        "Secret",
		Confidential: &confidential,
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if !out.Confidential {
		t.Error(msgConfidentialWant)
	}
}

// TestCreate_ConfidentialFalse verifies that Create passes confidential=false
// explicitly when the caller sets it to false.
func TestCreate_ConfidentialFalse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v, ok := body["confidential"].(bool); !ok || v {
				t.Errorf("confidential = %v, want false", body["confidential"])
			}
			testutil.RespondJSON(w, http.StatusCreated, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	confidential := false
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    testProjectID,
		Title:        testIssueTitle,
		Confidential: &confidential,
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if out.Confidential {
		t.Error("out.Confidential = true, want false")
	}
}

// TestUpdate_ConfidentialToggle verifies that Update can toggle the
// confidential flag from true to false.
func TestUpdate_ConfidentialToggle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssue10 {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v, ok := body["confidential"].(bool); !ok || v {
				t.Errorf("confidential = %v, want false", body["confidential"])
			}
			testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	confidential := false
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    testProjectID,
		IssueIID:     10,
		Confidential: &confidential,
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if out.Confidential {
		t.Error("out.Confidential = true, want false")
	}
}

// TestList_ConfidentialFilter verifies that List passes the confidential
// filter query parameter to the API.
func TestList_ConfidentialFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssues {
			testutil.AssertQueryParam(t, r, "confidential", "true")
			testutil.RespondJSON(w, http.StatusOK, "["+issueJSONEnriched+"]")
			return
		}
		http.NotFound(w, r)
	}))

	confidential := true
	out, err := List(context.Background(), client, ListInput{
		ProjectID:    testProjectID,
		Confidential: &confidential,
	})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtIssueCountWant1, len(out.Issues))
	}
	if !out.Issues[0].Confidential {
		t.Error(msgConfidentialWant)
	}
}

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestCreate_AssigneeIDSingular verifies that assignee_id (singular) is sent
// in the HTTP request body when creating an issue.
func TestCreate_AssigneeIDSingular(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssues {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if _, ok := body["assignee_id"]; !ok {
				t.Errorf("request body missing assignee_id field: %v", body)
			}
			if v, _ := body["assignee_id"].(float64); int64(v) != 28 {
				t.Errorf("assignee_id = %v, want 28", body["assignee_id"])
			}
			testutil.RespondJSON(w, http.StatusCreated, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		Title:      "Issue with single assignee",
		AssigneeID: 28,
	})
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
}

// TestUpdate_AssigneeIDSingular verifies that assignee_id (singular) is sent
// in the HTTP request body when updating an issue.
func TestUpdate_AssigneeIDSingular(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssue10 {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if _, ok := body["assignee_id"]; !ok {
				t.Errorf("request body missing assignee_id field: %v", body)
			}
			if v, _ := body["assignee_id"].(float64); int64(v) != 28 {
				t.Errorf("assignee_id = %v, want 28", body["assignee_id"])
			}
			testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  "42",
		IssueIID:   10,
		AssigneeID: 28,
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
}

// TestList_AllFilterFields verifies that List forwards Milestone, Scope,
// AssigneeUsername, AuthorUsername, OrderBy and Sort to the API.
func TestList_AllFilterFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("milestone") != "v1.0" {
			t.Errorf("milestone = %q, want v1.0", q.Get("milestone"))
		}
		if q.Get("scope") != "created_by_me" {
			t.Errorf("scope = %q, want created_by_me", q.Get("scope"))
		}
		if q.Get("assignee_username") != "alice" {
			t.Errorf("assignee_username = %q, want alice", q.Get("assignee_username"))
		}
		if q.Get("author_username") != "bob" {
			t.Errorf("author_username = %q, want bob", q.Get("author_username"))
		}
		if q.Get("order_by") != "updated_at" {
			t.Errorf("order_by = %q, want updated_at", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+issueJSONMinimal+`]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:        testProjectID,
		Milestone:        "v1.0",
		Scope:            "created_by_me",
		AssigneeUsername: "alice",
		AuthorUsername:   "bob",
		OrderBy:          "updated_at",
		Sort:             "desc",
	})
	if err != nil {
		t.Fatalf(fmtIssueListErr, err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf(fmtIssueCountWant1, len(out.Issues))
	}
}

// TestMove_NotFound verifies the 404 hint path in Move.
func TestMove_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Move(context.Background(), client, MoveInput{
		ProjectID: testProjectID, IssueIID: 10, ToProjectID: 999,
	})
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "target project") {
		t.Errorf("error = %q, want hint about target project", err.Error())
	}
}

// TestMove_BadRequest verifies the 400 hint path in Move.
func TestMove_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Move(context.Background(), client, MoveInput{
		ProjectID: testProjectID, IssueIID: 10, ToProjectID: 2,
	})
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
}

// TestMove_GenericError verifies the generic error path in Move.
func TestMove_GenericError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Move(context.Background(), client, MoveInput{
		ProjectID: testProjectID, IssueIID: 10, ToProjectID: 2,
	})
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}

// TestSubscribe_EOF verifies the EOF fallback in Subscribe (304 Not Modified).
func TestSubscribe_EOF(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/projects/42/issues/10/subscribe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	handler.HandleFunc("GET /api/v4/projects/42/issues/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Subscribe(context.Background(), client, SubscribeInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("IID = %d, want 10", out.IID)
	}
}

// TestSubscribe_APIError verifies the generic error path in Subscribe.
func TestSubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Subscribe(context.Background(), client, SubscribeInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}

// TestUnsubscribe_EOF verifies the EOF fallback in Unsubscribe.
func TestUnsubscribe_EOF(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/projects/42/issues/10/unsubscribe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	handler.HandleFunc("GET /api/v4/projects/42/issues/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Unsubscribe(context.Background(), client, UnsubscribeInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IID != 10 {
		t.Errorf("IID = %d, want 10", out.IID)
	}
}

// TestUnsubscribe_APIError verifies the generic error path in Unsubscribe.
func TestUnsubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Unsubscribe(context.Background(), client, UnsubscribeInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
}

// TestFormatGetMarkdown_EdgeCases covers References, IssueType, ClosedBy fields.
func TestFormatGetMarkdown_EdgeCases(t *testing.T) {
	out := Output{
		IID: 1, Title: "Test", State: "closed",
		References: "test/project#1", IssueType: "incident",
		ClosedBy: "admin", ClosedAt: "2025-01-01T00:00:00Z",
	}
	md := FormatMarkdown(out)
	if !strings.Contains(md, "test/project#1") {
		t.Error("expected references in output")
	}
	if !strings.Contains(md, "incident") {
		t.Error("expected issue type in output")
	}
	if !strings.Contains(md, "admin") {
		t.Error("expected closed-by in output")
	}
}

// TestCreateTodo_APIError verifies the error path in CreateTodo.
func TestCreateTodo_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := CreateTodo(context.Background(), client, CreateTodoInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestSetTimeEstimate_APIError verifies the error path in SetTimeEstimate.
func TestSetTimeEstimate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := SetTimeEstimate(context.Background(), client, SetTimeEstimateInput{
		ProjectID: testProjectID, IssueIID: 10, Duration: "1h",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestResetTimeEstimate_APIError verifies the error path.
func TestResetTimeEstimate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ResetTimeEstimate(context.Background(), client, GetInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestAddSpentTime_APIError verifies the error path.
func TestAddSpentTime_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := AddSpentTime(context.Background(), client, AddSpentTimeInput{
		ProjectID: testProjectID, IssueIID: 10, Duration: "2h",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestResetSpentTime_APIError verifies the error path.
func TestResetSpentTime_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ResetSpentTime(context.Background(), client, GetInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetTimeStats_APIError verifies the error path.
func TestGetTimeStats_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := GetTimeStats(context.Background(), client, GetInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetByID_APIError verifies the error path.
func TestGetByID_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := GetByID(context.Background(), client, GetByIDInput{IssueID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestReorder_APIError verifies the error path.
func TestReorder_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := Reorder(context.Background(), client, ReorderInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListAll_APIError verifies the error path.
func TestListAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListAll(context.Background(), client, ListAllInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListGroup_APIError verifies the error path.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "g"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetParticipants_APIError verifies the error path.
func TestGetParticipants_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := GetParticipants(context.Background(), client, GetInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListMRsClosing_APIError verifies the error path.
func TestListMRsClosing_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListMRsClosing(context.Background(), client, ListMRsClosingInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListMRsRelated_APIError verifies the error path.
func TestListMRsRelated_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListMRsRelated(context.Background(), client, ListMRsRelatedInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestUpdate_ExtraBranches covers AddLabels, RemoveLabels, Confidential, IssueType,
// Weight, EpicID, and DiscussionLocked branches in buildUpdateOpts.
func TestUpdate_ExtraBranches(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, issueJSONMinimal)
	}))
	conf := true
	locked := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: testProjectID, IssueIID: 10,
		AddLabels: "new-label", RemoveLabels: "old-label",
		Confidential: &conf, IssueType: "incident", Weight: 5,
		EpicID: 42, DiscussionLocked: &locked,
	})
	if err != nil {
		t.Fatalf(fmtIssueUpdateErr, err)
	}
	if out.IID != 10 {
		t.Errorf(fmtIIDWant10, out.IID)
	}
}

// TestDelete_Forbidden verifies Delete error path.
func TestDelete_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: testProjectID, IssueIID: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

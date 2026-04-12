// milestones_test.go contains unit tests for GitLab milestone listing operations.
// Tests use httptest to mock the GitLab Milestones API.
package milestones

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpectedNil        = "expected error, got nil"
	pathProjectMilestones = "/api/v4/projects/42/milestones"
	fmtMilestoneListErr   = "milestoneList() unexpected error: %v"
)

// TestMilestoneList_Success verifies the behavior of milestone list success.
func TestMilestoneList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":1,
					"iid":1,
					"project_id":42,
					"title":"v1.0",
					"description":"First release",
					"state":"active",
					"start_date":"2026-01-01",
					"due_date":"2026-03-31",
					"web_url":"https://gitlab.example.com/mygroup/api/-/milestones/1",
					"created_at":"2026-01-01T00:00:00Z",
					"updated_at":"2026-01-15T10:00:00Z",
					"expired":false
				},
				{
					"id":2,
					"iid":2,
					"project_id":42,
					"title":"v2.0",
					"description":"Second release",
					"state":"closed",
					"web_url":"https://gitlab.example.com/mygroup/api/-/milestones/2",
					"created_at":"2026-02-01T00:00:00Z",
					"updated_at":"2026-02-28T10:00:00Z",
					"expired":true
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
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 2 {
		t.Fatalf("len(Milestones) = %d, want 2", len(out.Milestones))
	}
	if out.Milestones[0].Title != "v1.0" {
		t.Errorf("Milestones[0].Title = %q, want %q", out.Milestones[0].Title, "v1.0")
	}
	if out.Milestones[0].State != "active" {
		t.Errorf("Milestones[0].State = %q, want %q", out.Milestones[0].State, "active")
	}
	if out.Milestones[1].Expired != true {
		t.Errorf("Milestones[1].Expired = %v, want true", out.Milestones[1].Expired)
	}
	if out.Milestones[0].WebURL == "" {
		t.Error("Milestones[0].WebURL is empty")
	}
}

// TestMilestoneList_WithStateFilter verifies the behavior of milestone list with state filter.
func TestMilestoneList_WithStateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			q := r.URL.Query()
			if q.Get("state") != "active" {
				t.Errorf("expected state=active, got %q", q.Get("state"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0","state":"active","web_url":"https://gitlab.example.com/-/milestones/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		State:     "active",
	})
	if err != nil {
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestMilestoneList_WithSearch verifies the behavior of milestone list with search.
func TestMilestoneList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			q := r.URL.Query()
			if q.Get("search") != "v1" {
				t.Errorf("expected search=v1, got %q", q.Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0","state":"active","web_url":"https://gitlab.example.com/-/milestones/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "v1",
	})
	if err != nil {
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestMilestoneList_EmptyProjectID verifies the behavior of milestone list empty project i d.
func TestMilestoneList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("milestoneList() expected error for empty project_id, got nil")
	}
}

// TestMilestoneListServer_Error verifies the behavior of milestone list server error.
func TestMilestoneListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("milestoneList() expected error, got nil")
	}
}

// ---------- Get ----------.

// TestMilestoneGet_Success verifies the behavior of milestone get success.
func TestMilestoneGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"v1.0",
				"description":"First release","state":"active",
				"start_date":"2026-01-01","due_date":"2026-03-31",
				"web_url":"https://gitlab.example.com/-/milestones/1",
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z",
				"expired":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGet() unexpected error: %v", err)
	}
	if out.Title != "v1.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.0")
	}
	if out.State != "active" {
		t.Errorf("State = %q, want %q", out.State, "active")
	}
}

// TestMilestoneGet_MissingParams verifies the behavior of milestone get missing params.
func TestMilestoneGet_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Get(context.Background(), client, GetInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Get(context.Background(), client, GetInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetServer_Error verifies the behavior of milestone get server error.
func TestMilestoneGetServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- Create ----------.

// TestMilestoneCreate_Success verifies the behavior of milestone create success.
func TestMilestoneCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectMilestones {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":3,"iid":3,"project_id":42,"title":"v3.0",
				"description":"Third release","state":"active",
				"start_date":"2026-06-01","due_date":"2026-09-30",
				"web_url":"https://gitlab.example.com/-/milestones/3",
				"created_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Title:       "v3.0",
		Description: "Third release",
		StartDate:   "2026-06-01",
		DueDate:     "2026-09-30",
	})
	if err != nil {
		t.Fatalf("milestoneCreate() unexpected error: %v", err)
	}
	if out.Title != "v3.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v3.0")
	}
	if out.StartDate != "2026-06-01" {
		t.Errorf("StartDate = %q, want %q", out.StartDate, "2026-06-01")
	}
}

// TestMilestoneCreate_MissingParams verifies the behavior of milestone create missing params.
func TestMilestoneCreate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Create(context.Background(), client, CreateInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Create(context.Background(), client, CreateInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for empty title")
	}
}

// TestMilestoneCreate_InvalidDate verifies the behavior of milestone create invalid date.
func TestMilestoneCreate_InvalidDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1.0", StartDate: "bad-date"})
	if err == nil {
		t.Fatal("expected error for invalid start_date")
	}
	_, err = Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1.0", DueDate: "bad-date"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// ---------- Update ----------.

// TestMilestoneUpdate_Success verifies the behavior of milestone update success.
func TestMilestoneUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodPut && r.URL.Path == pathProjectMilestones+"/1" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"v1.1",
				"description":"Updated","state":"active",
				"web_url":"https://gitlab.example.com/-/milestones/1",
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-05-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    "42",
		MilestoneIID: 1,
		Title:        "v1.1",
		Description:  "Updated",
	})
	if err != nil {
		t.Fatalf("milestoneUpdate() unexpected error: %v", err)
	}
	if out.Title != "v1.1" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.1")
	}
}

// TestMilestoneUpdate_MissingParams verifies the behavior of milestone update missing params.
func TestMilestoneUpdate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Update(context.Background(), client, UpdateInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneUpdate_InvalidDate verifies the behavior of milestone update invalid date.
func TestMilestoneUpdate_InvalidDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, StartDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid start_date")
	}
	_, err = Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, DueDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// ---------- Delete ----------.

// TestMilestoneDelete_Success verifies the behavior of milestone delete success.
func TestMilestoneDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == pathProjectMilestones+"/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneDelete() unexpected error: %v", err)
	}
}

// TestMilestoneDelete_MissingParams verifies the behavior of milestone delete missing params.
func TestMilestoneDelete_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if err := Delete(context.Background(), client, DeleteInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneDeleteServer_Error verifies the behavior of milestone delete server error.
func TestMilestoneDeleteServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- GetIssues ----------.

// TestMilestoneGetIssues_Success verifies the behavior of milestone get issues success.
func TestMilestoneGetIssues_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1/issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":10,"iid":1,"title":"Bug fix","state":"opened","web_url":"https://gitlab.example.com/-/issues/1","created_at":"2026-01-05T00:00:00Z"},
				{"id":11,"iid":2,"title":"Feature","state":"closed","web_url":"https://gitlab.example.com/-/issues/2","created_at":"2026-01-06T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGetIssues() unexpected error: %v", err)
	}
	if len(out.Issues) != 2 {
		t.Fatalf("len(Issues) = %d, want 2", len(out.Issues))
	}
	if out.Issues[0].Title != "Bug fix" {
		t.Errorf("Issues[0].Title = %q, want %q", out.Issues[0].Title, "Bug fix")
	}
	if out.Issues[1].State != "closed" {
		t.Errorf("Issues[1].State = %q, want %q", out.Issues[1].State, "closed")
	}
}

// TestMilestoneGetIssues_MissingParams verifies the behavior of milestone get issues missing params.
func TestMilestoneGetIssues_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := GetIssues(context.Background(), client, GetIssuesInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetIssuesServer_Error verifies the behavior of milestone get issues server error.
func TestMilestoneGetIssuesServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- GetMergeRequests ----------.

// TestMilestoneGetMergeRequests_Success verifies the behavior of milestone get merge requests success.
func TestMilestoneGetMergeRequests_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":20,"iid":1,"title":"Add feature X","state":"merged","source_branch":"feature-x","target_branch":"main","web_url":"https://gitlab.example.com/-/merge_requests/1","created_at":"2026-02-01T00:00:00Z"},
				{"id":21,"iid":2,"title":"Fix bug Y","state":"opened","source_branch":"fix-y","target_branch":"main","web_url":"https://gitlab.example.com/-/merge_requests/2","created_at":"2026-02-02T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGetMergeRequests() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("len(MergeRequests) = %d, want 2", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "Add feature X" {
		t.Errorf("MergeRequests[0].Title = %q, want %q", out.MergeRequests[0].Title, "Add feature X")
	}
	if out.MergeRequests[0].SourceBranch != "feature-x" {
		t.Errorf("MergeRequests[0].SourceBranch = %q, want %q", out.MergeRequests[0].SourceBranch, "feature-x")
	}
	if out.MergeRequests[1].State != "opened" {
		t.Errorf("MergeRequests[1].State = %q, want %q", out.MergeRequests[1].State, "opened")
	}
}

// TestMilestoneGetMergeRequests_MissingParams verifies the behavior of milestone get merge requests missing params.
func TestMilestoneGetMergeRequests_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetMergeRequestsServer_Error verifies the behavior of milestone get merge requests server error.
func TestMilestoneGetMergeRequestsServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestMilestoneList_CancelledContext verifies the behavior of milestone list cancelled context.
func TestMilestoneList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("milestoneList() expected error for canceled context, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// JSON fixtures
// ---------------------------------------------------------------------------.

const (
	errExpCancelledCtx      = "expected error for canceled context"
	fmtUnexpErr             = "unexpected error: %v"
	covMilestoneJSON        = `{"id":1,"iid":1,"project_id":42,"title":"v1.0","description":"First release","state":"active","start_date":"2026-01-01","due_date":"2026-03-31","web_url":"https://gitlab.example.com/-/milestones/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z","expired":false}`
	covMilestoneMinimalJSON = `{"id":2,"iid":2,"project_id":42,"title":"v2.0","state":"closed"}`
	covMilestoneGroupJSON   = `{"id":3,"iid":3,"project_id":42,"title":"v3.0","state":"active","group_id":99}`
	covMilestoneListJSON    = `[` + covMilestoneJSON + `]`
	covIssuePath            = "/api/v4/projects/42/milestones/1/issues"
	covMRPath               = "/api/v4/projects/42/milestones/1/merge_requests"
	covIssueJSON            = `[{"id":10,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1","created_at":"2026-01-05T00:00:00Z"}]`
	covIssueNoDateJSON      = `[{"id":11,"iid":2,"title":"Feature","state":"closed"}]`
	covMRJSON               = `[{"id":20,"iid":1,"title":"Feature X","state":"merged","source_branch":"feat-x","target_branch":"main","web_url":"https://example.com/mr/1","created_at":"2026-02-01T00:00:00Z"}]`
	covMRNoDateJSON         = `[{"id":21,"iid":2,"title":"Fix Y","state":"opened","source_branch":"fix-y","target_branch":"main"}]`
)

// ---------------------------------------------------------------------------
// List — additional coverage
// ---------------------------------------------------------------------------.

// TestList_IncludeAncestors verifies the behavior of cov list include ancestors.
func TestList_IncludeAncestors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("include_ancestors") != "true" {
			t.Errorf("expected include_ancestors=true, got %q", r.URL.Query().Get("include_ancestors"))
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IncludeAncestors: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithIIDs verifies the behavior of cov list with i i ds.
func TestList_WithIIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		q := r.URL.Query()
		iids := q["iids[]"]
		if len(iids) != 2 || iids[0] != "1" || iids[1] != "2" {
			t.Errorf("expected iids[]=[1,2], got %v", iids)
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IIDs: []int64{1, 2}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithTitleFilter verifies the behavior of cov list with title filter.
func TestList_WithTitleFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("title") != "v1.0" {
			t.Errorf("expected title=v1.0, got %q", r.URL.Query().Get("title"))
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", Title: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithPagination verifies the behavior of cov list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("expected per_page=5, got %q", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMilestoneListJSON,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:       "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// ---------------------------------------------------------------------------
// Get — canceled context
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of cov get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Create — server error, canceled context
// ---------------------------------------------------------------------------.

// TestCreate_ServerError verifies the behavior of cov create server error.
func TestCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestCreate_CancelledContext verifies the behavior of cov create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Title: "v1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — state_event, server error, canceled context
// ---------------------------------------------------------------------------.

// TestUpdate_WithStateEvent verifies the behavior of cov update with state event.
func TestUpdate_WithStateEvent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", MilestoneIID: 1, StateEvent: "close",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdate_ServerError verifies the behavior of cov update server error.
func TestUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, Title: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestUpdate_CancelledContext verifies the behavior of cov update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", MilestoneIID: 1, Title: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Delete — canceled context
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies the behavior of cov delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetIssues — canceled context, pagination, no created_at
// ---------------------------------------------------------------------------.

// TestGetIssues_CancelledContext verifies the behavior of cov get issues cancelled context.
func TestGetIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetIssues(ctx, client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetIssues_WithPagination verifies the behavior of cov get issues with pagination.
func TestGetIssues_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covIssueJSON,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "1", TotalPages: "1"})
	}))
	out, err := GetIssues(context.Background(), client, GetIssuesInput{
		ProjectID:       "42",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestGetIssues_NoCreatedAt verifies the behavior of cov get issues no created at.
func TestGetIssues_NoCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covIssueNoDateJSON)
	}))
	out, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Issues[0].CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.Issues[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// GetMergeRequests — canceled context, pagination, no created_at
// ---------------------------------------------------------------------------.

// TestGetMergeRequests_CancelledContext verifies the behavior of cov get merge requests cancelled context.
func TestGetMergeRequests_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetMergeRequests(ctx, client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetMergeRequests_WithPagination verifies the behavior of cov get merge requests with pagination.
func TestGetMergeRequests_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.URL.Query().Get("page") != "3" {
			t.Errorf("expected page=3, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMRJSON,
			testutil.PaginationHeaders{Page: "3", PerPage: "5", Total: "1", TotalPages: "1"})
	}))
	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{
		ProjectID:       "42",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("expected page 3, got %d", out.Pagination.Page)
	}
}

// TestGetMergeRequests_NoCreatedAt verifies the behavior of cov get merge requests no created at.
func TestGetMergeRequests_NoCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covMRNoDateJSON)
	}))
	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MergeRequests[0].CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.MergeRequests[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies the behavior of cov format markdown all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	o := Output{
		ID: 1, IID: 1, ProjectID: 42, Title: "v1.0", Description: "First release",
		State: "active", StartDate: "2026-01-01", DueDate: "2026-03-31",
		WebURL: "https://example.com/-/milestones/1", CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-15T10:00:00Z", Expired: false,
	}
	md := FormatMarkdown(o)
	for _, want := range []string{"v1.0", "active", "First release", "Start Date", "Due Date", "Created", "Updated", "URL"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q in:\n%s", want, md)
		}
	}
}

// TestFormatMarkdown_Minimal verifies the behavior of cov format markdown minimal.
func TestFormatMarkdown_Minimal(t *testing.T) {
	o := Output{ID: 2, IID: 2, Title: "v2.0", State: "closed"}
	md := FormatMarkdown(o)
	if strings.Contains(md, "Start Date") {
		t.Error("minimal milestone should not show Start Date")
	}
	if strings.Contains(md, "Due Date") {
		t.Error("minimal milestone should not show Due Date")
	}
	if strings.Contains(md, "Description") {
		t.Error("minimal milestone should not show Description")
	}
	if !strings.Contains(md, "v2.0") {
		t.Error("missing milestone title")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of cov format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No milestones found") {
		t.Errorf("expected 'No milestones found', got:\n%s", md)
	}
}

// TestFormatListMarkdownString_WithExpired verifies the behavior of cov format list markdown string with expired.
func TestFormatListMarkdownString_WithExpired(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{
			{IID: 1, Title: "v1.0", State: "active", DueDate: "2026-03-31", Expired: true},
			{IID: 2, Title: "v2.0", State: "active"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "Yes") {
		t.Errorf("expected 'Yes' for expired, got:\n%s", md)
	}
	if !strings.Contains(md, "No") {
		t.Errorf("expected 'No' for not expired, got:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatListMarkdown verifies the behavior of cov format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{{IID: 1, Title: "v1.0", State: "active"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestFormatListMarkdownString_ClickableLinks verifies that milestone IIDs
// in the list are rendered as clickable Markdown links [IID](weburl).
func TestFormatListMarkdownString_ClickableLinks(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{
			{IID: 3, Title: "v3.0", State: "active",
				WebURL: "https://gitlab.example.com/-/milestones/3"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "[3](https://gitlab.example.com/-/milestones/3)") {
		t.Errorf("expected clickable milestone link, got:\n%s", md)
	}
}

// TestFormatMarkdown_ClickableURL verifies that milestone detail renders
// the URL as a clickable Markdown link [url](url).
func TestFormatMarkdown_ClickableURL(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 1, IID: 1, Title: "v1.0", State: "active",
		WebURL: "https://gitlab.example.com/-/milestones/1",
	})
	if !strings.Contains(md, "[https://gitlab.example.com/-/milestones/1](https://gitlab.example.com/-/milestones/1)") {
		t.Errorf("expected clickable URL in detail, got:\n%s", md)
	}
}

// TestFormatMarkdown_NoURLWhenEmpty verifies that no URL line appears
// when WebURL is empty.
func TestFormatMarkdown_NoURLWhenEmpty(t *testing.T) {
	md := FormatMarkdown(Output{ID: 1, IID: 1, Title: "v1.0", State: "active"})
	if strings.Contains(md, "**URL**") {
		t.Errorf("expected no URL line when WebURL is empty, got:\n%s", md)
	}
}

// TestFormatIssuesMarkdownString_Empty verifies the behavior of cov format issues markdown string empty.
func TestFormatIssuesMarkdownString_Empty(t *testing.T) {
	md := FormatIssuesMarkdownString(MilestoneIssuesOutput{})
	if !strings.Contains(md, "No issues found") {
		t.Errorf("expected 'No issues found', got:\n%s", md)
	}
}

// TestFormatIssuesMarkdownString_WithIssues verifies the behavior of cov format issues markdown string with issues.
func TestFormatIssuesMarkdownString_WithIssues(t *testing.T) {
	out := MilestoneIssuesOutput{
		Issues: []IssueItem{
			{IID: 1, Title: "Bug fix", State: "opened", CreatedAt: "2026-01-05T00:00:00Z"},
			{IID: 2, Title: "Feature", State: "closed"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatIssuesMarkdownString(out)
	if !strings.Contains(md, "Bug fix") {
		t.Errorf("missing issue title:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatIssuesMarkdown verifies the behavior of cov format issues markdown.
func TestFormatIssuesMarkdown(t *testing.T) {
	out := MilestoneIssuesOutput{
		Issues:     []IssueItem{{IID: 1, Title: "x", State: "opened"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatIssuesMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestFormatMergeRequestsMarkdownString_Empty verifies the behavior of cov format merge requests markdown string empty.
func TestFormatMergeRequestsMarkdownString_Empty(t *testing.T) {
	md := FormatMergeRequestsMarkdownString(MilestoneMergeRequestsOutput{})
	if !strings.Contains(md, "No merge requests found") {
		t.Errorf("expected 'No merge requests found', got:\n%s", md)
	}
}

// TestFormatMergeRequestsMarkdownString_WithMRs verifies the behavior of cov format merge requests markdown string with m rs.
func TestFormatMergeRequestsMarkdownString_WithMRs(t *testing.T) {
	out := MilestoneMergeRequestsOutput{
		MergeRequests: []MergeRequestItem{
			{IID: 1, Title: "Feature X", State: "merged", SourceBranch: "feat-x", TargetBranch: "main", CreatedAt: "2026-02-01T00:00:00Z"},
			{IID: 2, Title: "Fix Y", State: "opened", SourceBranch: "fix-y", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatMergeRequestsMarkdownString(out)
	if !strings.Contains(md, "Feature X") {
		t.Errorf("missing MR title:\n%s", md)
	}
	if !strings.Contains(md, "feat-x") {
		t.Errorf("missing source branch:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatMergeRequestsMarkdown verifies the behavior of cov format merge requests markdown.
func TestFormatMergeRequestsMarkdown(t *testing.T) {
	out := MilestoneMergeRequestsOutput{
		MergeRequests: []MergeRequestItem{{IID: 1, Title: "x", State: "merged", SourceBranch: "a", TargetBranch: "main"}},
		Pagination:    toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatMergeRequestsMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic + MCP round-trip
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	const milestonePath = "/api/v4/projects/42/milestones"

	mux := http.NewServeMux()
	mux.HandleFunc(milestonePath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, covMilestoneListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, covMilestoneJSON)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(milestonePath+"/1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(milestonePath+"/1/issues", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, covIssueJSON,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	mux.HandleFunc(milestonePath+"/1/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMRJSON,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_milestone_list", map[string]any{"project_id": "42"}},
		{"gitlab_milestone_get", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_create", map[string]any{"project_id": "42", "title": "v1.0"}},
		{"gitlab_milestone_update", map[string]any{"project_id": "42", "milestone_iid": float64(1), "title": "v1.1"}},
		{"gitlab_milestone_delete", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_issues", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_merge_requests", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s): nil result", tc.name)
			}
		})
	}
}

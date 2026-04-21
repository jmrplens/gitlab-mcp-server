// groupmilestones_test.go contains unit tests for GitLab group milestone operations.
// Tests use httptest to mock the GitLab GroupMilestones API.
package groupmilestones

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
	pathGroupMilestones = "/api/v4/groups/10/milestones"
	pathMilestone1      = "/api/v4/groups/10/milestones/1"
	testMilestoneTitle  = "v1.0"
	testGroupID         = "10"
	testStateActive     = "active"
	testActionAdd       = "add"
	fmtTitleWant        = "Title = %q, want %q"
	milestoneJSON       = `{"id":1,"iid":1,"group_id":10,"title":"v1.0","description":"First release","state":"active","start_date":"2026-01-01","due_date":"2026-06-30","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T00:00:00Z","expired":false}`
)

// ---------- List ----------.

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+milestoneJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
	if out.Milestones[0].Title != testMilestoneTitle {
		t.Errorf(fmtTitleWant, out.Milestones[0].Title, testMilestoneTitle)
	}
	if out.Milestones[0].GroupID != 10 {
		t.Errorf("GroupID = %d, want 10", out.Milestones[0].GroupID)
	}
	if out.Milestones[0].GroupPath != testGroupID {
		t.Errorf("GroupPath = %q, want %q", out.Milestones[0].GroupPath, testGroupID)
	}
}

// TestList_WithFilters verifies the behavior of list with filters.
func TestList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones {
			q := r.URL.Query()
			if q.Get("state") != testStateActive {
				t.Errorf("expected state=active, got %q", q.Get("state"))
			}
			if q.Get("search") != "v1" {
				t.Errorf("expected search=v1, got %q", q.Get("search"))
			}
			if q.Get("include_ancestors") != "true" {
				t.Errorf("expected include_ancestors=true, got %q", q.Get("include_ancestors"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:          testGroupID,
		State:            testStateActive,
		Search:           "v1",
		IncludeAncestors: true,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestList_MissingGroupID verifies the behavior of list missing group i d.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id")
	}
}

// TestList_InvalidDate verifies the behavior of list invalid date.
func TestList_InvalidDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: testGroupID, UpdatedBefore: "not-a-date"})
	if err == nil {
		t.Fatal("List() expected error for invalid date")
	}
}

// ---------- Get ----------.

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1 {
			testutil.RespondJSON(w, http.StatusOK, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, MilestoneIID: 1})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Title != testMilestoneTitle {
		t.Errorf(fmtTitleWant, out.Title, testMilestoneTitle)
	}
	if out.State != testStateActive {
		t.Errorf("State = %q, want %q", out.State, testStateActive)
	}
}

// TestGet_MissingGroupID verifies the behavior of get missing group i d.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing group_id")
	}
}

// TestGet_MissingMilestoneIID verifies the behavior of get missing milestone i i d.
func TestGet_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Get() expected error for missing milestone_iid")
	}
}

// ---------- Create ----------.

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupMilestones {
			testutil.RespondJSON(w, http.StatusCreated, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: testGroupID,
		Title:   testMilestoneTitle,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Title != testMilestoneTitle {
		t.Errorf(fmtTitleWant, out.Title, testMilestoneTitle)
	}
}

// TestCreate_WithDates verifies the behavior of create with dates.
func TestCreate_WithDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupMilestones {
			testutil.RespondJSON(w, http.StatusCreated, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID:   testGroupID,
		Title:     testMilestoneTitle,
		StartDate: "2026-01-01",
		DueDate:   "2026-06-30",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// TestCreate_MissingGroupID verifies the behavior of create missing group i d.
func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Title: testMilestoneTitle})
	if err == nil {
		t.Fatal("Create() expected error for missing group_id")
	}
}

// TestCreate_InvalidDate verifies the behavior of create invalid date.
func TestCreate_InvalidDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{GroupID: testGroupID, Title: testMilestoneTitle, StartDate: "bad"})
	if err == nil {
		t.Fatal("Create() expected error for invalid date")
	}
}

// ---------- Update ----------.

// TestUpdate_Success verifies the behavior of update success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodPut && r.URL.Path == pathMilestone1 {
			testutil.RespondJSON(w, http.StatusOK, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:      testGroupID,
		MilestoneIID: 1,
		Title:        "v1.0-updated",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// TestUpdate_MissingGroupID verifies the behavior of update missing group i d.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("Update() expected error for missing group_id")
	}
}

// TestUpdate_MissingMilestoneIID verifies the behavior of update missing milestone i i d.
func TestUpdate_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Update() expected error for missing milestone_iid")
	}
}

// ---------- Delete ----------.

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == pathMilestone1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: testGroupID, MilestoneIID: 1})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_MissingGroupID verifies the behavior of delete missing group i d.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id")
	}
}

// ---------- GetIssues ----------.

// TestGetIssues_Success verifies the behavior of get issues success.
func TestGetIssues_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":100,"iid":5,"title":"Fix bug","state":"opened","web_url":"https://example.com/issues/5","created_at":"2026-01-10T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetIssues(context.Background(), client, GetIssuesInput{GroupID: testGroupID, MilestoneIID: 1})
	if err != nil {
		t.Fatalf("GetIssues() unexpected error: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(out.Issues))
	}
	if out.Issues[0].Title != "Fix bug" {
		t.Errorf(fmtTitleWant, out.Issues[0].Title, "Fix bug")
	}
	if out.Issues[0].WebURL != "https://example.com/issues/5" {
		t.Errorf("WebURL = %q, want %q", out.Issues[0].WebURL, "https://example.com/issues/5")
	}
}

// TestGetIssues_MissingMilestoneIID verifies the behavior of get issues missing milestone i i d.
func TestGetIssues_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := GetIssues(context.Background(), client, GetIssuesInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("GetIssues() expected error for missing milestone_iid")
	}
}

// ---------- GetMergeRequests ----------.

// TestGetMergeRequests_Success verifies the behavior of get merge requests success.
func TestGetMergeRequests_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":200,"iid":10,"title":"Feature MR","state":"merged","source_branch":"feature","target_branch":"main","web_url":"https://example.com/mr/10","created_at":"2026-02-01T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{GroupID: testGroupID, MilestoneIID: 1})
	if err != nil {
		t.Fatalf("GetMergeRequests() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "Feature MR" {
		t.Errorf(fmtTitleWant, out.MergeRequests[0].Title, "Feature MR")
	}
	if out.MergeRequests[0].SourceBranch != "feature" {
		t.Errorf("SourceBranch = %q, want %q", out.MergeRequests[0].SourceBranch, "feature")
	}
}

// TestGetMergeRequests_MissingGroupID verifies the behavior of get merge requests missing group i d.
func TestGetMergeRequests_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("GetMergeRequests() expected error for missing group_id")
	}
}

// ---------- GetBurndownChartEvents ----------.

// TestGetBurndownChartEvents_Success verifies the behavior of get burndown chart events success.
func TestGetBurndownChartEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/burndown_events" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"created_at":"2026-01-05T00:00:00Z","weight":3,"action":"add"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{GroupID: testGroupID, MilestoneIID: 1})
	if err != nil {
		t.Fatalf("GetBurndownChartEvents() unexpected error: %v", err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(out.Events))
	}
	if out.Events[0].Weight != 3 {
		t.Errorf("Weight = %d, want 3", out.Events[0].Weight)
	}
	if out.Events[0].Action != testActionAdd {
		t.Errorf("Action = %q, want %q", out.Events[0].Action, testActionAdd)
	}
}

// TestGetBurndownChartEvents_MissingMilestoneIID verifies the behavior of get burndown chart events missing milestone i i d.
func TestGetBurndownChartEvents_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("GetBurndownChartEvents() expected error for missing milestone_iid")
	}
}

// ---------- Formatters ----------.

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	out := Output{
		ID: 1, IID: 1, GroupID: 10, Title: testMilestoneTitle,
		Description: "Release", State: testStateActive,
		StartDate: "2026-01-01", DueDate: "2026-06-30",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-15T00:00:00Z",
	}
	md := FormatMarkdown(out)
	if md == "" {
		t.Fatal("FormatMarkdown returned empty string")
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Milestones: nil,
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 0, TotalPages: 1},
	}
	md := FormatListMarkdownString(out)
	if md == "" {
		t.Fatal("FormatListMarkdownString returned empty string")
	}
}

// TestFormatIssuesMarkdown verifies the behavior of format issues markdown.
func TestFormatIssuesMarkdown(t *testing.T) {
	out := IssuesOutput{
		Issues:     []IssueItem{{ID: 100, IID: 5, Title: "Fix", State: "opened"}},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1},
	}
	result := FormatIssuesMarkdown(out)
	if result == nil {
		t.Fatal("FormatIssuesMarkdown returned nil")
	}
}

// TestFormatMergeRequestsMarkdown verifies the behavior of format merge requests markdown.
func TestFormatMergeRequestsMarkdown(t *testing.T) {
	out := MergeRequestsOutput{
		MergeRequests: []MergeRequestItem{{ID: 200, IID: 10, Title: "Feature", State: "merged"}},
		Pagination:    toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1},
	}
	result := FormatMergeRequestsMarkdown(out)
	if result == nil {
		t.Fatal("FormatMergeRequestsMarkdown returned nil")
	}
}

// TestFormatBurndownChartEventsMarkdown verifies the behavior of format burndown chart events markdown.
func TestFormatBurndownChartEventsMarkdown(t *testing.T) {
	out := BurndownChartEventsOutput{
		Events:     []BurndownChartEventItem{{CreatedAt: "2026-01-05T00:00:00Z", Weight: 3, Action: testActionAdd}},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1},
	}
	result := FormatBurndownChartEventsMarkdown(out)
	if result == nil {
		t.Fatal("FormatBurndownChartEventsMarkdown returned nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const testDateStart = "2026-01-01"

const fmtMarkdownMissing = "markdown missing %q:\n%s"

const testTableHeaderID = "| ID |"

const fmtExpectedEmptyMsg = "expected empty message:\n%s"

// ---------------------------------------------------------------------------
// List — API error, canceled context, pagination, date filters
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_WithPagination verifies the behavior of list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones {
			q := r.URL.Query()
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %q", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", q.Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+milestoneJSON+`]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "6", TotalPages: "2", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{
		GroupID:         "10",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// TestList_InvalidUpdatedAfterDate verifies the behavior of list invalid updated after date.
func TestList_InvalidUpdatedAfterDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10", UpdatedAfter: "bad-date"})
	if err == nil {
		t.Fatal("expected error for invalid updated_after date")
	}
}

// TestList_InvalidContainingDate verifies the behavior of list invalid containing date.
func TestList_InvalidContainingDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10", ContainingDate: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid containing_date")
	}
}

// TestList_AllFilterParams verifies the behavior of list all filter params.
func TestList_AllFilterParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones {
			q := r.URL.Query()
			if q.Get("title") != "v1.0" {
				t.Errorf("expected title=v1.0, got %q", q.Get("title"))
			}
			if q.Get("search_title") != "v1" {
				t.Errorf("expected search_title=v1, got %q", q.Get("search_title"))
			}
			if q.Get("include_descendents") != "true" {
				t.Errorf("expected include_descendents=true, got %q", q.Get("include_descendents"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:            "10",
		Title:              "v1.0",
		SearchTitle:        "v1",
		IncludeDescendants: true,
		IIDs:               []int64{1, 2},
		UpdatedBefore:      "2026-12-31",
		UpdatedAfter:       testDateStart,
		ContainingDate:     "2026-06-15",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// ---------------------------------------------------------------------------
// Get — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, canceled context, invalid due_date
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Title: "v2.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{GroupID: "10", Title: "v2.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_InvalidDueDate verifies the behavior of create invalid due date.
func TestCreate_InvalidDueDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Title: "v2.0", DueDate: "not-valid"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// TestCreate_WithDescription verifies the behavior of create with description.
func TestCreate_WithDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupMilestones {
			testutil.RespondJSON(w, http.StatusCreated, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		GroupID:     "10",
		Title:       "v1.0",
		Description: "First release milestone",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Title != "v1.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.0")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, canceled context, invalid dates
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", MilestoneIID: 1, Title: "new"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_InvalidStartDate verifies the behavior of update invalid start date.
func TestUpdate_InvalidStartDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", MilestoneIID: 1, StartDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid start_date")
	}
}

// TestUpdate_InvalidDueDate verifies the behavior of update invalid due date.
func TestUpdate_InvalidDueDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", MilestoneIID: 1, DueDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodPut && r.URL.Path == pathMilestone1 {
			testutil.RespondJSON(w, http.StatusOK, milestoneJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:      "10",
		MilestoneIID: 1,
		Title:        "v1.0-final",
		Description:  "Updated desc",
		StartDate:    "2026-01-15",
		DueDate:      "2026-07-31",
		StateEvent:   "close",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, canceled context, missing milestone_iid
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDelete_MissingMilestoneIID verifies the behavior of delete missing milestone i i d.
func TestDelete_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for missing milestone_iid")
	}
}

// ---------------------------------------------------------------------------
// GetIssues — API error, canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestGetIssues_APIError verifies the behavior of get issues a p i error.
func TestGetIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetIssues(context.Background(), client, GetIssuesInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetIssues_CancelledContext verifies the behavior of get issues cancelled context.
func TestGetIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetIssues(ctx, client, GetIssuesInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetIssues_MissingGroupID verifies the behavior of get issues missing group i d.
func TestGetIssues_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := GetIssues(context.Background(), client, GetIssuesInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetIssues_WithPagination verifies the behavior of get issues with pagination.
func TestGetIssues_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":100,"iid":5,"title":"Bug","state":"opened","web_url":"https://example.com/issues/5","created_at":"2026-01-10T00:00:00Z"},{"id":101,"iid":6,"title":"Feature","state":"closed","created_at":"2026-01-11T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetIssues(context.Background(), client, GetIssuesInput{
		GroupID:         "10",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Issues) != 2 {
		t.Fatalf("len(Issues) = %d, want 2", len(out.Issues))
	}
	if out.Issues[0].CreatedAt == "" {
		t.Error("expected CreatedAt to be populated")
	}
	if out.Issues[1].WebURL != "" {
		t.Error("expected WebURL to be empty when not set in response")
	}
}

// ---------------------------------------------------------------------------
// GetMergeRequests — API error, canceled context, missing milestone_iid
// ---------------------------------------------------------------------------.

// TestGetMergeRequests_APIError verifies the behavior of get merge requests a p i error.
func TestGetMergeRequests_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetMergeRequests_CancelledContext verifies the behavior of get merge requests cancelled context.
func TestGetMergeRequests_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetMergeRequests(ctx, client, GetMergeRequestsInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetMergeRequests_MissingMilestoneIID verifies the behavior of get merge requests missing milestone i i d.
func TestGetMergeRequests_MissingMilestoneIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for missing milestone_iid")
	}
}

// TestGetMergeRequests_WithPagination verifies the behavior of get merge requests with pagination.
func TestGetMergeRequests_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":200,"iid":10,"title":"MR 1","state":"merged","source_branch":"feat","target_branch":"main","web_url":"https://example.com/mr/10","created_at":"2026-02-01T00:00:00Z"},{"id":201,"iid":11,"title":"MR 2","state":"opened","source_branch":"fix","target_branch":"main","created_at":"2026-02-02T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{
		GroupID:         "10",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("len(MergeRequests) = %d, want 2", len(out.MergeRequests))
	}
	if out.MergeRequests[0].CreatedAt == "" {
		t.Error("expected CreatedAt to be populated")
	}
	if out.MergeRequests[1].WebURL != "" {
		t.Error("expected WebURL to be empty when not set in response")
	}
}

// ---------------------------------------------------------------------------
// GetBurndownChartEvents — API error, canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestGetBurndownChartEvents_APIError verifies the behavior of get burndown chart events a p i error.
func TestGetBurndownChartEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetBurndownChartEvents_CancelledContext verifies the behavior of get burndown chart events cancelled context.
func TestGetBurndownChartEvents_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetBurndownChartEvents(ctx, client, GetBurndownChartEventsInput{GroupID: "10", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetBurndownChartEvents_MissingGroupID verifies the behavior of get burndown chart events missing group i d.
func TestGetBurndownChartEvents_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetBurndownChartEvents_WithPagination verifies the behavior of get burndown chart events with pagination.
func TestGetBurndownChartEvents_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathMilestone1+"/burndown_events" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"created_at":"2026-01-05T00:00:00Z","weight":3,"action":"add"},{"created_at":"2026-01-06T00:00:00Z","weight":2,"action":"remove"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{
		GroupID:         "10",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(out.Events))
	}
	if out.Events[0].Action != "add" {
		t.Errorf("Action = %q, want %q", out.Events[0].Action, "add")
	}
	if out.Events[1].Weight != 2 {
		t.Errorf("Weight = %d, want 2", out.Events[1].Weight)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — with data, empty/zero
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_WithAllFields verifies the behavior of format markdown with all fields.
func TestFormatMarkdown_WithAllFields(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 1, IID: 1, GroupID: 10, GroupPath: "my-org/backend", Title: "v1.0",
		Description: "Release milestone", State: "active",
		StartDate: testDateStart, DueDate: "2026-06-30",
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-15T00:00:00Z",
		Expired: true,
	})

	for _, want := range []string{
		"## Group Milestone: v1.0",
		"**ID**: 1 (IID: 1)",
		"**Group**: my-org/backend",
		"**State**: active",
		"**Description**: Release milestone",
		"**Start Date**: 1 Jan 2026",
		"**Due Date**: 30 Jun 2026",
		"**Expired**: true",
		"**Created**: 1 Jan 2026 00:00 UTC",
		"**Updated**: 15 Jan 2026 00:00 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtMarkdownMissing, want, md)
		}
	}
	if strings.Contains(md, "Group ID") {
		t.Errorf("should not contain legacy 'Group ID' label:\n%s", md)
	}
}

// TestFormatMarkdown_FallbackNumericGroupID verifies the behavior of format markdown fallback numeric group i d.
func TestFormatMarkdown_FallbackNumericGroupID(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 3, IID: 3, GroupID: 42, Title: "Fallback", State: "active",
	})
	if !strings.Contains(md, "**Group**: 42") {
		t.Errorf("expected fallback to numeric GroupID:\n%s", md)
	}
}

// TestFormatMarkdown_MinimalFields verifies the behavior of format markdown minimal fields.
func TestFormatMarkdown_MinimalFields(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 2, IID: 2, GroupID: 10, Title: "Bare", State: "closed",
	})
	if !strings.Contains(md, "## Group Milestone: Bare") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"**Description**",
		"**Start Date**",
		"**Due Date**",
		"**Created**",
		"**Updated**",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithMilestones verifies the behavior of format list markdown with milestones.
func TestFormatListMarkdown_WithMilestones(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{
			{ID: 1, IID: 1, Title: "v1.0", State: "active", StartDate: testDateStart, DueDate: "2026-06-30"},
			{ID: 2, IID: 2, Title: "v2.0", State: "closed", StartDate: "2026-07-01", DueDate: "2026-12-31"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdownString(out)

	for _, want := range []string{
		"## Group Milestones (2)",
		testTableHeaderID,
		"|----",
		"| 1 |",
		"| 2 |",
		"v1.0",
		"v2.0",
		"active",
		"closed",
	} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtMarkdownMissing, want, md)
		}
	}
}

// TestFormatListMarkdown_EmptyList verifies the behavior of format list markdown empty list.
func TestFormatListMarkdown_EmptyList(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{
		Pagination: toolutil.PaginationOutput{TotalItems: 0, Page: 1, PerPage: 20, TotalPages: 0},
	})
	if !strings.Contains(md, "No group milestones found") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
	if strings.Contains(md, testTableHeaderID) {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatIssuesMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatIssuesMarkdown_WithData verifies the behavior of format issues markdown with data.
func TestFormatIssuesMarkdown_WithData(t *testing.T) {
	out := IssuesOutput{
		Issues: []IssueItem{
			{ID: 100, IID: 5, Title: "Fix bug", State: "opened"},
			{ID: 101, IID: 6, Title: "Add feature", State: "closed"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatIssuesMarkdownString(out)

	for _, want := range []string{
		"## Milestone Issues (2)",
		testTableHeaderID,
		"| 100 |",
		"| 101 |",
		"Fix bug",
		"Add feature",
	} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtMarkdownMissing, want, md)
		}
	}
}

// TestFormatIssuesMarkdown_Empty verifies the behavior of format issues markdown empty.
func TestFormatIssuesMarkdown_Empty(t *testing.T) {
	md := FormatIssuesMarkdownString(IssuesOutput{
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	})
	if !strings.Contains(md, "No issues found for this milestone") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// FormatMergeRequestsMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatMergeRequestsMarkdown_WithData verifies the behavior of format merge requests markdown with data.
func TestFormatMergeRequestsMarkdown_WithData(t *testing.T) {
	out := MergeRequestsOutput{
		MergeRequests: []MergeRequestItem{
			{ID: 200, IID: 10, Title: "Feature MR", State: "merged", SourceBranch: "feat", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatMergeRequestsMarkdownString(out)

	for _, want := range []string{
		"## Milestone Merge Requests (1)",
		testTableHeaderID,
		"| 200 |",
		"Feature MR",
		"merged",
		"feat",
		"main",
	} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtMarkdownMissing, want, md)
		}
	}
}

// TestFormatMergeRequestsMarkdown_Empty verifies the behavior of format merge requests markdown empty.
func TestFormatMergeRequestsMarkdown_Empty(t *testing.T) {
	md := FormatMergeRequestsMarkdownString(MergeRequestsOutput{
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	})
	if !strings.Contains(md, "No merge requests found for this milestone") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// FormatBurndownChartEventsMarkdownString — with data, empty
// ---------------------------------------------------------------------------.

// TestFormatBurndownChartEventsMarkdown_WithData verifies the behavior of format burndown chart events markdown with data.
func TestFormatBurndownChartEventsMarkdown_WithData(t *testing.T) {
	out := BurndownChartEventsOutput{
		Events: []BurndownChartEventItem{
			{CreatedAt: "2026-01-05T00:00:00Z", Weight: 3, Action: "add"},
			{CreatedAt: "2026-01-06T00:00:00Z", Weight: 2, Action: "remove"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatBurndownChartEventsMarkdownString(out)

	for _, want := range []string{
		"## Burndown Chart Events (2)",
		"| Created At |",
		"5 Jan 2026 00:00 UTC",
		"| 3 |",
		"| 2 |",
		"add",
		"remove",
	} {
		if !strings.Contains(md, want) {
			t.Errorf(fmtMarkdownMissing, want, md)
		}
	}
}

// TestFormatBurndownChartEventsMarkdown_Empty verifies the behavior of format burndown chart events markdown empty.
func TestFormatBurndownChartEventsMarkdown_Empty(t *testing.T) {
	md := FormatBurndownChartEventsMarkdownString(BurndownChartEventsOutput{
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	})
	if !strings.Contains(md, "No burndown chart events found") {
		t.Errorf(fmtExpectedEmptyMsg, md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown (MCP result wrapper)
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_ReturnsCallToolResult verifies the behavior of format list markdown returns call tool result.
func TestFormatListMarkdown_ReturnsCallToolResult(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{{ID: 1, IID: 1, Title: "v1.0", State: "active"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil")
	}
}

// ---------------------------------------------------------------------------
// parseISODate edge case
// ---------------------------------------------------------------------------.

// TestParseISODate_Valid verifies the behavior of parse i s o date valid.
func TestParseISODate_Valid(t *testing.T) {
	d, err := parseISODate("2026-06-15")
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if d == nil {
		t.Fatal("expected non-nil ISOTime")
	}
}

// TestParseISODate_Invalid verifies the behavior of parse i s o date invalid.
func TestParseISODate_Invalid(t *testing.T) {
	_, err := parseISODate("June 15, 2026")
	if err == nil {
		t.Fatal("expected error for invalid date format")
	}
}

// TestResolveGroupIID_NotFound verifies error when resolveGroupIID returns empty results.
func TestResolveGroupIID_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, MilestoneIID: 999})
	if err == nil {
		t.Fatal("expected error for milestone IID not found, got nil")
	}
}

// TestGet_APIErrorAfterResolve verifies Get wraps API errors after successful IID resolution.
func TestGet_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestUpdate_BadStartDate verifies Update returns error for invalid start_date format.
func TestUpdate_BadStartDate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID: testGroupID, MilestoneIID: 1, StartDate: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid start_date, got nil")
	}
}

// TestUpdate_BadDueDate verifies Update returns error for invalid due_date format.
func TestUpdate_BadDueDate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID: testGroupID, MilestoneIID: 1, DueDate: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid due_date, got nil")
	}
}

// TestUpdate_APIErrorAfterResolve verifies Update wraps API errors when
// resolveGroupIID succeeds but the UpdateGroupMilestone API call fails.
func TestUpdate_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("PUT /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID: testGroupID, MilestoneIID: 1, Title: "new title",
	})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestDelete_APIErrorAfterResolve verifies Delete wraps API errors after successful resolve.
func TestDelete_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("DELETE /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := Delete(context.Background(), client, DeleteInput{GroupID: testGroupID, MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetIssues_APIErrorAfterResolve verifies GetIssues wraps API errors after resolve.
func TestGetIssues_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/10/milestones/1/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetIssues(context.Background(), client, GetIssuesInput{GroupID: testGroupID, MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetMergeRequests_APIErrorAfterResolve verifies GetMergeRequests wraps API errors after resolve.
func TestGetMergeRequests_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/10/milestones/1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{GroupID: testGroupID, MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetBurndownChartEvents_APIErrorAfterResolve verifies GetBurndownChartEvents wraps API errors after resolve.
func TestGetBurndownChartEvents_APIErrorAfterResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+milestoneJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/10/milestones/1/burndown_events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetBurndownChartEvents(context.Background(), client, GetBurndownChartEventsInput{GroupID: testGroupID, MilestoneIID: 1})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanicCoverage verifies the behavior of register tools no panic coverage.
func TestRegisterTools_NoPanicCoverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 8 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCPCoverage validates register tools call all through m c p coverage across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCPCoverage(t *testing.T) {
	session := newGroupMilestonesMCPSession(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_group_milestone_list", map[string]any{"group_id": "10"}},
		{"get", "gitlab_group_milestone_get", map[string]any{"group_id": "10", "milestone_iid": 1}},
		{"create", "gitlab_group_milestone_create", map[string]any{"group_id": "10", "title": "v2.0"}},
		{"update", "gitlab_group_milestone_update", map[string]any{"group_id": "10", "milestone_iid": 1, "title": "v1.0-updated"}},
		{"delete", "gitlab_group_milestone_delete", map[string]any{"group_id": "10", "milestone_iid": 1}},
		{"issues", "gitlab_group_milestone_issues", map[string]any{"group_id": "10", "milestone_iid": 1}},
		{"merge_requests", "gitlab_group_milestone_merge_requests", map[string]any{"group_id": "10", "milestone_iid": 1}},
		{"burndown_events", "gitlab_group_milestone_burndown_events", map[string]any{"group_id": "10", "milestone_iid": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			requireToolSuccess(t, session, tt.tool, tt.args)
		})
	}
}

// requireToolSuccess calls an MCP tool and fails the test if the call
// returns an error or an IsError result.
func requireToolSuccess(t *testing.T, session *mcp.ClientSession, toolName string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", toolName, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", toolName, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", toolName)
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newGroupMilestonesMCPSession is an internal helper for the groupmilestones package.
func newGroupMilestonesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	// List group milestones
	handler.HandleFunc("GET /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+milestoneJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	// Get group milestone
	handler.HandleFunc("GET /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, milestoneJSON)
	})

	// Create group milestone
	handler.HandleFunc("POST /api/v4/groups/10/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, milestoneJSON)
	})

	// Update group milestone
	handler.HandleFunc("PUT /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, milestoneJSON)
	})

	// Delete group milestone
	handler.HandleFunc("DELETE /api/v4/groups/10/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Get milestone issues
	handler.HandleFunc("GET /api/v4/groups/10/milestones/1/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":100,"iid":5,"title":"Fix bug","state":"opened","web_url":"https://example.com/issues/5","created_at":"2026-01-10T00:00:00Z"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	// Get milestone merge requests
	handler.HandleFunc("GET /api/v4/groups/10/milestones/1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":200,"iid":10,"title":"Feature MR","state":"merged","source_branch":"feature","target_branch":"main","web_url":"https://example.com/mr/10","created_at":"2026-02-01T00:00:00Z"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	// Get milestone burndown chart events
	handler.HandleFunc("GET /api/v4/groups/10/milestones/1/burndown_events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"created_at":"2026-01-05T00:00:00Z","weight":3,"action":"add"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	client := testutil.NewTestClient(t, handler)
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

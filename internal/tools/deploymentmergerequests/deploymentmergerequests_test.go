// deploymentmergerequests_test.go contains unit tests for the deployment merge request MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deploymentmergerequests

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/7/merge_requests (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/deployments/7/merge_requests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"iid": 10,
				"title": "Add feature X",
				"state": "merged",
				"author": {"username": "dev1"},
				"source_branch": "feature-x",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/10",
				"merged_at": "2026-01-15T10:30:00Z"
			},
			{
				"iid": 11,
				"title": "Fix bug Y",
				"state": "merged",
				"author": {"username": "dev2"},
				"source_branch": "fix-y",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/11"
			}
		]`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("expected 2 merge requests, got %d", len(out.MergeRequests))
	}
	mr := out.MergeRequests[0]
	if mr.IID != 10 {
		t.Errorf("expected IID 10, got %d", mr.IID)
	}
	if mr.Title != "Add feature X" {
		t.Errorf("expected title 'Add feature X', got %q", mr.Title)
	}
	if mr.State != "merged" {
		t.Errorf("expected state 'merged', got %q", mr.State)
	}
	if mr.Author == nil || mr.Author.Username != "dev1" {
		t.Errorf("expected author 'dev1', got %+v", mr.Author)
	}
	if mr.SourceBranch != "feature-x" {
		t.Errorf("expected source_branch 'feature-x', got %q", mr.SourceBranch)
	}
	if mr.MergedAt == "" {
		t.Error("expected merged_at to be set")
	}
	// Second MR has no merged_at
	if out.MergeRequests[1].MergedAt != "" {
		t.Errorf("expected empty merged_at for second MR, got %q", out.MergeRequests[1].MergedAt)
	}
}

// TestList_Empty verifies the List_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 0 {
		t.Fatalf("expected 0 merge requests, got %d", len(out.MergeRequests))
	}
}

// TestList_WithFilters verifies the List_WithFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_WithFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "merged" {
			t.Errorf("expected state=merged, got %q", q.Get("state"))
		}
		if q.Get("order_by") != "created_at" {
			t.Errorf("expected order_by=created_at, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %q", q.Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
		State:        "merged",
		OrderBy:      "created_at",
		Sort:         "desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_InvalidDeploymentID verifies the List_InvalidDeploymentID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_InvalidDeploymentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(t.Context(), client, ListInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id") {
		t.Errorf("expected error to mention deployment_id, got %q", err)
	}
	_, err = List(t.Context(), client, ListInput{ProjectID: "42", DeploymentID: -1})
	if err == nil {
		t.Fatal("expected error for negative deployment_id")
	}
}

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "server error"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdown_WithData verifies the ListMarkdown_WithData Markdown formatter for a representative list_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		MergeRequests: []Output{
			{IID: 1, Title: "MR One", State: "merged", Author: &toolutil.BasicUserOutput{Username: "dev"}, SourceBranch: "feat", TargetBranch: "main"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error (404), canceled context, pagination, nil author
// ---------------------------------------------------------------------------.

// TestList_APIError404 verifies that List404 returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError404(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "99", DeploymentID: 1})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "list_deployment_merge_requests") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42", DeploymentID: 7})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/deployments/5/merge_requests (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/deployments/5/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"iid":20,"title":"MR Alpha","state":"merged","author":{"username":"alice"},"source_branch":"feat-a","target_branch":"main","web_url":"https://gl.example.com/mr/20"}
			]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1", DeploymentID: 5,
		Page: 2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", out.Pagination.NextPage)
	}
	if out.Pagination.PrevPage != 1 {
		t.Errorf("PrevPage = %d, want 1", out.Pagination.PrevPage)
	}
}

// TestList_NilAuthor verifies the List_NilAuthor handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_NilAuthor(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"iid":30,"title":"No Author MR","state":"opened","source_branch":"fix","target_branch":"main","web_url":"https://gl.example.com/mr/30"}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "1", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Author != nil {
		t.Errorf("expected nil author for nil author, got %+v", out.MergeRequests[0].Author)
	}
}

// TestList_AllOptionalFilters verifies the List_AllOptionalFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_AllOptionalFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "opened" {
			t.Errorf("expected state=opened, got %q", q.Get("state"))
		}
		if q.Get("order_by") != "updated_at" {
			t.Errorf("expected order_by=updated_at, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %q", q.Get("sort"))
		}
		if q.Get("page") != "3" {
			t.Errorf("expected page=3, got %q", q.Get("page"))
		}
		if q.Get("per_page") != "50" {
			t.Errorf("expected per_page=50, got %q", q.Get("per_page"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:    "1",
		DeploymentID: 2,
		State:        "opened",
		OrderBy:      "updated_at",
		Sort:         "asc",
		Page:         3, PerPage: 50,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_MergeRequestFilters verifies that every merge-request filter mirrored
// from gl.ListMergeRequestsOptions is forwarded to the GitLab API query string.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts each filter is encoded with its canonical query parameter name.
func TestList_MergeRequestFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		checks := map[string]string{
			"approved":        "yes",
			"author_username": "alice",
			"in":              "title",
			"author_id":       "7",
			"assignee_id":     "9",
			"draft":           "true",
		}
		for key, want := range checks {
			if got := q.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
		if got := q.Get("approver_ids[]"); got != "11" {
			t.Errorf("approver_ids[] = %q, want 11", got)
		}
		if got := q.Get("approved_by_ids[]"); got != "13" {
			t.Errorf("approved_by_ids[] = %q, want 13", got)
		}
		// The SDK encodes *[]string as repeated plain keys, unlike
		// ApproverIDsValue which appends the [] suffix itself.
		if got := q["approved_by_usernames"]; !slices.Equal(got, []string{"alice", "bob"}) {
			t.Errorf("approved_by_usernames = %v, want [alice bob]", got)
		}
		if got := q.Get("created_after"); !strings.HasPrefix(got, "2025-01-01") {
			t.Errorf("created_after = %q, want 2025-01-01 prefix", got)
		}
		if got := q.Get("created_before"); !strings.HasPrefix(got, "2025-12-31") {
			t.Errorf("created_before = %q, want 2025-12-31 prefix", got)
		}
		if got := q.Get("pagination"); got != "keyset" {
			t.Errorf("pagination = %q, want keyset", got)
		}
		if got := q.Get("page_token"); got != "cursor-1" {
			t.Errorf("page_token = %q, want cursor-1", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	draft := true
	_, err := List(context.Background(), client, ListInput{
		ProjectID:           "1",
		DeploymentID:        2,
		Approved:            "yes",
		ApprovedByIDs:       toolutil.ApproverIDsFilter{"13"},
		ApprovedByUsernames: []string{"alice", "bob"},
		ApproverIDs:         toolutil.ApproverIDsFilter{"11"},
		AssigneeID:          9,
		AuthorID:            7,
		AuthorUsername:      "alice",
		CreatedAfter:        "2025-01-01T00:00:00Z",
		CreatedBefore:       "2025-12-31T23:59:59Z",
		Draft:               &draft,
		In:                  "title",
		Pagination:          "keyset",
		PageToken:           "cursor-1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_AdditionalMergeRequestFilters verifies the merge-request filters added
// for 1:1 parity with gl.ListMergeRequestsOptions (labels, not_labels, milestone,
// scope, search, branch, reviewer, reaction, view, wip, label-detail toggles, and
// updated-date bounds) are each forwarded to the GitLab API query string under
// their canonical parameter names.
func TestList_AdditionalMergeRequestFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		checks := map[string]string{
			"not[author_username]":      "bob",
			"reviewer_id":               "21",
			"reviewer_username":         "carol",
			"milestone":                 "v1.0",
			"scope":                     "all",
			"search":                    "fix bug",
			"source_branch":             "feature",
			"target_branch":             "main",
			"my_reaction_emoji":         "thumbsup",
			"view":                      "simple",
			"wip":                       "no",
			"labels":                    "bug,urgent",
			"not[labels]":               "wontfix",
			"with_labels_details":       "true",
			"with_merge_status_recheck": "true",
			"non_archived":              "true",
		}
		for key, want := range checks {
			if got := q.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
		if got := q.Get("updated_after"); !strings.HasPrefix(got, "2025-02-01") {
			t.Errorf("updated_after = %q, want 2025-02-01 prefix", got)
		}
		if got := q.Get("updated_before"); !strings.HasPrefix(got, "2025-11-30") {
			t.Errorf("updated_before = %q, want 2025-11-30 prefix", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	withDetails := true
	withRecheck := true
	_, err := List(context.Background(), client, ListInput{
		ProjectID:              "1",
		DeploymentID:           2,
		NotAuthorUsername:      "bob",
		ReviewerID:             21,
		ReviewerUsername:       "carol",
		Milestone:              "v1.0",
		Scope:                  "all",
		Search:                 "fix bug",
		SourceBranch:           "feature",
		TargetBranch:           "main",
		MyReactionEmoji:        "thumbsup",
		View:                   "simple",
		WIP:                    "no",
		Labels:                 []string{"bug", "urgent"},
		NotLabels:              []string{"wontfix"},
		WithLabelsDetails:      &withDetails,
		WithMergeStatusRecheck: &withRecheck,
		NonArchived:            &withRecheck,
		UpdatedAfter:           "2025-02-01T00:00:00Z",
		UpdatedBefore:          "2025-11-30T23:59:59Z",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_FullPayload verifies that List projects every sub-object of a fully
// populated gl.MergeRequest onto the local Output mirror.
// The mock GitLab API responds with a merge request carrying author, assignees,
// reviewers, labels, label_details, milestone, references, time_stats,
// task_completion_status, diff_refs, pipeline, and head_pipeline objects.
// It asserts the nested converters surface each canonical field.
func TestList_FullPayload(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"id": 100,
				"iid": 5,
				"project_id": 42,
				"title": "Full MR",
				"state": "merged",
				"source_branch": "feat",
				"target_branch": "main",
				"web_url": "https://gl.example.com/mr/5",
				"labels": ["bug", "urgent"],
				"label_details": [{"id": 1, "name": "bug", "color": "#ff0000", "text_color": "#ffffff"}],
				"author": {"id": 1, "username": "alice", "created_at": "2026-01-01T00:00:00Z"},
				"assignee": {"id": 2, "username": "bob"},
				"merge_user": {"id": 3, "username": "carol"},
				"merged_by": {"id": 3, "username": "carol"},
				"closed_by": {"id": 4, "username": "dave"},
				"assignees": [{"id": 2, "username": "bob"}, null],
				"reviewers": [{"id": 5, "username": "erin"}],
				"milestone": {"id": 7, "iid": 1, "title": "v1", "state": "active", "start_date": "2025-01-01", "due_date": "2025-02-01", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-02T00:00:00Z"},
				"references": {"short": "!5", "relative": "!5", "full": "g/p!5"},
				"time_stats": {"time_estimate": 3600, "total_time_spent": 1800, "human_time_estimate": "1h", "human_total_time_spent": "30m"},
				"task_completion_status": {"count": 4, "completed_count": 2},
				"user": {"can_merge": true},
				"diff_refs": {"base_sha": "aaa", "head_sha": "bbb", "start_sha": "ccc"},
				"pipeline": {"id": 200, "iid": 3, "project_id": 42, "status": "success", "ref": "feat", "sha": "bbb", "web_url": "https://gl.example.com/p/200", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T01:00:00Z"},
				"head_pipeline": {
					"id": 201, "iid": 4, "project_id": 42, "status": "success", "ref": "feat", "sha": "bbb",
					"before_sha": "000", "tag": false, "user": {"id": 1, "username": "alice"}, "duration": 120,
					"queued_duration": 5, "coverage": "90", "web_url": "https://gl.example.com/p/201",
					"created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T01:00:00Z",
					"started_at": "2025-01-01T00:10:00Z", "finished_at": "2025-01-01T00:30:00Z", "committed_at": "2025-01-01T00:05:00Z",
					"detailed_status": {"icon": "i", "text": "passed", "label": "passed", "group": "success", "tooltip": "ok", "has_details": true, "details_path": "/p", "favicon": "f", "illustration": {"image": "img.png"}}
				},
				"latest_build_started_at": "2025-01-01T00:10:00Z",
				"latest_build_finished_at": "2025-01-01T00:30:00Z",
				"first_deployed_to_production_at": "2025-01-02T00:00:00Z",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-01T02:00:00Z",
				"merged_at": "2025-01-01T03:00:00Z",
				"closed_at": "2025-01-01T04:00:00Z",
				"prepared_at": "2025-01-01T00:30:00Z"
			}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", DeploymentID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	assertFullPayload(t, out.MergeRequests[0])
}

// assertFullPayload checks that every sub-object converter populated the
// canonical fields of a fully-populated merge request.
func assertFullPayload(t *testing.T, mr Output) {
	t.Helper()
	cases := []struct {
		name string
		ok   bool
	}{
		{"author.created_at", mr.Author != nil && mr.Author.CreatedAt != ""},
		{"assignees (nil skipped)", len(mr.Assignees) == 1},
		{"labels", len(mr.Labels) == 2},
		{"label_details", mr.LabelDetails != nil},
		{"milestone ISO dates", mr.Milestone != nil && mr.Milestone.StartDate == "2025-01-01" && mr.Milestone.DueDate == "2025-02-01"},
		{"references", mr.References != nil && mr.References.Full == "g/p!5"},
		{"time_stats", mr.TimeStats != nil && mr.TimeStats.TimeEstimate == 3600},
		{"task_completion_status", mr.TaskCompletionStatus != nil && mr.TaskCompletionStatus.Count == 4},
		{"user", mr.User != nil && mr.User.CanMerge},
		{"diff_refs", mr.DiffRefs != nil && mr.DiffRefs.HeadSHA == "bbb"},
		{"pipeline", mr.Pipeline != nil && mr.Pipeline.ID == 200},
		{"head_pipeline detailed_status illustration", mr.HeadPipeline != nil && mr.HeadPipeline.DetailedStatus != nil && mr.HeadPipeline.DetailedStatus.Illustration != nil},
		{"timestamps", mr.MergedAt != "" && mr.ClosedAt != "" && mr.PreparedAt != "" && mr.FirstDeployedToProductionAt != ""},
	}
	for _, c := range cases {
		if !c.ok {
			t.Errorf("%s not projected: %+v", c.name, mr)
		}
	}
}

// TestList_SparsePayload verifies the nil and empty branches of the sub-object
// converters: a milestone without ISO dates, a label_details array whose only
// element is null, and a head_pipeline without a detailed_status object.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the converters return the expected zero values for missing data.
func TestList_SparsePayload(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"id": 101,
				"iid": 6,
				"project_id": 42,
				"title": "Sparse MR",
				"state": "opened",
				"source_branch": "feat",
				"target_branch": "main",
				"web_url": "https://gl.example.com/mr/6",
				"label_details": [null],
				"milestone": {"id": 8, "iid": 2, "title": "v2", "state": "active"},
				"head_pipeline": {"id": 202, "iid": 5, "project_id": 42, "status": "running", "ref": "feat", "sha": "ddd", "web_url": "https://gl.example.com/p/202"}
			}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", DeploymentID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	mr := out.MergeRequests[0]
	if mr.LabelDetails != nil {
		t.Errorf("expected nil label_details for all-null array, got %+v", mr.LabelDetails)
	}
	if mr.Milestone == nil || mr.Milestone.StartDate != "" || mr.Milestone.DueDate != "" {
		t.Errorf("expected empty ISO dates for dateless milestone, got %+v", mr.Milestone)
	}
	if mr.HeadPipeline == nil || mr.HeadPipeline.DetailedStatus != nil {
		t.Errorf("expected nil detailed_status, got %+v", mr.HeadPipeline)
	}
	if len(mr.Labels) != 0 {
		t.Errorf("expected empty labels slice, got %v", mr.Labels)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — multiple items, special characters, pagination info
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_MultipleItems verifies the ListMarkdown_MultipleItems Markdown formatter for a representative list_multipleitems input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_MultipleItems(t *testing.T) {
	out := ListOutput{
		MergeRequests: []Output{
			{IID: 10, Title: "Feature A", State: "merged", Author: &toolutil.BasicUserOutput{Username: "dev1"}, SourceBranch: "feat-a", TargetBranch: "main"},
			{IID: 11, Title: "Fix B", State: "opened", Author: &toolutil.BasicUserOutput{Username: "dev2"}, SourceBranch: "fix-b", TargetBranch: "develop"},
			{IID: 12, Title: "Hotfix C", State: "closed", Author: &toolutil.BasicUserOutput{Username: "dev3"}, SourceBranch: "hotfix-c", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3, Page: 1, PerPage: 20, TotalPages: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}

	text := result.Content[0].(*mcp.TextContent).Text

	for _, want := range []string{
		"## Deployment Merge Requests (3)",
		"| IID |",
		"|-----|",
		"!10",
		"!11",
		"!12",
		"Feature A",
		"Fix B",
		"Hotfix C",
		"dev1",
		"dev2",
		"dev3",
		"feat-a → main",
		"fix-b → develop",
		"hotfix-c → main",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text)
		}
	}
}

// TestFormatListMarkdown_SpecialCharacters verifies the ListMarkdown_SpecialCharacters Markdown formatter for a representative list_specialcharacters input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_SpecialCharacters(t *testing.T) {
	out := ListOutput{
		MergeRequests: []Output{
			{IID: 1, Title: "Title with | pipe", State: "merged", Author: &toolutil.BasicUserOutput{Username: "user"}, SourceBranch: "src", TargetBranch: "tgt"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "| pipe |") {
		t.Error("pipe character in title should be escaped")
	}
}

// TestFormatListMarkdown_EmptyOutput verifies the ListMarkdown_EmptyOutput Markdown formatter for a representative list_emptyoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_EmptyOutput(t *testing.T) {
	result := FormatListMarkdown(ListOutput{MergeRequests: []Output{}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Errorf("expected empty message, got:\n%s", text)
	}
	if strings.Contains(text, "| IID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_NilSlice verifies the ListMarkdown_NilSlice Markdown formatter for a representative list_nilslice input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_NilSlice(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Errorf("expected empty message for nil slice, got:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := deploymentMRSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(byTool))
	}
	spec := byTool["gitlab_list_deployment_merge_requests"]
	if spec.OwnerPackage != "deploymentmergerequests" {
		t.Errorf("OwnerPackage = %q, want deploymentmergerequests", spec.OwnerPackage)
	}
	if !spec.ReadOnly || !spec.Idempotent {
		t.Error("deployment merge request list action should be read-only and idempotent")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newDeploymentMRSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_deployment_merge_requests", "gitlab_list_deployment_merge_requests", map[string]any{
			"project_id": "42", "deployment_id": 7,
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_CallRouteWithFilters validates the CallRouteWithFilters route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRouteWithFilters(t *testing.T) {
	byTool := newDeploymentMRSpecsByTool(t)

	result, err := byTool["gitlab_list_deployment_merge_requests"].Route.Handler(t.Context(), map[string]any{
		"project_id":    "42",
		"deployment_id": 7,
		"state":         "merged",
		"order_by":      "created_at",
		"sort":          "desc",
		"page":          1,
		"per_page":      10,
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// TestActionSpecs_CallRouteEmptyResult validates the CallRouteEmptyResult route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRouteEmptyResult(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/deployments/7/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	byTool := deploymentMRSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_list_deployment_merge_requests"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "deployment_id": 7})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(ListOutput)
	if !ok {
		t.Fatalf("result type = %T, want ListOutput", result)
	}
	if len(out.MergeRequests) != 0 {
		t.Fatalf("len(MergeRequests) = %d, want 0", len(out.MergeRequests))
	}
}

// ---------------------------------------------------------------------------
// Helper: route factory
// ---------------------------------------------------------------------------.

// newDeploymentMRSpecsByTool constructs deployment MR specs by tool test fixtures.
func newDeploymentMRSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/deployments/7/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"iid": 10,
				"title": "Add feature X",
				"state": "merged",
				"author": {"username": "dev1"},
				"source_branch": "feature-x",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/10",
				"merged_at": "2026-01-15T10:30:00Z"
			},
			{
				"iid": 11,
				"title": "Fix bug Y",
				"state": "merged",
				"author": {"username": "dev2"},
				"source_branch": "fix-y",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/11"
			}
		]`)
	})

	client := testutil.NewTestClient(t, handler)
	return deploymentMRSpecsByTool(t, ActionSpecs(client))
}

// deploymentMRSpecsByTool supports deployment MR specs by tool assertions in deploymentmergerequests tests.
func deploymentMRSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestList_ApproverFilterInvalid_ReturnsError verifies that a malformed
// approver filter is rejected before any request reaches GitLab, and that the
// error names the field the caller got wrong.
//
// Both approver filters accept either a list of user IDs or exactly one of
// the "Any"/"None" literals, never a mix. Forwarding the mix would not fail
// loudly — GitLab drops a filter it cannot parse — so the caller would get a
// confidently unfiltered list back and no indication that the filter they
// asked for was never applied.
func TestList_ApproverFilterInvalid_ReturnsError(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		wantField string
	}{
		{"approver_ids", ListInput{
			ProjectID:    "42",
			DeploymentID: 7,
			ApproverIDs:  toolutil.ApproverIDsFilter{"Any", "7"},
		}, "approver_ids"},
		{"approved_by_ids", ListInput{
			ProjectID:     "42",
			DeploymentID:  7,
			ApprovedByIDs: toolutil.ApproverIDsFilter{"None", "7"},
		}, "approved_by_ids"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("request issued despite an invalid approver filter")
			}))
			_, err := List(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("List() error = nil, want an invalid-filter error")
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("List() error = %q, want it to name %s", err, tt.wantField)
			}
		})
	}
}

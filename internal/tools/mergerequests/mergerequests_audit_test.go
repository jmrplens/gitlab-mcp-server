// mergerequests_audit_test.go contains the 1:1 audit (slice A) tests that
// verify the additive merge request INPUT fields, keyset pagination, and the
// R-META discovery metadata introduced for the merge requests domain. The
// query-assertion tests confirm each newly wired input field reaches the
// outgoing GitLab API request, and the metadata test guards that every action
// carries action-specific Usage, natural-language aliases, canonical related
// actions, and a "Returns: … See also: …" individual-tool description.
package mergerequests

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// captureQuery is a tiny helper returning a handler that records the query
// values of the first matching request and replies with an empty paginated
// list. The captured url.Values are written through the provided pointer.
func captureQueryHandler(t *testing.T, method, path string, captured *url.Values) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method && r.URL.Path == path {
			*captured = r.URL.Query()
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}
}

// assertQuery fails the test when the captured query does not contain the
// expected key/value pair.
func assertQuery(t *testing.T, q url.Values, key, want string) {
	t.Helper()
	if got := q.Get(key); got != want {
		t.Errorf("query %q = %q, want %q (all=%v)", key, got, want, q)
	}
}

// TestList_NewFilterFields_ReachQuery verifies that every newly added project
// list filter (author/assignee/reviewer IDs, approvals, environment, reaction,
// view, wip, deployment dates, label-detail/recheck toggles, non_archived,
// not_author) is encoded onto the outgoing project merge requests request.
func TestList_NewFilterFields_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, pathMRs, &q))

	boolTrue := true
	_, err := List(context.Background(), client, ListInput{
		ProjectID:              testProjectID,
		NotAuthorUsername:      "mallory",
		ReviewerUsername:       "rita",
		Environment:            "production",
		MyReactionEmoji:        "thumbsup",
		View:                   "simple",
		WIP:                    "no",
		AuthorID:               7,
		AssigneeID:             8,
		ReviewerID:             9,
		ApproverIDs:            []int64{11, 12},
		ApprovedByIDs:          []int64{13},
		ApprovedByUsernames:    []string{"alice", "bob"},
		WithLabelsDetails:      &boolTrue,
		WithMergeStatusRecheck: &boolTrue,
		NonArchived:            &boolTrue,
		DeployedAfter:          "2025-01-01T00:00:00Z",
		DeployedBefore:         "2025-12-31T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	assertQuery(t, q, "not[author_username]", "mallory")
	assertQuery(t, q, "reviewer_username", "rita")
	assertQuery(t, q, "environment", "production")
	assertQuery(t, q, "my_reaction_emoji", "thumbsup")
	assertQuery(t, q, "view", "simple")
	assertQuery(t, q, "wip", "no")
	assertQuery(t, q, "author_id", "7")
	assertQuery(t, q, "assignee_id", "8")
	assertQuery(t, q, "reviewer_id", "9")
	assertQuery(t, q, "with_labels_details", "true")
	assertQuery(t, q, "with_merge_status_recheck", "true")
	assertQuery(t, q, "non_archived", "true")
	if got := q["approver_ids[]"]; !slices.Equal(got, []string{"11", "12"}) {
		t.Errorf("approver_ids[] = %v, want [11 12]", got)
	}
	if got := q["approved_by_ids[]"]; !slices.Equal(got, []string{"13"}) {
		t.Errorf("approved_by_ids[] = %v, want [13]", got)
	}
	// The SDK encodes *[]string as repeated plain keys, unlike ApproverIDsValue
	// which appends the [] suffix itself.
	if got := q["approved_by_usernames"]; !slices.Equal(got, []string{"alice", "bob"}) {
		t.Errorf("approved_by_usernames = %v, want [alice bob]", got)
	}
	if q.Get("deployed_after") == "" {
		t.Errorf("deployed_after missing (all=%v)", q)
	}
	if q.Get("deployed_before") == "" {
		t.Errorf("deployed_before missing (all=%v)", q)
	}
}

// TestList_Keyset_ReachQuery verifies keyset pagination parameters are wired
// onto the project list request.
func TestList_Keyset_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, pathMRs, &q))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:             testProjectID,
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "500"},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "500")
}

// TestListGlobal_NewFilterFields_ReachQuery verifies the new global list
// filters (approved plus author/assignee/reviewer IDs, in, view, wip, reaction,
// not_author, label toggles, non_archived) and keyset reach the global request.
func TestListGlobal_NewFilterFields_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, "/api/v4/merge_requests", &q))

	boolTrue := true
	_, err := ListGlobal(context.Background(), client, ListGlobalInput{
		Approved:               "yes",
		NotAuthorUsername:      "mallory",
		In:                     "title,description",
		MyReactionEmoji:        "rocket",
		View:                   "simple",
		WIP:                    "yes",
		AuthorID:               1,
		AssigneeID:             2,
		ReviewerID:             3,
		ApproverIDs:            []int64{4},
		ApprovedByIDs:          []int64{5, 6},
		WithLabelsDetails:      &boolTrue,
		WithMergeStatusRecheck: &boolTrue,
		NonArchived:            &boolTrue,
		KeysetPaginationInput:  toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "900"},
	})
	if err != nil {
		t.Fatalf("ListGlobal() unexpected error: %v", err)
	}
	assertQuery(t, q, "approved", "yes")
	assertQuery(t, q, "not[author_username]", "mallory")
	assertQuery(t, q, "in", "title,description")
	assertQuery(t, q, "my_reaction_emoji", "rocket")
	assertQuery(t, q, "view", "simple")
	assertQuery(t, q, "wip", "yes")
	assertQuery(t, q, "author_id", "1")
	assertQuery(t, q, "assignee_id", "2")
	assertQuery(t, q, "reviewer_id", "3")
	assertQuery(t, q, "with_labels_details", "true")
	assertQuery(t, q, "with_merge_status_recheck", "true")
	assertQuery(t, q, "non_archived", "true")
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "900")
	if got := q["approver_ids[]"]; !slices.Equal(got, []string{"4"}) {
		t.Errorf("approver_ids[] = %v, want [4]", got)
	}
	if got := q["approved_by_ids[]"]; !slices.Equal(got, []string{"5", "6"}) {
		t.Errorf("approved_by_ids[] = %v, want [5 6]", got)
	}
}

// TestListGroup_NewFilterFields_ReachQuery verifies the new group list filters
// and keyset reach the group request (group listing has no approved or
// environment/deployment filters).
func TestListGroup_NewFilterFields_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, "/api/v4/groups/99/merge_requests", &q))

	boolTrue := true
	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:                "99",
		NotAuthorUsername:      "mallory",
		In:                     "title",
		MyReactionEmoji:        "eyes",
		View:                   "simple",
		WIP:                    "no",
		AuthorID:               10,
		AssigneeID:             20,
		ReviewerID:             30,
		ApproverIDs:            []int64{40},
		ApprovedByIDs:          []int64{50},
		WithLabelsDetails:      &boolTrue,
		WithMergeStatusRecheck: &boolTrue,
		NonArchived:            &boolTrue,
		KeysetPaginationInput:  toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "123"},
	})
	if err != nil {
		t.Fatalf("ListGroup() unexpected error: %v", err)
	}
	assertQuery(t, q, "not[author_username]", "mallory")
	assertQuery(t, q, "in", "title")
	assertQuery(t, q, "my_reaction_emoji", "eyes")
	assertQuery(t, q, "view", "simple")
	assertQuery(t, q, "wip", "no")
	assertQuery(t, q, "author_id", "10")
	assertQuery(t, q, "assignee_id", "20")
	assertQuery(t, q, "reviewer_id", "30")
	assertQuery(t, q, "with_labels_details", "true")
	assertQuery(t, q, "with_merge_status_recheck", "true")
	assertQuery(t, q, "non_archived", "true")
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "123")
	if got := q["approver_ids[]"]; !slices.Equal(got, []string{"40"}) {
		t.Errorf("approver_ids[] = %v, want [40]", got)
	}
	if got := q["approved_by_ids[]"]; !slices.Equal(got, []string{"50"}) {
		t.Errorf("approved_by_ids[] = %v, want [50]", got)
	}
}

// TestCommits_OrderSortKeyset_ReachQuery verifies order_by/sort/keyset reach
// the MR commits list request.
func TestCommits_OrderSortKeyset_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, pathMR1+"/commits", &q))

	_, err := Commits(context.Background(), client, CommitsInput{
		ProjectID:             testProjectID,
		MRIID:                 1,
		OrderBy:               "created_at",
		Sort:                  "asc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "77"},
	})
	if err != nil {
		t.Fatalf("Commits() unexpected error: %v", err)
	}
	assertQuery(t, q, "order_by", "created_at")
	assertQuery(t, q, "sort", "asc")
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "77")
}

// TestIssuesClosed_OrderSortKeyset_ReachQuery verifies order_by/sort/keyset
// reach the issues-closed-on-merge request.
func TestIssuesClosed_OrderSortKeyset_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, pathMR1+"/closes_issues", &q))

	_, err := IssuesClosed(context.Background(), client, IssuesClosedInput{
		ProjectID:             testProjectID,
		MRIID:                 1,
		OrderBy:               "updated_at",
		Sort:                  "desc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "88"},
	})
	if err != nil {
		t.Fatalf("IssuesClosed() unexpected error: %v", err)
	}
	assertQuery(t, q, "order_by", "updated_at")
	assertQuery(t, q, "sort", "desc")
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "88")
}

// TestRelatedIssues_OrderSortKeyset_ReachQuery verifies order_by/sort/keyset
// reach the related-issues request.
func TestRelatedIssues_OrderSortKeyset_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, captureQueryHandler(t, http.MethodGet, pathMR1+"/related_issues", &q))

	_, err := RelatedIssues(context.Background(), client, RelatedIssuesInput{
		ProjectID:             testProjectID,
		MRIID:                 1,
		OrderBy:               "created_at",
		Sort:                  "desc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "99"},
	})
	if err != nil {
		t.Fatalf("RelatedIssues() unexpected error: %v", err)
	}
	assertQuery(t, q, "order_by", "created_at")
	assertQuery(t, q, "sort", "desc")
	assertQuery(t, q, "pagination", "keyset")
	assertQuery(t, q, "page_token", "99")
}

// TestGet_OptionFields_ReachQuery verifies include_diverged_commits_count,
// include_rebase_in_progress, and render_html reach the MR get request.
func TestGet_OptionFields_ReachQuery(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == pathMR1 {
				q = r.URL.Query()
				testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
				return
			}
			http.NotFound(w, r)
		}
	}())

	boolTrue := true
	_, err := Get(context.Background(), client, GetInput{
		ProjectID:                   testProjectID,
		MRIID:                       1,
		IncludeDivergedCommitsCount: &boolTrue,
		IncludeRebaseInProgress:     &boolTrue,
		RenderHTML:                  &boolTrue,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertQuery(t, q, "include_diverged_commits_count", "true")
	assertQuery(t, q, "include_rebase_in_progress", "true")
	assertQuery(t, q, "render_html", "true")
}

// TestApprove_SHA_ReachRequest verifies the sha safety check reaches the
// approve request body.
func TestApprove_SHA_ReachRequest(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == pathMR1+"/approve" {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				testutil.RespondJSON(w, http.StatusOK, `{"approvals_required":2,"approved":false,"approved_by":[]}`)
				return
			}
			http.NotFound(w, r)
		}
	}())

	_, err := Approve(context.Background(), client, ApproveInput{
		ProjectID: testProjectID,
		MRIID:     1,
		SHA:       testSHAAbc,
	})
	if err != nil {
		t.Fatalf("Approve() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"sha":"`+testSHAAbc+`"`) {
		t.Errorf("approve body missing sha: %s", body)
	}
}

// TestMerge_MergeWhenPipelineSucceeds_ReachRequest verifies the deprecated
// merge_when_pipeline_succeeds field reaches the merge request body.
func TestMerge_MergeWhenPipelineSucceeds_ReachRequest(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && r.URL.Path == pathMR1+"/merge" {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				testutil.RespondJSON(w, http.StatusOK, mrJSONCoverage)
				return
			}
			http.NotFound(w, r)
		}
	}())

	boolTrue := true
	_, err := Merge(context.Background(), client, MergeInput{
		ProjectID:                 testProjectID,
		MRIID:                     1,
		MergeWhenPipelineSucceeds: &boolTrue,
	})
	if err != nil {
		t.Fatalf("Merge() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"merge_when_pipeline_succeeds":true`) {
		t.Errorf("merge body missing merge_when_pipeline_succeeds: %s", body)
	}
}

// TestCreate_ApprovalsBeforeMerge_ReachRequest verifies approvals_before_merge
// reaches the create request body.
func TestCreate_ApprovalsBeforeMerge_ReachRequest(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == pathMRs {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				testutil.RespondJSON(w, http.StatusCreated, mrJSONCoverage)
				return
			}
			http.NotFound(w, r)
		}
	}())

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:            testProjectID,
		SourceBranch:         testBranchFeat,
		TargetBranch:         testBranchMain,
		Title:                "MR with approvals",
		ApprovalsBeforeMerge: 3,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"approvals_before_merge":3`) {
		t.Errorf("create body missing approvals_before_merge: %s", body)
	}
}

// mrMetaActions enumerates the 29 R-META-flagged merge request tools that must
// each carry action-specific Usage, natural-language aliases, canonical
// related actions, and a "Returns: … See also: …" individual-tool description.
var mrMetaActions = []string{
	"gitlab_mr_approve", "gitlab_mr_cancel_auto_merge", "gitlab_mr_commits", "gitlab_mr_create",
	"gitlab_mr_create_pipeline", "gitlab_mr_create_todo", "gitlab_mr_delete", "gitlab_mr_dependencies_list",
	"gitlab_mr_dependency_create", "gitlab_mr_dependency_delete", "gitlab_mr_get", "gitlab_mr_issues_closed",
	"gitlab_mr_list", "gitlab_mr_list_global", "gitlab_mr_list_group", "gitlab_mr_participants",
	"gitlab_mr_pipelines", "gitlab_mr_rebase", "gitlab_mr_related_issues", "gitlab_mr_reviewers",
	"gitlab_mr_add_spent_time", "gitlab_mr_reset_spent_time", "gitlab_mr_subscribe",
	"gitlab_mr_reset_time_estimate", "gitlab_mr_set_time_estimate", "gitlab_mr_time_stats",
	"gitlab_mr_unapprove", "gitlab_mr_unsubscribe", "gitlab_mr_update",
}

// TestActionSpecs_Metadata_NoGenericPlaceholders guards that every flagged
// merge request action exposes non-generic discovery metadata: a purpose
// Usage sentence, at least one natural-language alias beyond the tool name,
// non-empty related actions, and a "Returns: … See also: …" description.
func TestActionSpecs_Metadata_NoGenericPlaceholders(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := mergeRequestSpecsByTool(t, ActionSpecs(client))

	for _, tool := range mrMetaActions {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("missing action spec for %s", tool)
		}
		if u := strings.ToLower(strings.TrimSpace(spec.Usage)); u == "" || strings.HasPrefix(u, "use to execute") {
			t.Errorf("%s: generic or empty Usage = %q", tool, spec.Usage)
		}
		if !hasNaturalLanguageAlias(spec, tool) {
			t.Errorf("%s: aliases lack a natural-language entry: %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: RelatedActions is empty", tool)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: individual description lacks Returns/See also: %q", tool, desc)
		}
	}
}

// hasNaturalLanguageAlias reports whether the spec has at least one alias that
// is neither the canonical action name nor the individual tool name, matching
// the R-META aliases_only_toolname detector.
func hasNaturalLanguageAlias(spec toolutil.ActionSpec, tool string) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	toolLower := strings.ToLower(strings.TrimSpace(tool))
	for _, alias := range spec.Aliases {
		a := strings.ToLower(strings.TrimSpace(alias))
		if a == "" || a == canonical || a == toolLower {
			continue
		}
		return true
	}
	return false
}

// prompt_error_paths_test.go contains unit tests for the error and edge
// branches of the prompt handlers: GitLab API failures, missing arguments,
// empty results, and warn-and-continue paths that the happy-path tests in
// the sibling files do not exercise.
package prompts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	// notFoundJSON is the canonical GitLab 404 error body used by the
	// error-branch tests. 404 is used instead of 500 because client-go
	// retries 5xx responses, which would slow the tests down.
	notFoundJSON = `{"message":"404 Not Found"}`
	// basicUserJSON is the current-user payload for resolveUser success paths.
	basicUserJSON = `{"id":1,"username":"me"}`
	// aliceLookupJSON is the /users lookup payload resolving username "alice".
	aliceLookupJSON = `[{"id":42,"username":"alice"}]`
	// groupProjMRJSON is a minimal BasicMergeRequest list with a project reference.
	groupProjMRJSON = `[{"iid":1,"project_id":10,"title":"MR1","source_branch":"a","target_branch":"main","references":{"full":"group/proj!1"},"created_at":"2026-01-01T00:00:00Z"}]`
)

// respondNotFound writes the canonical GitLab 404 JSON error response.
func respondNotFound(w http.ResponseWriter) {
	respondJSON(w, http.StatusNotFound, notFoundJSON)
}

// notFoundHandler returns a handler that answers every request with 404,
// used to drive the first GitLab API call of a handler into its error branch.
func notFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
}

// getPromptExpectError issues a GetPrompt call that is expected to fail and
// fails the test when the prompt handler does not surface an error.
func getPromptExpectError(t *testing.T, handler http.Handler, name string, args map[string]string) {
	t.Helper()
	session := newMCPSession(t, handler)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err == nil {
		t.Errorf("expected error from prompt %q", name)
	}
}

// getPromptText issues a GetPrompt call that is expected to succeed and
// returns the text of the first message.
func getPromptText(t *testing.T, handler http.Handler, name string, args map[string]string) string {
	t.Helper()
	session := newMCPSession(t, handler)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	return result.Messages[0].Content.(*mcp.TextContent).Text
}

// prompts.go error branches.

// TestFetchContributionEvents_APIError verifies that fetchContributionEvents
// logs and returns an empty slice (instead of failing the prompt) when the
// events API fails, for both the current-user and other-user endpoints.
func TestFetchContributionEvents_APIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	since := time.Now().Add(-7 * 24 * time.Hour)

	if events := fetchContributionEvents(t.Context(), client, 42, false, since); len(events) != 0 {
		t.Errorf("expected no events for other-user API error, got %d", len(events))
	}
	if events := fetchContributionEvents(t.Context(), client, 1, true, since); len(events) != 0 {
		t.Errorf("expected no events for current-user API error, got %d", len(events))
	}
}

// TestWriteOpenMRsSection_APIError verifies that the open-MRs section of the
// project health check is silently skipped when the MR list API fails.
func TestWriteOpenMRsSection_APIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	var b strings.Builder
	writeOpenMRsSection(t.Context(), &b, client, "42")
	if b.Len() != 0 {
		t.Errorf("expected empty section on API error, got: %q", b.String())
	}
}

// TestWriteBranchesSection_APIError verifies that the branches section of the
// project health check is silently skipped when the branch list API fails.
func TestWriteBranchesSection_APIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	var b strings.Builder
	writeBranchesSection(t.Context(), &b, client, "42")
	if b.Len() != 0 {
		t.Errorf("expected empty section on API error, got: %q", b.String())
	}
}

// TestWriteMRSection_FetchError verifies that writeMRSection skips the whole
// section (warn-and-continue) when the MR fetch that produced the slice failed.
func TestWriteMRSection_FetchError(t *testing.T) {
	var b strings.Builder
	writeMRSection(&b, "Open MRs by", "alice", nil, errors.New("boom"))
	if b.Len() != 0 {
		t.Errorf("expected no output for failed fetch, got: %q", b.String())
	}
}

// TestTeamMemberWorkload_MissingProjectID verifies that the
// team_member_workload prompt rejects requests without a project_id.
func TestTeamMemberWorkload_MissingProjectID(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "team_member_workload", nil)
}

// prompt_team.go error branches.

// TestUserActivityReport_UserLookupAPIError verifies that the
// user_activity_report prompt returns an error when the /users lookup fails
// (covers both the handler branch and resolveUser's lookup-error branch).
func TestUserActivityReport_UserLookupAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "user_activity_report", map[string]string{"username": "alice"})
}

// TestUserActivityReport_NoEventsWithMRs verifies the user_activity_report
// branches for an empty event list combined with non-empty merged and
// under-review MR lists (the grouped per-project MR tables).
func TestUserActivityReport_NoEventsWithMRs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, aliceLookupJSON)
	})
	mux.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, groupProjMRJSON)
	})

	text := getPromptText(t, mux, "user_activity_report", map[string]string{"username": "alice"})
	if !strings.Contains(text, "No contribution events found") {
		t.Error("expected empty-events message")
	}
	if !strings.Contains(text, "### group/proj") {
		t.Error("expected merged MRs grouped by project")
	}
	if !strings.Contains(text, "## MRs Under Review") {
		t.Error("expected MRs under review section")
	}
}

// TestUserActivityReport_MultiDayChart verifies the daily activity chart
// separator branches when contribution events span more than one day.
func TestUserActivityReport_MultiDayChart(t *testing.T) {
	d1 := time.Now().Add(-48 * time.Hour)
	d2 := time.Now().Add(-24 * time.Hour)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, aliceLookupJSON)
	})
	mux.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		events := []*gl.ContributionEvent{
			{ActionName: "pushed to", CreatedAt: &d1},
			{ActionName: "opened", CreatedAt: &d2},
		}
		data, _ := json.Marshal(events)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "user_activity_report", map[string]string{"username": "alice"})
	if !strings.Contains(text, "## Daily Activity") {
		t.Error("expected daily activity chart")
	}
	if !strings.Contains(text, "bar [1, 1]") {
		t.Error("expected two comma-separated bar values")
	}
}

// TestTeamOverview_MembersAPIError verifies that team_overview returns an
// error when the group members API fails.
func TestTeamOverview_MembersAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "team_overview", map[string]string{"group_id": "g1"})
}

// TestBuildTeamOverviewStats_MergedMRAuthors verifies the merged-MR
// attribution branches: authors that are active members are counted while
// unknown or nil authors are ignored.
func TestBuildTeamOverviewStats_MergedMRAuthors(t *testing.T) {
	members := []*gl.GroupMember{{Username: "alice", Name: "Alice", State: "active"}}
	mergedMRs := []*gl.BasicMergeRequest{
		{Author: &gl.BasicUser{Username: "alice"}},
		{Author: &gl.BasicUser{Username: "ghost"}},
		{},
	}

	stats := buildTeamOverviewStats(members, nil, mergedMRs)

	if stats["alice"].mergedMRs != 1 {
		t.Errorf("alice mergedMRs = %d, want 1", stats["alice"].mergedMRs)
	}
	if _, ok := stats["ghost"]; ok {
		t.Error("non-member author should not create a stats entry")
	}
}

// TestWriteTeamOverviewChart_EmptyStats verifies the chart is omitted when
// there are no member stats to plot.
func TestWriteTeamOverviewChart_EmptyStats(t *testing.T) {
	var b strings.Builder
	writeTeamOverviewChart(&b, map[string]*teamOverviewMemberStats{})
	if b.Len() != 0 {
		t.Errorf("expected no chart for empty stats, got: %q", b.String())
	}
}

// TestGroupMRDashboard_MissingGroupID verifies that group_mr_dashboard
// rejects requests without a group_id.
func TestGroupMRDashboard_MissingGroupID(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "group_mr_dashboard", nil)
}

// TestGroupMRDashboard_APIError verifies that group_mr_dashboard returns an
// error when the group MR list API fails.
func TestGroupMRDashboard_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "group_mr_dashboard", map[string]string{"group_id": "g1"})
}

// TestGroupMRDashboard_ConflictCount verifies the conflict-counting branch
// when an MR in the dashboard has merge conflicts.
func TestGroupMRDashboard_ConflictCount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/g1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":10,"title":"MR1","source_branch":"a","target_branch":"main","has_conflicts":true,"references":{"full":"group/proj!1"},"created_at":"2026-01-01T00:00:00Z"}]`)
	})

	text := getPromptText(t, mux, "group_mr_dashboard", map[string]string{"group_id": "g1"})
	if !strings.Contains(text, "| With conflicts | 1 |") {
		t.Error("expected conflict count of 1")
	}
}

// TestReviewerWorkload_MembersAPIError verifies that reviewer_workload
// returns an error when the group members API fails.
func TestReviewerWorkload_MembersAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "reviewer_workload", map[string]string{"group_id": "g1"})
}

// TestRecordReviewerWorkloadMR_UnknownReviewer verifies that a reviewer who
// is not a group member gets a stats entry created on the fly.
func TestRecordReviewerWorkloadMR_UnknownReviewer(t *testing.T) {
	stats := map[string]*reviewerWorkloadStats{}
	created := time.Now().Add(-24 * time.Hour)

	recordReviewerWorkloadMR(stats, "ghost", &created)

	stat, ok := stats["ghost"]
	if !ok {
		t.Fatal("expected stats entry for unknown reviewer")
	}
	if stat.count != 1 || stat.name != "ghost" {
		t.Errorf("stat = {count:%d name:%q}, want {count:1 name:\"ghost\"}", stat.count, stat.name)
	}
	if stat.oldestMR == nil || !stat.oldestMR.Equal(created) {
		t.Errorf("oldestMR = %v, want %v", stat.oldestMR, created)
	}
}

// TestWriteReviewerWorkloadChart_NoActiveReviewers verifies the chart is
// omitted when nobody is reviewing anything.
func TestWriteReviewerWorkloadChart_NoActiveReviewers(t *testing.T) {
	var b strings.Builder
	writeReviewerWorkloadChart(&b, map[string]*reviewerWorkloadStats{"a": {name: "A"}}, 0)
	if b.Len() != 0 {
		t.Errorf("expected no chart with zero active reviewers, got: %q", b.String())
	}
}

// prompt_audit.go error branches.

// TestAuditBranchProtection_ProjectAPIError verifies the project-fetch error
// branch of the branch protection audit.
func TestAuditBranchProtection_ProjectAPIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditBranchProtection_BranchesAPIError verifies the protected-branches
// error branch of the branch protection audit (project fetch succeeds).
func TestAuditBranchProtection_BranchesAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p","default_branch":"main"}`)
	})
	mux.HandleFunc(routeProtectedBranches, func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	_, err := handleAuditBranchProtection(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when protected branches API fails")
	}
}

// TestWriteSharedGroups_WithExpiry verifies the expiry-date branch of the
// shared-groups table.
func TestWriteSharedGroups_WithExpiry(t *testing.T) {
	expires := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	var b strings.Builder
	writeSharedGroups(&b, []gl.ProjectSharedWithGroup{
		{GroupID: 7, GroupName: "team-x", GroupAccessLevel: 30, ExpiresAt: &expires},
	})
	if !strings.Contains(b.String(), "2026-01-01") {
		t.Errorf("expected expiry date in output, got: %s", b.String())
	}
}

// TestAuditProjectAccess_MembersAPIError verifies the members-fetch error
// branch of the project access audit.
func TestAuditProjectAccess_MembersAPIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when members API fails")
	}
}

// TestAuditProjectAccess_ProjectAPIError verifies the project-fetch error
// branch of the project access audit (members fetch succeeds).
func TestAuditProjectAccess_ProjectAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	_, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditProjectAccess_InactiveAccounts verifies the inactive-accounts
// section branch when a member is neither active nor blocked.
func TestAuditProjectAccess_InactiveAccounts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeMembersAll, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":1,"username":"u1","name":"U One","access_level":30,"state":"awaiting"}]`)
	})
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p"}`)
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectAccess(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Inactive Accounts") {
		t.Error("expected inactive accounts section")
	}
}

// TestAuditProjectWorkflow_ProjectAPIError verifies the project-fetch error
// branch of the workflow audit.
func TestAuditProjectWorkflow_ProjectAPIError(t *testing.T) {
	client := newTestClient(t, notFoundHandler())
	_, err := handleAuditProjectWorkflow(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err == nil {
		t.Error("expected error when project API fails")
	}
}

// TestAuditProjectWorkflow_SubResourceAPIErrors verifies that the workflow
// audit degrades gracefully (warn-and-continue) when the labels and template
// APIs fail while the project fetch succeeds.
func TestAuditProjectWorkflow_SubResourceAPIErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProject, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"path_with_namespace":"group/p"}`)
	})
	mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	// Labels and both template endpoints fall through to 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	client := newTestClient(t, mux)
	result, err := handleAuditProjectWorkflow(t.Context(), client, testPromptRequest(map[string]string{"project_id": "42"}))
	if err != nil {
		t.Fatalf(errMsgUnexpected, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No labels configured") {
		t.Error("expected no-labels warning when labels API fails")
	}
	if !strings.Contains(text, "No templates found") {
		t.Error("expected no-templates warning when template APIs fail")
	}
}

// prompt_cross_project.go error branches.

// TestMyOpenMRs_UserAPIError verifies that my_open_mrs returns an error when
// the current-user lookup fails.
func TestMyOpenMRs_UserAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_open_mrs", nil)
}

// TestMyOpenMRs_MRListAPIErrors verifies that my_open_mrs degrades to an
// empty dashboard (warn-and-continue) when both MR list calls fail.
func TestMyOpenMRs_MRListAPIErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	text := getPromptText(t, mux, "my_open_mrs", nil)
	if !strings.Contains(text, "| Total open MRs | 0 |") {
		t.Error("expected zero MRs when both list calls fail")
	}
}

// TestMyPendingReviews_UserAPIError verifies that my_pending_reviews returns
// an error when the current-user lookup fails.
func TestMyPendingReviews_UserAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_pending_reviews", nil)
}

// TestMyPendingReviews_ListAPIError verifies that my_pending_reviews returns
// an error when the reviewer MR list fails.
func TestMyPendingReviews_ListAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
	getPromptExpectError(t, mux, "my_pending_reviews", nil)
}

// TestMyIssues_UserAPIError verifies that my_issues returns an error when the
// current-user lookup fails.
func TestMyIssues_UserAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_issues", nil)
}

// TestMyIssues_ListAPIError verifies that my_issues returns an error when the
// issue list fails.
func TestMyIssues_ListAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
	getPromptExpectError(t, mux, "my_issues", nil)
}

// TestMyActivitySummary_UserAPIError verifies that my_activity_summary
// returns an error when the current-user lookup fails.
func TestMyActivitySummary_UserAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_activity_summary", nil)
}

// TestMyActivitySummary_MRListAPIErrors verifies that my_activity_summary
// renders N/A rows (warn-and-continue) when the merged and reviewed MR list
// calls fail.
func TestMyActivitySummary_MRListAPIErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("GET /api/v4/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	text := getPromptText(t, mux, "my_activity_summary", nil)
	if !strings.Contains(text, "| MRs merged | N/A |") {
		t.Error("expected N/A for merged MRs when list fails")
	}
	if !strings.Contains(text, "| MRs reviewed | N/A |") {
		t.Error("expected N/A for reviewed MRs when list fails")
	}
}

// prompt_analytics.go error branches.

// TestMergeVelocity_APIError verifies that merge_velocity returns an error
// when the MR list API fails.
func TestMergeVelocity_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "merge_velocity", map[string]string{"project_id": "42"})
}

// TestWriteDailyMergeChart_NoMergedDates verifies that the daily merge chart
// is omitted when no MR carries a merged_at timestamp.
func TestWriteDailyMergeChart_NoMergedDates(t *testing.T) {
	var b strings.Builder
	writeDailyMergeChart(&b, []*gl.BasicMergeRequest{{IID: 1}})
	if b.Len() != 0 {
		t.Errorf("expected no chart without merge dates, got: %q", b.String())
	}
}

// TestReleaseReadiness_APIError verifies that release_readiness returns an
// error when the MR list API fails.
func TestReleaseReadiness_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "release_readiness", map[string]string{"project_id": "42"})
}

// TestReleaseReadiness_DiscussionsAPIError verifies that unresolved-thread
// counting skips MRs whose discussion API fails (continue branch).
func TestReleaseReadiness_DiscussionsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":42,"title":"MR1","source_branch":"a","target_branch":"main"}]`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	text := getPromptText(t, mux, "release_readiness", map[string]string{"project_id": "42"})
	if !strings.Contains(text, "| Unresolved threads | 0 |") {
		t.Error("expected zero unresolved threads when discussions API fails")
	}
}

// TestReleaseCadence_APIError verifies that release_cadence returns an error
// when the releases API fails.
func TestReleaseCadence_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "release_cadence", map[string]string{"project_id": "42"})
}

// TestFilterRecentReleases_CreatedAtFallback verifies that a release without
// released_at falls back to created_at for the recency filter.
func TestFilterRecentReleases_CreatedAtFallback(t *testing.T) {
	created := time.Now().Add(-24 * time.Hour)
	since := time.Now().Add(-30 * 24 * time.Hour)

	filtered := filterRecentReleases([]*gl.Release{{CreatedAt: &created}}, since)
	if len(filtered) != 1 {
		t.Errorf("expected 1 release via created_at fallback, got %d", len(filtered))
	}
}

// TestWriteReleaseHistoryTable_TagNameFallback verifies that a release
// without a name is rendered using its tag name. The slice is built
// dynamically so gosec does not bounds-propagate a literal length into the
// production loop (G602 false positive).
func TestWriteReleaseHistoryTable_TagNameFallback(t *testing.T) {
	now := time.Now()
	var releases []*gl.Release
	releases = append(releases, &gl.Release{TagName: "v1.0.0", ReleasedAt: &now})

	var b strings.Builder
	writeReleaseHistoryTable(&b, releases)
	if !strings.Contains(b.String(), "| v1.0.0 | v1.0.0 |") {
		t.Errorf("expected tag name fallback in table, got: %s", b.String())
	}
}

// TestWeeklyTeamRecap_MergedAPIError verifies that weekly_team_recap degrades
// to a zero-count recap (warn-and-continue) when the group MR API fails.
func TestWeeklyTeamRecap_MergedAPIError(t *testing.T) {
	text := getPromptText(t, notFoundHandler(), "weekly_team_recap", map[string]string{"group_id": "g1"})
	if !strings.Contains(text, "| MRs merged | 0 |") {
		t.Error("expected zero merged MRs when API fails")
	}
}

// TestWeeklyTeamRecap_OpenMRConflicts verifies the open-MR conflict counting
// branch of weekly_team_recap.
func TestWeeklyTeamRecap_OpenMRConflicts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/g1/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") == "merged" {
			respondJSON(w, http.StatusOK, `[]`)
			return
		}
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":10,"title":"MR1","source_branch":"a","target_branch":"main","has_conflicts":true,"references":{"full":"group/proj!1"},"created_at":"2026-01-01T00:00:00Z"}]`)
	})
	mux.HandleFunc("GET /api/v4/groups/g1/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "weekly_team_recap", map[string]string{"group_id": "g1"})
	if !strings.Contains(text, "## Open MR Health") {
		t.Error("expected open MR health section")
	}
	if !strings.Contains(text, "| With conflicts | 1 |") {
		t.Error("expected conflict count of 1")
	}
}

// prompt_project_reports.go error branches.

// TestBranchMRSummary_APIError verifies that branch_mr_summary returns an
// error when the MR list API fails.
func TestBranchMRSummary_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "branch_mr_summary",
		map[string]string{"project_id": "42", "target_branch": "main"})
}

// TestProjectActivityReport_EventsAPIErrorWithMergedMRs verifies that
// project_activity_report tolerates a failing events API (warn-and-continue)
// and still renders the recently-merged MRs section when merged MRs exist.
func TestProjectActivityReport_EventsAPIErrorWithMergedMRs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") == "merged" {
			respondJSON(w, http.StatusOK, groupProjMRJSON)
			return
		}
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(pathIssues, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	// Project events endpoint falls through to 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	text := getPromptText(t, mux, "project_activity_report", map[string]string{"project_id": "42"})
	if !strings.Contains(text, "| Events | 0 |") {
		t.Error("expected zero events when events API fails")
	}
	if !strings.Contains(text, "Recently Merged MRs") {
		t.Error("expected recently merged MRs section")
	}
}

// TestMRDiscussionHealth_APIError verifies that mr_discussion_health returns
// an error when the MR list API fails.
func TestMRDiscussionHealth_APIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "mr_discussion_health", map[string]string{"project_id": "42"})
}

// TestMRDiscussionHealth_DiscussionsAPIError verifies that a failing
// discussion API for a single MR yields a zero-thread row instead of failing
// the whole prompt (warn-and-continue branch).
func TestMRDiscussionHealth_DiscussionsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":42,"title":"MR1","source_branch":"a","target_branch":"main","author":{"username":"alice"}}]`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})

	text := getPromptText(t, mux, "mr_discussion_health", map[string]string{"project_id": "42"})
	if !strings.Contains(text, "| !1 | MR1 | @alice | 0 | 0 |") {
		t.Error("expected zero-thread row when discussions API fails")
	}
}

// prompt_milestone_label.go edge branch.

// TestProjectContributors_PieChartCap verifies that the contributor pie chart
// is capped at eight entries while the table still lists everyone.
func TestProjectContributors_PieChartCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 1; i <= 9; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"name":"c%d","commits":%d,"additions":0,"deletions":0}`, i, 10-i)
	}
	sb.WriteString("]")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/repository/contributors", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, sb.String())
	})

	text := getPromptText(t, mux, "project_contributors", map[string]string{"project_id": "42"})
	if !strings.Contains(text, "| c9 |") {
		t.Error("expected ninth contributor in the table")
	}
	if !strings.Contains(text, `"c8" :`) {
		t.Error("expected eighth contributor in the pie chart")
	}
	if strings.Contains(text, `"c9" :`) {
		t.Error("pie chart should be capped at eight entries")
	}
}

// prompt_git_workflow.go error branches.

// TestAuditCommitHygiene_CompareAPIError verifies that audit_commit_hygiene
// returns an error when the repository compare API fails.
func TestAuditCommitHygiene_CompareAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "audit_commit_hygiene",
		map[string]string{"project_id": "42", "from": "v1.0.0"})
}

// TestAuditCommitHygiene_SameRef verifies the early-exit branch when both
// refs point at the same commit and no commits are returned.
func TestAuditCommitHygiene_SameRef(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathRepoCompare, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"compare_same_ref":true,"commits":[],"diffs":[]}`)
	})

	text := getPromptText(t, mux, "audit_commit_hygiene",
		map[string]string{"project_id": "42", "from": "v1.0.0", "to": "v1.0.0"})
	if !strings.Contains(text, "No commits found") {
		t.Error("expected no-commits message for same ref")
	}
}

// TestMRDescriptionQuality_InvalidIID verifies that a non-numeric MR IID is
// rejected before any API call.
func TestMRDescriptionQuality_InvalidIID(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "mr_description_quality",
		map[string]string{"project_id": "42", "merge_request_iid": "abc"})
}

// TestMRDescriptionQuality_MRAPIError verifies the MR-fetch error branch of
// mr_description_quality.
func TestMRDescriptionQuality_MRAPIError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "mr_description_quality",
		map[string]string{"project_id": "42", "merge_request_iid": "5"})
}

// TestMRDescriptionQuality_DiffsAPIError verifies the diff-fetch error branch
// of mr_description_quality (MR fetch succeeds).
func TestMRDescriptionQuality_DiffsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+pathMR5, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"iid":5,"title":"T","source_branch":"a","target_branch":"main","description":""}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
	getPromptExpectError(t, mux, "mr_description_quality",
		map[string]string{"project_id": "42", "merge_request_iid": "5"})
}

// TestCommitBody_EmptyMessage verifies that a whitespace-only commit message
// yields an empty body.
func TestCommitBody_EmptyMessage(t *testing.T) {
	if got := commitBody(&gl.Commit{Message: "   "}); got != "" {
		t.Errorf("commitBody() = %q, want empty", got)
	}
}

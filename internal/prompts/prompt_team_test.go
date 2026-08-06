// prompt_team_test.go contains unit tests for team analysis MCP prompts.
package prompts

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestUserActivityReport_EventBreakdown verifies UserActivityReport when event breakdown.
func TestUserActivityReport_EventBreakdown(t *testing.T) {
	mux := http.NewServeMux()
	now := time.Now()

	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id": 42, "username": "alice"}]`)
	})
	mux.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, r *http.Request) {
		events := []*gl.ContributionEvent{
			{ActionName: "pushed to", CreatedAt: &now},
			{ActionName: "pushed to", CreatedAt: &now},
			{ActionName: "opened", CreatedAt: &now},
		}
		data, _ := json.Marshal(events)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "user_activity_report",
		Arguments: map[string]string{"username": "alice"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "@alice") {
		t.Error("expected @alice in output")
	}
	if !strings.Contains(text, "Total events: 3") {
		t.Error("expected total events count of 3")
	}
	if !strings.Contains(text, "pushed to") {
		t.Error("expected 'pushed to' event type")
	}
}

// TestUserActivityReport_MissingUsername verifies UserActivityReport when missing username.
func TestUserActivityReport_MissingUsername(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})

	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "user_activity_report",
	})
	if err == nil {
		t.Error("expected error for missing username")
	}
}

// TestTeamOverview_MemberWorkload verifies TeamOverview when member workload.
func TestTeamOverview_MemberWorkload(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/members", func(w http.ResponseWriter, r *http.Request) {
		members := []*gl.GroupMember{
			{ID: 1, Username: "alice", Name: "Alice A", State: "active"},
			{ID: 2, Username: "bob", Name: "Bob B", State: "active"},
		}
		data, _ := json.Marshal(members)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state == "merged" {
			respondJSON(w, http.StatusOK, `[]`)
			return
		}
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "MR1", ProjectID: 10, SourceBranch: "a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}},
				References: &gl.IssueReferences{Full: "group/proj!1"},
			},
			{
				IID: 2, Title: "MR2", ProjectID: 10, SourceBranch: "b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/proj!2"},
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "team_overview",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Active members | 2") {
		t.Error("expected 2 active members")
	}
	if !strings.Contains(text, "Open MRs | 2") {
		t.Error("expected 2 open MRs")
	}
	if !strings.Contains(text, "@alice") {
		t.Error("expected alice in member table")
	}
	if !strings.Contains(text, "@bob") {
		t.Error("expected bob in member table")
	}
}

// TestTeamOverview_MissingGroupID verifies TeamOverview when missing group ID.
func TestTeamOverview_MissingGroupID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "team_overview",
	})
	if err == nil {
		t.Error("expected error for missing group_id")
	}
}

// TestTeamMRDashboard_GroupsByProject verifies TeamMRDashboard when groups by project.
func TestTeamMRDashboard_GroupsByProject(t *testing.T) {
	created := time.Now().Add(-2 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 10, Title: "Feature A", ProjectID: 10, SourceBranch: "feat/a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/alpha!10"},
			},
			{
				IID: 20, Title: "Feature B", ProjectID: 20, SourceBranch: "feat/b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, Draft: true,
				References: &gl.IssueReferences{Full: "group/beta!20"},
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_mr_dashboard",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "group/alpha") {
		t.Error("expected group/alpha project")
	}
	if !strings.Contains(text, "group/beta") {
		t.Error("expected group/beta project")
	}
	if !strings.Contains(text, "Draft | 1") {
		t.Error("expected draft count of 1")
	}
}

// TestGroupMRDashboard_TargetBranchFilter verifies GroupMRDashboard when target branch filter.
func TestGroupMRDashboard_TargetBranchFilter(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("target_branch") != "develop" {
			t.Errorf("expected target_branch=develop, got %q", r.URL.Query().Get("target_branch"))
		}
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_mr_dashboard",
		Arguments: map[string]string{"group_id": "mygroup", "target_branch": "develop"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "targeting develop") {
		t.Error("expected 'targeting develop' in output")
	}
}

// TestGroupMRDashboard_EmptyResult verifies GroupMRDashboard when empty result.
func TestGroupMRDashboard_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_mr_dashboard",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Error("expected empty message")
	}
}

// TestReviewerWorkload_Distribution verifies ReviewerWorkload when distribution.
func TestReviewerWorkload_Distribution(t *testing.T) {
	created := time.Now().Add(-4 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/members", func(w http.ResponseWriter, r *http.Request) {
		members := []*gl.GroupMember{
			{ID: 1, Username: "alice", Name: "Alice A", State: "active"},
			{ID: 2, Username: "bob", Name: "Bob B", State: "active"},
			{ID: 3, Username: "charlie", Name: "Charlie C", State: "active"},
		}
		data, _ := json.Marshal(members)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "MR1", ProjectID: 10, SourceBranch: "a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}, {Username: "charlie"}},
				References: &gl.IssueReferences{Full: "group/proj!1"},
			},
			{
				IID: 2, Title: "MR2", ProjectID: 10, SourceBranch: "b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}},
				References: &gl.IssueReferences{Full: "group/proj!2"},
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "reviewer_workload",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	// bob reviews 2 MRs (MR1 + MR2), charlie reviews 1 (MR1)
	if !strings.Contains(text, "Total review assignments | 3") {
		t.Error("expected total review assignments of 3")
	}
	if !strings.Contains(text, "Active reviewers | 2") {
		t.Error("expected 2 active reviewers")
	}
	if !strings.Contains(text, "@bob") {
		t.Error("expected bob in output")
	}
}

// TestReviewerWorkload_MissingGroupID verifies ReviewerWorkload when missing group ID.
func TestReviewerWorkload_MissingGroupID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "reviewer_workload",
	})
	if err == nil {
		t.Error("expected error for missing group_id")
	}
}

// notFoundHandler returns a handler that answers every request with 404,
// used to drive the first GitLab API call of a handler into its error branch.
func notFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
}

// getPromptExpectErrorWithoutAPICall asserts that a prompt fails argument
// validation before it reaches GitLab.
//
// getPromptExpectError with notFoundHandler cannot express this: a handler that
// answers 404 makes the prompt fail either way, so the test passes whether the
// argument was rejected up front or the first API call simply failed. Counting
// requests and requiring zero is what pins the validation to the right side of
// the client call.
func getPromptExpectErrorWithoutAPICall(t *testing.T, name string, args map[string]string) {
	t.Helper()
	var requests atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		respondNotFound(w)
	})
	getPromptExpectError(t, handler, name, args)
	if got := requests.Load(); got != 0 {
		t.Errorf("%s made %d GitLab request(s); argument validation must reject before the client call", name, got)
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

// TestUserActivityReport_UserLookupAPIError_ReturnsError verifies that the
// user_activity_report prompt returns an error when the /users lookup fails
// (covers both the handler branch and resolveUser's lookup-error branch).
func TestUserActivityReport_UserLookupAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "user_activity_report", map[string]string{"username": "alice"})
}

// TestUserActivityReport_NoEventsWithMRs_RendersMRSectionOnly verifies the user_activity_report
// branches for an empty event list combined with non-empty merged and
// under-review MR lists (the grouped per-project MR tables).
func TestUserActivityReport_NoEventsWithMRs_RendersMRSectionOnly(t *testing.T) {
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

// TestUserActivityReport_MultiDayChart_RendersDailyChart verifies the daily activity chart
// separator branches when contribution events span more than one day.
func TestUserActivityReport_MultiDayChart_RendersDailyChart(t *testing.T) {
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

// TestTeamOverview_MembersAPIError_ReturnsError verifies that team_overview returns an
// error when the group members API fails.
func TestTeamOverview_MembersAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "team_overview", map[string]string{"group_id": "g1"})
}

// TestBuildTeamOverviewStats_MergedMRAuthors_CountsMergedMRAuthors verifies the merged-MR
// attribution branches: authors that are active members are counted while
// unknown or nil authors are ignored.
func TestBuildTeamOverviewStats_MergedMRAuthors_CountsMergedMRAuthors(t *testing.T) {
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

// TestWriteTeamOverviewChart_EmptyStats_WritesNothing verifies the chart is omitted when
// there are no member stats to plot.
func TestWriteTeamOverviewChart_EmptyStats_WritesNothing(t *testing.T) {
	var b strings.Builder
	writeTeamOverviewChart(&b, map[string]*teamOverviewMemberStats{})
	if b.Len() != 0 {
		t.Errorf("expected no chart for empty stats, got: %q", b.String())
	}
}

// TestGroupMRDashboard_MissingGroupID_ReturnsError verifies that group_mr_dashboard
// rejects requests without a group_id.
func TestGroupMRDashboard_MissingGroupID_ReturnsError(t *testing.T) {
	getPromptExpectErrorWithoutAPICall(t, "group_mr_dashboard", nil)
}

// TestGroupMRDashboard_APIError_ReturnsError verifies that group_mr_dashboard returns an
// error when the group MR list API fails.
func TestGroupMRDashboard_APIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "group_mr_dashboard", map[string]string{"group_id": "g1"})
}

// TestGroupMRDashboard_ConflictCount_ReportsConflictCount verifies the conflict-counting branch
// when an MR in the dashboard has merge conflicts.
func TestGroupMRDashboard_ConflictCount_ReportsConflictCount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/g1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":10,"title":"MR1","source_branch":"a","target_branch":"main","has_conflicts":true,"references":{"full":"group/proj!1"},"created_at":"2026-01-01T00:00:00Z"}]`)
	})

	text := getPromptText(t, mux, "group_mr_dashboard", map[string]string{"group_id": "g1"})
	if !strings.Contains(text, "| With conflicts | 1 |") {
		t.Error("expected conflict count of 1")
	}
}

// TestReviewerWorkload_MembersAPIError_ReturnsError verifies that reviewer_workload
// returns an error when the group members API fails.
func TestReviewerWorkload_MembersAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "reviewer_workload", map[string]string{"group_id": "g1"})
}

// TestRecordReviewerWorkloadMR_UnknownReviewer_IsIgnored verifies that a reviewer who
// is not a group member gets a stats entry created on the fly.
func TestRecordReviewerWorkloadMR_UnknownReviewer_IsIgnored(t *testing.T) {
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

// TestWriteReviewerWorkloadChart_NoActiveReviewers_WritesNothing verifies the chart is
// omitted when nobody is reviewing anything.
func TestWriteReviewerWorkloadChart_NoActiveReviewers_WritesNothing(t *testing.T) {
	var b strings.Builder
	writeReviewerWorkloadChart(&b, map[string]*reviewerWorkloadStats{"a": {name: "A"}}, 0)
	if b.Len() != 0 {
		t.Errorf("expected no chart with zero active reviewers, got: %q", b.String())
	}
}

// prompt_audit.go error branches.

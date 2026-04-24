// prompt_analytics_test.go contains unit tests for analytics MCP prompts.

package prompts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	routeProjectMergeRequests = "GET /api/v4/projects/{project}/merge_requests"
	testAnalyticsProjectPath  = "mygroup/myproject"
	errMissingProjectID       = "expected error for missing project_id"
	testReleaseBranch         = "release/2.0"
)

// merge_velocity.

// TestMergeVelocity_CalculatesMetrics verifies the behavior of merge velocity calculates metrics.
func TestMergeVelocity_CalculatesMetrics(t *testing.T) {
	created := time.Now().Add(-10 * 24 * time.Hour)
	merged := time.Now().Add(-2 * 24 * time.Hour)
	merged2 := time.Now().Add(-1 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeProjectMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Feature A", SourceBranch: "feat/a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created, MergedAt: &merged},
			{IID: 2, Title: "Feature B", SourceBranch: "feat/b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, MergedAt: &merged2},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "merge_velocity",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath, "days": "30"},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "MRs merged | 2") {
		t.Error("expected 2 merged MRs")
	}
	if !strings.Contains(text, "MRs/week") {
		t.Error("expected merge rate")
	}
	if !strings.Contains(text, "Average time-to-merge") {
		t.Error("expected average time-to-merge")
	}
}

// TestMergeVelocity_EmptyResult verifies the behavior of merge velocity empty result.
func TestMergeVelocity_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProjectMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "merge_velocity",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No merged MRs found") {
		t.Error("expected empty result message")
	}
}

// TestMergeVelocity_MissingProjectID verifies the behavior of merge velocity missing project i d.
func TestMergeVelocity_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "merge_velocity",
	})
	if err == nil {
		t.Fatal(errMissingProjectID)
	}
}

// release_readiness.

// TestReleaseReadiness_ShowsBlockers verifies the behavior of release readiness shows blockers.
func TestReleaseReadiness_ShowsBlockers(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeProjectMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Feature A", SourceBranch: "feat/a", TargetBranch: testReleaseBranch,
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created, Draft: true},
			{IID: 2, Title: "Feature B", SourceBranch: "feat/b", TargetBranch: testReleaseBranch,
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, HasConflicts: true},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/1/discussions", func(w http.ResponseWriter, r *http.Request) {
		discussions := []*gl.Discussion{
			{ID: "d1", Notes: []*gl.Note{{Resolvable: true, Resolved: false}}},
		}
		data, _ := json.Marshal(discussions)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/2/discussions", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "release_readiness",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath, "branch": testReleaseBranch},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, testReleaseBranch) {
		t.Error("expected branch name in output")
	}
	if !strings.Contains(text, "Drafts | 1") {
		t.Error("expected 1 draft")
	}
	if !strings.Contains(text, "With conflicts | 1") {
		t.Error("expected 1 conflict")
	}
	if !strings.Contains(text, "Unresolved threads | 1") {
		t.Error("expected 1 unresolved thread")
	}
}

// TestReleaseReadiness_NoOpenMRs verifies the behavior of release readiness no open m rs.
func TestReleaseReadiness_NoOpenMRs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeProjectMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "release_readiness",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "appears ready for release") {
		t.Error("expected ready message")
	}
}

// TestReleaseReadiness_MissingProjectID verifies the behavior of release readiness missing project i d.
func TestReleaseReadiness_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "release_readiness",
	})
	if err == nil {
		t.Fatal(errMissingProjectID)
	}
}

// release_cadence.

// TestReleaseCadence_CalculatesIntervals verifies the behavior of release cadence calculates intervals.
func TestReleaseCadence_CalculatesIntervals(t *testing.T) {
	r1Date := time.Now().Add(-60 * 24 * time.Hour)
	r2Date := time.Now().Add(-30 * 24 * time.Hour)
	r3Date := time.Now().Add(-5 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/releases", func(w http.ResponseWriter, r *http.Request) {
		releases := []*gl.Release{
			{TagName: "v1.0.0", Name: "Release 1.0.0", ReleasedAt: &r1Date},
			{TagName: "v1.1.0", Name: "Release 1.1.0", ReleasedAt: &r2Date},
			{TagName: "v1.2.0", Name: "Release 1.2.0", ReleasedAt: &r3Date},
		}
		data, _ := json.Marshal(releases)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "release_cadence",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath, "days": "90"},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Total releases | 3") {
		t.Error("expected 3 releases")
	}
	if !strings.Contains(text, "Average interval") {
		t.Error("expected average interval")
	}
	if !strings.Contains(text, "v1.0.0") {
		t.Error("expected first release tag")
	}
}

// TestReleaseCadence_NoReleases verifies the behavior of release cadence no releases.
func TestReleaseCadence_NoReleases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/releases", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "release_cadence",
		Arguments: map[string]string{"project_id": testAnalyticsProjectPath},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No releases found") {
		t.Error("expected empty result message")
	}
}

// TestReleaseCadence_MissingProjectID verifies the behavior of release cadence missing project i d.
func TestReleaseCadence_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "release_cadence",
	})
	if err == nil {
		t.Fatal(errMissingProjectID)
	}
}

// weekly_team_recap.

// TestWeeklyTeam_RecapCombinesData verifies the behavior of weekly team recap combines data.
func TestWeeklyTeam_RecapCombinesData(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	merged := time.Now().Add(-1 * 24 * time.Hour)
	mux := http.NewServeMux()

	callCount := 0
	mux.HandleFunc("GET /api/v4/groups/{group}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		state := r.URL.Query().Get("state")
		if state == "merged" {
			mrs := []*gl.BasicMergeRequest{
				{IID: 1, Title: "Merged feature", SourceBranch: "feat/done", TargetBranch: "main",
					Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created, MergedAt: &merged,
					References: &gl.IssueReferences{Full: "group/alpha!1"}},
			}
			data, _ := json.Marshal(mrs)
			respondJSON(w, http.StatusOK, string(data))
		} else {
			mrs := []*gl.BasicMergeRequest{
				{IID: 2, Title: "Open MR", SourceBranch: "feat/wip", TargetBranch: "main",
					Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, Draft: true,
					References: &gl.IssueReferences{Full: "group/alpha!2"}},
			}
			data, _ := json.Marshal(mrs)
			respondJSON(w, http.StatusOK, string(data))
		}
	})
	mux.HandleFunc("GET /api/v4/groups/{group}/issues", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "weekly_team_recap",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "MRs merged | 1") {
		t.Error("expected 1 merged MR")
	}
	if !strings.Contains(text, "MRs open | 1") {
		t.Error("expected 1 open MR")
	}
	if !strings.Contains(text, "Drafts | 1") {
		t.Error("expected 1 draft in health section")
	}
}

// TestWeeklyTeamRecap_MissingGroupID verifies the behavior of weekly team recap missing group i d.
func TestWeeklyTeamRecap_MissingGroupID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "weekly_team_recap",
	})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

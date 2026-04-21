// prompt_team_test.go contains unit tests for team analysis MCP prompts.
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

// TestUserActivityReport_EventBreakdown verifies that UserActivityReport handles the event breakdown scenario correctly.
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

// TestUserActivityReport_MissingUsername verifies that UserActivityReport handles the missing username scenario correctly.
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

// TestTeamOverview_MemberWorkload verifies that TeamOverview handles the member workload scenario correctly.
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
			{IID: 1, Title: "MR1", ProjectID: 10, SourceBranch: "a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}},
				References: &gl.IssueReferences{Full: "group/proj!1"}},
			{IID: 2, Title: "MR2", ProjectID: 10, SourceBranch: "b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/proj!2"}},
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

// TestTeamOverview_MissingGroupID verifies that TeamOverview handles the missing group i d scenario correctly.
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

// TestTeamMRDashboard_GroupsByProject verifies that TeamMRDashboard handles the groups by project scenario correctly.
func TestTeamMRDashboard_GroupsByProject(t *testing.T) {
	created := time.Now().Add(-2 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 10, Title: "Feature A", ProjectID: 10, SourceBranch: "feat/a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/alpha!10"}},
			{IID: 20, Title: "Feature B", ProjectID: 20, SourceBranch: "feat/b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, Draft: true,
				References: &gl.IssueReferences{Full: "group/beta!20"}},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "team_mr_dashboard",
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

// TestTeamMRDashboard_TargetBranchFilter verifies that TeamMRDashboard handles the target branch filter scenario correctly.
func TestTeamMRDashboard_TargetBranchFilter(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("target_branch") != "develop" {
			t.Errorf("expected target_branch=develop, got %q", r.URL.Query().Get("target_branch"))
		}
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "team_mr_dashboard",
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

// TestTeamMRDashboard_EmptyResult verifies that TeamMRDashboard handles the empty result scenario correctly.
func TestTeamMRDashboard_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/mygroup/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "team_mr_dashboard",
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

// TestReviewerWorkload_Distribution verifies that ReviewerWorkload handles the distribution scenario correctly.
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
			{IID: 1, Title: "MR1", ProjectID: 10, SourceBranch: "a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}, {Username: "charlie"}},
				References: &gl.IssueReferences{Full: "group/proj!1"}},
			{IID: 2, Title: "MR2", ProjectID: 10, SourceBranch: "b", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created,
				Reviewers:  []*gl.BasicUser{{Username: "bob"}},
				References: &gl.IssueReferences{Full: "group/proj!2"}},
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

// TestReviewerWorkload_MissingGroupID verifies that ReviewerWorkload handles the missing group i d scenario correctly.
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

// prompt_cross_project_test.go contains unit tests for cross-project MCP prompts.

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
	routeGetUser          = "GET /api/v4/user"
	routeGetMergeRequests = "GET /api/v4/merge_requests"
	routeGetIssues        = "GET /api/v4/issues"
	fmtGetPromptFailed    = "GetPrompt failed: %v"
	actionPushedTo        = "pushed to"
)

// TestMyOpenMRsGroups_ByProject verifies the behavior of my open m rs groups by project.
func TestMyOpenMRsGroups_ByProject(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})

	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Fix auth", ProjectID: 10, SourceBranch: "fix/auth", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/alpha!1"}},
			{IID: 2, Title: "Add cache", ProjectID: 10, SourceBranch: "feat/cache", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created, Draft: true,
				References: &gl.IssueReferences{Full: "group/alpha!2"}},
			{IID: 3, Title: "Update docs", ProjectID: 20, SourceBranch: "docs/update", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created, HasConflicts: true,
				References: &gl.IssueReferences{Full: "group/beta!3"}},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_open_mrs",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text

	if !strings.Contains(text, "group/alpha") {
		t.Error("expected group/alpha project heading")
	}
	if !strings.Contains(text, "group/beta") {
		t.Error("expected group/beta project heading")
	}
	if !strings.Contains(text, "With conflicts | 1") {
		t.Error("expected conflict count of 1")
	}
	if !strings.Contains(text, "Draft | 1") {
		t.Error("expected draft count of 1")
	}
}

// TestMyOpenMRs_EmptyResult verifies the behavior of my open m rs empty result.
func TestMyOpenMRs_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_open_mrs",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Total open MRs | 0") {
		t.Error("expected total count 0")
	}
}

// TestMy_PendingReviewsGroupsByProject verifies the behavior of my pending reviews groups by project.
func TestMy_PendingReviewsGroupsByProject(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 10, Title: "Need review", ProjectID: 10, SourceBranch: "feat/a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "team/frontend!10"}},
			{IID: 20, Title: "Also review", ProjectID: 20, SourceBranch: "feat/b", TargetBranch: "develop",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "team/backend!20"}},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_pending_reviews",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "team/frontend") {
		t.Error("expected team/frontend project heading")
	}
	if !strings.Contains(text, "team/backend") {
		t.Error("expected team/backend project heading")
	}
	if !strings.Contains(text, "2 MRs") {
		t.Error("expected total MR count of 2")
	}
}

// TestMyPendingReviews_EmptyResult verifies the behavior of my pending reviews empty result.
func TestMyPendingReviews_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_pending_reviews",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No pending reviews") {
		t.Error("expected empty message")
	}
}

// TestMyIssuesGroups_ByProject verifies the behavior of my issues groups by project.
func TestMyIssuesGroups_ByProject(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 1, Title: "Bug in login", ProjectID: 10, CreatedAt: &created,
				Labels:     gl.Labels{"bug"},
				References: &gl.IssueReferences{Full: "group/alpha#1"}},
			{IID: 2, Title: "Add feature X", ProjectID: 20, CreatedAt: &created,
				Milestone:  &gl.Milestone{Title: "v2.0"},
				References: &gl.IssueReferences{Full: "group/beta#2"}},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_issues",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "group/alpha") {
		t.Error("expected group/alpha project heading")
	}
	if !strings.Contains(text, "group/beta") {
		t.Error("expected group/beta project heading")
	}
	if !strings.Contains(text, "Total | 2") {
		t.Error("expected total count of 2")
	}
}

// TestMyIssues_OverdueDetection verifies the behavior of my issues overdue detection.
func TestMyIssues_OverdueDetection(t *testing.T) {
	created := time.Now().Add(-10 * 24 * time.Hour)
	pastDue := gl.ISOTime(time.Now().Add(-2 * 24 * time.Hour))
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 1, Title: "Overdue task", ProjectID: 10, CreatedAt: &created,
				DueDate:    &pastDue,
				References: &gl.IssueReferences{Full: "group/alpha#1"}},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_issues",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Overdue | 1") {
		t.Error("expected overdue count of 1")
	}
}

// TestMyIssues_StateFilter verifies the behavior of my issues state filter.
func TestMyIssues_StateFilter(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "closed" {
			t.Errorf("expected state=closed, got %q", r.URL.Query().Get("state"))
		}
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "my_issues",
		Arguments: map[string]string{"state": "closed"},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}
}

// TestMyActivity_SummaryEventBreakdown verifies the behavior of my activity summary event breakdown.
func TestMyActivity_SummaryEventBreakdown(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc("GET /api/v4/events", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		events := []*gl.ContributionEvent{
			{ActionName: actionPushedTo, CreatedAt: &now},
			{ActionName: actionPushedTo, CreatedAt: &now},
			{ActionName: "commented on", CreatedAt: &now},
		}
		data, _ := json.Marshal(events)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "my_activity_summary",
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Total events: 3") {
		t.Error("expected total events count of 3")
	}
	if !strings.Contains(text, actionPushedTo) {
		t.Error("expected 'pushed to' event type")
	}
}

// TestMyActivitySummary_CustomDays verifies the behavior of my activity summary custom days.
func TestMyActivitySummary_CustomDays(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc("GET /api/v4/events", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "my_activity_summary",
		Arguments: map[string]string{"days": "14"},
	})
	if err != nil {
		t.Fatalf(fmtGetPromptFailed, err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "last 14 days") {
		t.Error("expected 'last 14 days' in output")
	}
}

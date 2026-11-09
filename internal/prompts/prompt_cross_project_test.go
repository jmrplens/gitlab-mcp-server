// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// prompt_cross_project_test.go contains unit tests for cross-project MCP prompts.
package prompts

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const (
	// routeGetUser identifies the route get user constant used by this package.
	routeGetUser = "GET /api/v4/user"
	// routeGetMergeRequests identifies the route get merge requests constant used by this package.
	routeGetMergeRequests = "GET /api/v4/merge_requests"
	// routeGetIssues identifies the route get issues constant used by this package.
	routeGetIssues = "GET /api/v4/issues"
	// fmtGetPromptFailed identifies the fmt get prompt failed constant used by this package.
	fmtGetPromptFailed = "GetPrompt failed: %v"
	// actionPushedTo identifies the action pushed to constant used by this package.
	actionPushedTo = "pushed to"
)

// TestMyOpenMRsGroups_ByProject verifies MyOpenMRsGroups when by project.
func TestMyOpenMRsGroups_ByProject(t *testing.T) {
	created := time.Now().Add(-3 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})

	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "Fix auth", ProjectID: 10, SourceBranch: "fix/auth", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "group/alpha!1"},
			},
			{
				IID: 2, Title: "Add cache", ProjectID: 10, SourceBranch: "feat/cache", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created, Draft: true,
				References: &gl.IssueReferences{Full: "group/alpha!2"},
			},
			{
				IID: 3, Title: "Update docs", ProjectID: 20, SourceBranch: "docs/update", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "testuser"}, CreatedAt: &created, HasConflicts: true,
				References: &gl.IssueReferences{Full: "group/beta!3"},
			},
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

// TestMyOpenMRs_EmptyResult verifies MyOpenMRs when empty result.
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

// TestMy_PendingReviewsGroupsByProject verifies My when pending reviews groups by project.
func TestMy_PendingReviewsGroupsByProject(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 10, Title: "Need review", ProjectID: 10, SourceBranch: "feat/a", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "team/frontend!10"},
			},
			{
				IID: 20, Title: "Also review", ProjectID: 20, SourceBranch: "feat/b", TargetBranch: "develop",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created,
				References: &gl.IssueReferences{Full: "team/backend!20"},
			},
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

// TestMyPendingReviews_EmptyResult verifies MyPendingReviews when empty result.
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

// TestMyIssuesGroups_ByProject verifies MyIssuesGroups when by project.
func TestMyIssuesGroups_ByProject(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{
				IID: 1, Title: "Bug in login", ProjectID: 10, CreatedAt: &created,
				Labels:     gl.Labels{"bug"},
				References: &gl.IssueReferences{Full: "group/alpha#1"},
			},
			{
				IID: 2, Title: "Add feature X", ProjectID: 20, CreatedAt: &created,
				Milestone:  &gl.Milestone{Title: "v2.0"},
				References: &gl.IssueReferences{Full: "group/beta#2"},
			},
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

// TestMyIssues_OverdueDetection verifies MyIssues when overdue detection.
func TestMyIssues_OverdueDetection(t *testing.T) {
	created := time.Now().Add(-10 * 24 * time.Hour)
	pastDue := gl.ISOTime(time.Now().Add(-2 * 24 * time.Hour))
	mux := http.NewServeMux()

	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id": 1, "username": "testuser"}`)
	})
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{
				IID: 1, Title: "Overdue task", ProjectID: 10, CreatedAt: &created,
				DueDate:    &pastDue,
				References: &gl.IssueReferences{Full: "group/alpha#1"},
			},
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

// TestMyIssues_StateFilter verifies MyIssues when state filter.
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

// TestMyActivity_SummaryEventBreakdown verifies MyActivity when summary event breakdown.
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

// TestMyActivitySummary_CustomDays verifies MyActivitySummary when custom days.
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

// TestMyOpenMRs_UserAPIError_ReturnsError verifies that my_open_mrs returns an error when
// the current-user lookup fails.
func TestMyOpenMRs_UserAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_open_mrs", nil)
}

// TestMyOpenMRs_MRListAPIErrors_StillRendersSummary verifies that my_open_mrs degrades to an
// empty dashboard (warn-and-continue) when both MR list calls fail.
func TestMyOpenMRs_MRListAPIErrors_StillRendersSummary(t *testing.T) {
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

// TestMyPendingReviews_UserAPIError_ReturnsError verifies that my_pending_reviews returns
// an error when the current-user lookup fails.
func TestMyPendingReviews_UserAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_pending_reviews", nil)
}

// TestMyPendingReviews_ListAPIError_ReturnsError verifies that my_pending_reviews returns
// an error when the reviewer MR list fails.
func TestMyPendingReviews_ListAPIError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
	getPromptExpectError(t, mux, "my_pending_reviews", nil)
}

// TestMyIssues_UserAPIError_ReturnsError verifies that my_issues returns an error when the
// current-user lookup fails.
func TestMyIssues_UserAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_issues", nil)
}

// TestMyIssues_ListAPIError_ReturnsError verifies that my_issues returns an error when the
// issue list fails.
func TestMyIssues_ListAPIError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		respondNotFound(w)
	})
	getPromptExpectError(t, mux, "my_issues", nil)
}

// TestMyActivitySummary_UserAPIError_ReturnsError verifies that my_activity_summary
// returns an error when the current-user lookup fails.
func TestMyActivitySummary_UserAPIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "my_activity_summary", nil)
}

// TestMyActivitySummary_MRListAPIErrors_StillRendersSummary verifies that my_activity_summary
// renders N/A rows (warn-and-continue) when the merged and reviewed MR list
// calls fail.
func TestMyActivitySummary_MRListAPIErrors_StillRendersSummary(t *testing.T) {
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

// TestMyOpenMRs_SameIIDInTwoProjects_CountsThemApart verifies that the
// author/assignee split counts merge requests by project and IID together.
//
// An IID is unique inside a project and nowhere else, and this prompt spans
// every project the caller can see. Keyed on the IID alone, an MR merely
// assigned to the caller counted as authored by them whenever some project they
// do author in happened to have an MR with the same number — which, since IIDs
// start at 1 in every project, is most of them.
func TestMyOpenMRs_SameIIDInTwoProjects_CountsThemApart(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":7,"username":"alice"}`)
	})
	mux.HandleFunc("GET /api/v4/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		// !1 of project 1 is authored by the caller; !1 of project 2 is only
		// assigned to them, and the two share nothing but their number.
		if r.URL.Query().Get("author_id") != "" {
			respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":1,"title":"Authored","references":{"full":"group/one!1"}}]`)
			return
		}
		respondJSON(w, http.StatusOK, `[{"iid":1,"project_id":2,"title":"Assigned","references":{"full":"group/two!1"}}]`)
	})

	text := getPromptText(t, mux, "my_open_mrs", nil)

	for _, want := range []string{
		"| Total open MRs | 2 |",
		"| As author | 1 |",
		"| As assignee (not author) | 1 |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("summary is missing %q:\n%s", want, text)
			}
		})
	}
	if strings.Contains(text, "| As author | 2 |") {
		t.Errorf("an assigned MR was counted as authored because the IIDs matched:\n%s", text)
	}
}

// TestMyIssues_TheNoMilestoneCount_IsTheIssuesWithoutOne pins the second
// counter of the personal issue dashboard.
//
// The overdue count beside it is asserted elsewhere; this one is an equality
// against nil and an increment, and every fixture gave every issue the same
// answer, so the test could not tell "has no milestone" from "has one" and the
// counter could as easily count down. A backlog dashboard exists to name what
// is unplanned.
func TestMyIssues_TheNoMilestoneCount_IsTheIssuesWithoutOne(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	// The due dates straddle today, so the overdue comparison is driven both
	// ways in the same run: it used to see only dates in the past, where a
	// reversed comparison counts every issue as overdue and reads the same.
	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02")
	future := time.Now().Add(48 * time.Hour).Format("2006-01-02")
	mux.HandleFunc(routeGetIssues, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[
			{"iid":1,"project_id":10,"title":"Planned","references":{"full":"group/proj#1"},"milestone":{"id":5,"title":"v1.0"},"due_date":"`+future+`","created_at":"2026-01-01T00:00:00Z"},
			{"iid":2,"project_id":10,"title":"Unplanned","references":{"full":"group/proj#2"},"due_date":"`+past+`","created_at":"2026-01-01T00:00:00Z"},
			{"iid":3,"project_id":10,"title":"Also unplanned","references":{"full":"group/proj#3"},"created_at":"2026-01-01T00:00:00Z"}
		]`)
	})

	text := getPromptText(t, mux, "my_issues", nil)

	if !strings.Contains(text, "| Total | 3 |") {
		t.Errorf("expected three issues:\n%s", text)
	}
	if !strings.Contains(text, "| No milestone | 2 |") {
		t.Errorf("expected the two issues without a milestone to be counted:\n%s", text)
	}
	if !strings.Contains(text, "| Overdue | 1 |") {
		t.Errorf("only the issue whose due date has passed is overdue:\n%s", text)
	}
}

// TestMyActivitySummary_APeriodWithNoEvents_HasNoChart verifies that the daily
// activity chart is written only when there were events to chart.
//
// The guard is a `len(events) > 0` no test saw false, so read as `>= 0` a
// quiet week produces a Mermaid block with an empty axis and an empty bar list
// — a chart of nothing, inside a fence, which clients render as a broken
// diagram rather than as the absence it is.
func TestMyActivitySummary_APeriodWithNoEvents_HasNoChart(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("GET /api/v4/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(routeGetMergeRequests, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "my_activity_summary", map[string]string{"days": "7"})

	if !strings.Contains(text, "No contribution events found in this period.") {
		t.Errorf("expected the empty-period sentence:\n%s", text)
	}
	if strings.Contains(text, "## Daily Activity") {
		t.Errorf("there were no events, yet a chart section was written:\n%s", text)
	}
}

// promptHandlerFunc is the signature every prompt handler of this package
// shares, so a case table can name the handler it drives.
type promptHandlerFunc func(context.Context, *gitlabclient.Client, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

// assertGlobalListScope drives one prompt handler against a GitLab that
// answers route, one of the global lists (GET /merge_requests or GET /issues),
// and fails unless every request to it asked for scope=all and the handler
// made wantCalls of them.
//
// Those two routes read a missing scope as created_by_me, so a prompt that
// filters them by a user without one sees that user's items only among what
// the caller created (issue 1164). A mock answers whatever it is asked, which
// is why the prompts rendered the same text with and without the scope and no
// test noticed: the request is the only place the defect shows. The call count
// is held as well, so a list the handler stopped reading cannot pass by never
// reaching the assertion.
//
// The caller resolves to user 1 and the username "alice" to user 42, and both
// event routes answer empty, so one handler serves a prompt read for the
// caller and one read for somebody else.
func assertGlobalListScope(t *testing.T, handle promptHandlerFunc, args map[string]string, route string, wantCalls int64) {
	t.Helper()
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, aliceLookupJSON)
	})
	mux.HandleFunc("GET /api/v4/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		testutil.AssertQueryParam(t, r, "scope", "all")
		respondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, mux)
	if _, err := handle(t.Context(), client, testPromptRequest(args)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got := calls.Load(); got != wantCalls {
		t.Errorf("%s was requested %d time(s), want %d", route, got, wantCalls)
	}
}

// TestCrossProjectPrompts_GlobalListFilteredByUser_SendsScopeAll verifies that
// every global merge request list a cross-project prompt reads asks GitLab for
// everybody's items, for the caller and for another user alike, and so does
// the global issue list read about another user.
//
// Each of these lists is filtered by the user the prompt is about: the merge
// requests they author, are assigned or review, the issues assigned to them.
// Without scope=all GitLab applies that filter to the caller's own merge
// requests and issues only, so the pending reviews were the ones the caller
// had opened and was reviewing, almost always none, and every list about
// another user was that user's work intersected with the caller's. The
// caller's own issues are read with scope=assigned_to_me instead, which
// GitLab.com accepts where it refuses scope=all on the issue list, and which
// TestMyIssues_Scope_NamesTheCallerOrEverybody holds.
func TestCrossProjectPrompts_GlobalListFilteredByUser_SendsScopeAll(t *testing.T) {
	alice := map[string]string{argUsername: "alice"}
	tests := []struct {
		name      string
		handle    promptHandlerFunc
		args      map[string]string
		route     string
		wantCalls int64
	}{
		{name: "my_open_mrs for the caller", handle: handleMyOpenMRs, route: routeGetMergeRequests, wantCalls: 2},
		{name: "my_open_mrs for another user", handle: handleMyOpenMRs, args: alice, route: routeGetMergeRequests, wantCalls: 2},
		{name: "my_pending_reviews for the caller", handle: handleMyPendingReviews, route: routeGetMergeRequests, wantCalls: 1},
		{name: "my_pending_reviews for another user", handle: handleMyPendingReviews, args: alice, route: routeGetMergeRequests, wantCalls: 1},
		{name: "my_issues for another user", handle: handleMyIssues, args: alice, route: routeGetIssues, wantCalls: 1},
		{name: "my_activity_summary for the caller", handle: handleMyActivitySummary, route: routeGetMergeRequests, wantCalls: 2},
		{name: "my_activity_summary for another user", handle: handleMyActivitySummary, args: alice, route: routeGetMergeRequests, wantCalls: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertGlobalListScope(t, tt.handle, tt.args, tt.route, tt.wantCalls)
		})
	}
}

// scopeAllRefusedJSON is the body GitLab.com answers GET /issues?scope=all
// with since the feature flag restrict_global_issues_scope_all was enabled
// there, byte for byte as the issue 1779 report recorded it.
const scopeAllRefusedJSON = "{\"message\":\"400 Bad request - `scope=all` is not supported on this endpoint. Use `created_by_me` or `assigned_to_me`, or query a specific group or project.\"}"

// routePostGraphQL is the route GitLab serves its GraphQL API on.
const routePostGraphQL = "POST /api/graphql"

// myIssuesAgainst drives my_issues with args against a GitLab whose global
// issue list is answered by issues and whose GraphQL API by graphQL, the
// caller resolving to user 1 and the username "alice" to user 42, and returns
// the prompt's result and error. A nil graphQL fails the test on any GraphQL
// request, since my_issues reads GraphQL only when GitLab.com refuses the REST
// list.
func myIssuesAgainst(t *testing.T, args map[string]string, issues, graphQL http.HandlerFunc) (*mcp.GetPromptResult, error) {
	t.Helper()
	if graphQL == nil {
		graphQL = func(w http.ResponseWriter, _ *http.Request) {
			t.Errorf("my_issues sent a GraphQL request, want the REST issue list's answer alone")
			testutil.RespondGraphQLError(w, http.StatusOK, "unexpected request")
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc(routeGetUser, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, basicUserJSON)
	})
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, aliceLookupJSON)
	})
	mux.HandleFunc(routeGetIssues, issues)
	mux.HandleFunc(routePostGraphQL, graphQL)
	return handleMyIssues(t.Context(), testutil.NewTestClient(t, mux), testPromptRequest(args))
}

// refuseScopeAll answers the global issue list the way GitLab.com does since
// it refuses scope=all there (issue 1779).
func refuseScopeAll(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusBadRequest, scopeAllRefusedJSON)
}

// answerGraphQL returns a GraphQL handler that writes body, the whole JSON
// envelope, as GitLab's answer.
func answerGraphQL(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, body)
	}
}

// promptText returns the text of the one message result carries.
func promptText(t *testing.T, result *mcp.GetPromptResult) string {
	t.Helper()
	if len(result.Messages) != 1 {
		t.Fatalf("the prompt answered %d messages, want 1", len(result.Messages))
	}
	return result.Messages[0].Content.(*mcp.TextContent).Text
}

// wantAssignedIssuesQuery is the document my_issues sends where GitLab.com
// refuses scope=all, spelled out rather than read from [assignedIssuesQuery]
// so that a change to the document fails the comparison instead of moving
// with it. The issue types are what keeps the read to the issues GET /issues
// lists: every base type but epic, key_result and objective, where the root
// resolver, sent no types, would list objectives and key results as well.
const wantAssignedIssuesQuery = `query AssignedIssues($assignees: [String!], $state: IssuableState, $first: Int) {
  issues(assigneeUsernames: $assignees, state: $state, types: [ISSUE, INCIDENT, TEST_CASE, REQUIREMENT, TASK, TICKET], first: $first) {
    nodes {
      iid
      title
      reference(full: true)
      createdAt
      dueDate
      milestone {
        title
      }
      labels {
        nodes {
          title
        }
      }
    }
  }
}`

// assertAssignedIssuesRequest reports, with t.Errorf since it runs on the mock
// server's goroutine, a GraphQL request that is not [wantAssignedIssuesQuery]
// sent for the username "alice" in wantState, one page of 100.
func assertAssignedIssuesRequest(t *testing.T, r *http.Request, wantState string) {
	t.Helper()
	var body struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decoding the GraphQL request: %v", err)
	}
	if body.Query != wantAssignedIssuesQuery {
		t.Errorf("query = %q, want %q", body.Query, wantAssignedIssuesQuery)
	}
	want := map[string]any{"assignees": []any{"alice"}, "state": wantState, "first": float64(100)}
	if !reflect.DeepEqual(body.Variables, want) {
		t.Errorf("variables = %v, want %v", body.Variables, want)
	}
}

// assignedIssueJSON is an issue node as GitLab answers [assignedIssuesQuery]
// with: the iid as a String, a label it could not resolve answered null.
const assignedIssueJSON = `{"iid":"12","title":"Fix the parser","reference":"group/api#12","createdAt":"2026-01-02T10:00:00Z","dueDate":null,"milestone":{"title":"19.6"},"labels":{"nodes":[{"title":"bug"},null]}}`

// TestMyIssues_Scope_NamesTheCallerOrEverybody verifies the request my_issues
// sends for each user it can be about.
//
// GitLab.com refuses scope=all on the global issue list (issue 1779), which the
// prompt sent for the caller as well, so the default form of the prompt, which
// names no user, failed there on every call. The caller's issues are what
// scope=assigned_to_me names on every instance, with no assignee filter beside
// it. Another user's are named by the assignee filter, which needs scope=all,
// since GitLab otherwise applies it to the caller's own issues only (issue
// 1164).
func TestMyIssues_Scope_NamesTheCallerOrEverybody(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         map[string]string
		wantScope    string
		wantAssignee string
	}{
		{name: "the caller", wantScope: "assigned_to_me"},
		{name: "another user", args: map[string]string{argUsername: "alice"}, wantScope: "all", wantAssignee: "42"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			_, err := myIssuesAgainst(t, tt.args, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				testutil.AssertQueryParam(t, r, "scope", tt.wantScope)
				testutil.AssertQueryParam(t, r, "assignee_id", tt.wantAssignee)
				testutil.AssertQueryParam(t, r, "state", "opened")
				testutil.AssertQueryParam(t, r, "per_page", "100")
				respondJSON(w, http.StatusOK, `[]`)
			}, nil)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("the global issue list was requested %d time(s), want 1", got)
			}
		})
	}
}

// TestMyIssues_ScopeAllRefused_ReadsTheGraphQLIssueList verifies that
// my_issues still lists another user's issues across every project on
// GitLab.com, which refuses the scope=all its REST read sends (issue 1779).
//
// GraphQL's root issue list is not behind that refusal, and the issue
// dashboard of GitLab.com reads it, so the prompt reads it in its place, once,
// with the user's username, the state asked for and the same page of 100. The
// answer has to read as the REST one does: grouped under the project its
// reference names, the milestone, the labels (one GitLab answered null left
// out), the due date and the overdue and no-milestone counts, and with no
// note of a partial answer when GitLab answered the list whole.
func TestMyIssues_ScopeAllRefused_ReadsTheGraphQLIssueList(t *testing.T) {
	due := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	created := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	answer := `{"data":{"issues":{"nodes":[` +
		`{"iid":"12","title":"Fix the parser","reference":"group/api#12","createdAt":"` + created + `","dueDate":"` + due + `","milestone":{"title":"19.6"},"labels":{"nodes":[{"title":"bug"},null,{"title":"backend"}]}},` +
		`{"iid":"7","title":"Write the guide","reference":"group/docs#7","createdAt":"` + created + `","dueDate":null,"milestone":null,"labels":null}` +
		`]}}}`
	for _, tt := range []struct {
		name      string
		args      map[string]string
		wantState string
	}{
		{name: "the default state", args: map[string]string{argUsername: "alice"}, wantState: "opened"},
		{name: "a state named", args: map[string]string{argUsername: "alice", argState: "closed"}, wantState: "closed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var reads atomic.Int64
			result, err := myIssuesAgainst(t, tt.args, refuseScopeAll, func(w http.ResponseWriter, r *http.Request) {
				reads.Add(1)
				assertAssignedIssuesRequest(t, r, tt.wantState)
				respondJSON(w, http.StatusOK, answer)
			})
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got := reads.Load(); got != 1 {
				t.Errorf("the GraphQL issue list was read %d time(s), want 1", got)
			}
			text := promptText(t, result)
			for _, want := range []string{
				"# Issues Assigned to @alice (2 issues, state: " + tt.wantState + ")\n\n## Summary\n",
				"| Overdue | 1 |",
				"| No milestone | 1 |",
				"## group/api (1 issues)",
				"| #12 | Fix the parser | bug, backend | 19.6 | 1w | " + due + " |",
				"## group/docs (1 issues)",
				"| #7 | Write the guide | - | - | 1w | - |",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("the prompt does not say %q:\n%s", want, text)
				}
			}
		})
	}
}

// TestMyIssues_GraphQLListAnsweredInPart_SaysSo verifies what my_issues
// writes about the GraphQL list it reads on GitLab.com when GitLab answered it
// in part (issue 1298).
//
// GitLab nulls an issue it could not resolve where it sits and explains the
// null in errors[], keeping the rest of the list, so a list answered that way
// lists fewer issues than the user has, and the prompt says so above its
// summary, with GitLab's words escaped. A list answered whole, an empty one
// included, carries no such note, and an empty one is a list of no issues
// rather than a failure.
func TestMyIssues_GraphQLListAnsweredInPart_SaysSo(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		wantIssues string
		wantNote   string
	}{
		{
			name:       "errors beside the list",
			body:       `{"data":{"issues":{"nodes":[` + assignedIssueJSON + `,null]}},"errors":[{"message":"Timeout on labels | retry"}]}`,
			wantIssues: "(1 issues, state: opened)",
			wantNote:   "> GitLab answered this list in part (my_issues GraphQL errors: Timeout on labels &#124; retry), so an issue it could not resolve may be missing from it.\n\n## Summary\n",
		},
		{
			name:       "an issue answered null with no error",
			body:       `{"data":{"issues":{"nodes":[null,` + assignedIssueJSON + `]}}}`,
			wantIssues: "(1 issues, state: opened)",
			wantNote:   "> GitLab answered this list in part (my_issues: GitLab answered 1 of the page's items null with no error), so an issue it could not resolve may be missing from it.\n\n## Summary\n",
		},
		{
			name:       "an empty list",
			body:       `{"data":{"issues":{"nodes":[]}}}`,
			wantIssues: "(0 issues, state: opened)",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := myIssuesAgainst(t, map[string]string{argUsername: "alice"}, refuseScopeAll, answerGraphQL(tt.body))
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			text := promptText(t, result)
			if !strings.Contains(text, tt.wantIssues) {
				t.Errorf("the prompt does not count %q:\n%s", tt.wantIssues, text)
			}
			if got := strings.Contains(text, "answered this list in part"); got != (tt.wantNote != "") {
				t.Errorf("the prompt notes a partial answer = %v, want %v:\n%s", got, tt.wantNote != "", text)
			}
			if !strings.Contains(text, tt.wantNote) {
				t.Errorf("the prompt does not say %q:\n%s", tt.wantNote, text)
			}
		})
	}
}

// TestMyIssues_GraphQLListFails_ReportsBothReads verifies what my_issues
// answers when the GraphQL list it reads on GitLab.com fails as well.
//
// The error says that GitLab.com refused the REST list and that the GraphQL
// list read in its place failed, and then why: GitLab's refusal of the
// request, its errors[] beside a list it answered null or whose every issue it
// answered null, a list answered null with no error (read as empty, it would
// say the user has no issues, which nobody knows), and an iid that is not the
// number GitLab sends as a String.
func TestMyIssues_GraphQLListFails_ReportsBothReads(t *testing.T) {
	const both = "failed to list issues: GitLab.com refuses scope=all on the REST issue list, and the GraphQL issue list read in its place failed: "
	for _, tt := range []struct {
		name    string
		graphQL http.HandlerFunc
		wantErr string
	}{
		{
			name: "GitLab refuses the request",
			graphQL: func(w http.ResponseWriter, _ *http.Request) {
				respondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			},
			wantErr: "500",
		},
		{
			name:    "errors and no list",
			graphQL: answerGraphQL(`{"data":{"issues":null},"errors":[{"message":"You must provide at least one filter argument for this query"}]}`),
			wantErr: "my_issues GraphQL errors: You must provide at least one filter argument for this query",
		},
		{
			name:    "a list answered null with no error",
			graphQL: answerGraphQL(`{"data":{"issues":null}}`),
			wantErr: "my_issues: GitLab answered the issue list null with no error",
		},
		{
			name:    "every issue answered null, with errors",
			graphQL: answerGraphQL(`{"data":{"issues":{"nodes":[null]}},"errors":[{"message":"Internal server error"}]}`),
			wantErr: "my_issues GraphQL errors: Internal server error",
		},
		{
			name:    "an iid that is not a number",
			graphQL: answerGraphQL(`{"data":{"issues":{"nodes":[{"iid":"twelve","title":"t","reference":"g/p#1","createdAt":"2026-01-02T10:00:00Z","dueDate":null,"milestone":null,"labels":null}]}}}`),
			wantErr: "iid",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := myIssuesAgainst(t, map[string]string{argUsername: "alice"}, refuseScopeAll, tt.graphQL)
			if err == nil {
				t.Fatalf("my_issues answered %v, want the failure reported", result)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, both) {
				t.Errorf("error = %q, want it to start %q", msg, both)
			}
			if !strings.Contains(strings.TrimPrefix(msg, both), tt.wantErr) {
				t.Errorf("error = %q, want it to name %q", msg, tt.wantErr)
			}
		})
	}
}

// TestMyIssues_OtherRefusals_KeepThePlainError verifies that my_issues reads
// GraphQL only on GitLab.com's refusal of scope=all, which it recognizes by
// GitLab's sentence under a 400.
//
// GitLab answers 400 for any parameter it cannot read, and the sentence under
// another status is not that refusal either, so both keep the plain error of
// the REST list and send nothing else.
func TestMyIssues_OtherRefusals_KeepThePlainError(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "another 400", status: http.StatusBadRequest, body: `{"message":"400 Bad request - scope does not have a valid value"}`},
		{name: "the sentence under another status", status: http.StatusUnprocessableEntity, body: scopeAllRefusedJSON},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := myIssuesAgainst(t, map[string]string{argUsername: "alice"}, func(w http.ResponseWriter, _ *http.Request) {
				respondJSON(w, tt.status, tt.body)
			}, nil)
			if err == nil {
				t.Fatalf("my_issues answered %v, want the refusal reported", result)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "failed to list issues: ") {
				t.Errorf("error = %q, want it to say the issue list failed", msg)
			}
			if strings.Contains(msg, "GraphQL") {
				t.Errorf("error = %q, names a GraphQL read that was never sent", msg)
			}
		})
	}
}

// TestAssignedIssuesQuery_Complexity_IsUnderGitLabsLimit holds the GraphQL
// read my_issues sends on GitLab.com under the complexity GitLab refuses a
// query above. The figure is the estimate testutil computes from the pinned
// schema; no GitLab has reported one for this document, so it is held under
// the limit rather than pinned.
func TestAssignedIssuesQuery_Complexity_IsUnderGitLabsLimit(t *testing.T) {
	got, err := testutil.GitLabQueryComplexity(assignedIssuesQuery, map[string]any{"assignees": []any{"alice"}, "state": "opened", "first": maxListItems})
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got > testutil.GitLabAuthenticatedMaxComplexity {
		t.Errorf("assignedIssuesQuery costs %d, over the %d GitLab refuses a query above", got, testutil.GitLabAuthenticatedMaxComplexity)
	}
}

// prompt_analytics.go error branches.

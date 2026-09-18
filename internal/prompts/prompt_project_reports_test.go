// prompt_project_reports_test.go contains unit tests for project report MCP prompts.
package prompts

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// branch_mr_summary.

// TestBranchMRSummary_ListsMRsByBranch verifies BranchMRSummary lists MRs by branch.
func TestBranchMRSummary_ListsMRsByBranch(t *testing.T) {
	created := time.Now().Add(-2 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("target_branch") != "release/1.0" {
			t.Errorf("expected target_branch=release/1.0, got %s", r.URL.Query().Get("target_branch"))
		}
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "Fix auth", SourceBranch: "fix/auth", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created, Draft: true,
			},
			{
				IID: 2, Title: "Add cache", SourceBranch: "feat/cache", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, HasConflicts: true,
			},
			{
				IID: 3, Title: "Update docs", SourceBranch: "docs/update", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "charlie"}, CreatedAt: &created,
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "branch_mr_summary",
		Arguments: map[string]string{
			"project_id":    "mygroup/myproject",
			"target_branch": "release/1.0",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "release/1.0") {
		t.Error("expected target branch in header")
	}
	if !strings.Contains(text, "Draft | 1") {
		t.Error("expected draft count of 1")
	}
	if !strings.Contains(text, "With conflicts | 1") {
		t.Error("expected conflict count of 1")
	}
	if !strings.Contains(text, "Total | 3") {
		t.Error("expected total count of 3")
	}
}

// TestBranchMRSummary_MissingProjectID verifies BranchMRSummary when missing project ID.
func TestBranchMRSummary_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "branch_mr_summary",
		Arguments: map[string]string{"target_branch": "main"},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestBranchMRSummary_MissingTargetBranch verifies BranchMRSummary when missing target branch.
func TestBranchMRSummary_MissingTargetBranch(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "branch_mr_summary",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err == nil {
		t.Fatal("expected error for missing target_branch")
	}
}

// TestBranchMRSummary_EmptyResult verifies BranchMRSummary when empty result.
func TestBranchMRSummary_EmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "branch_mr_summary",
		Arguments: map[string]string{
			"project_id":    "mygroup/myproject",
			"target_branch": "main",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Error("expected empty result message")
	}
}

// project_activity_report.

// TestProjectActivityReport_EventBreakdown verifies ProjectActivityReport when event breakdown.
func TestProjectActivityReport_EventBreakdown(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/events", func(w http.ResponseWriter, r *http.Request) {
		events := []*gl.ProjectEvent{
			{ActionName: "pushed to", CreatedAt: now},
			{ActionName: "pushed to", CreatedAt: now},
			{ActionName: "opened", CreatedAt: now},
			{ActionName: "commented on", CreatedAt: now},
		}
		data, _ := json.Marshal(events)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "project_activity_report",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Events | 4") {
		t.Error("expected 4 events in summary")
	}
	if !strings.Contains(text, "pushed to | 2") {
		t.Error("expected 'pushed to' count of 2")
	}
	if !strings.Contains(text, "opened | 1") {
		t.Error("expected 'opened' count of 1")
	}
}

// TestProjectActivityReport_MissingProjectID verifies ProjectActivityReport when missing project ID.
func TestProjectActivityReport_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "project_activity_report",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// mr_discussion_health.

// TestMRDiscussionHealth_UnresolvedThreads verifies mr_discussion_health when unresolved threads.
func TestMRDiscussionHealth_UnresolvedThreads(t *testing.T) {
	created := time.Now().Add(-24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 10, Title: "Feature A", Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created},
			{IID: 20, Title: "Feature B", Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	// MR 10 has 2 resolvable threads: 1 resolved, 1 unresolved
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/10/discussions", func(w http.ResponseWriter, r *http.Request) {
		discussions := []*gl.Discussion{
			{ID: "d1", Notes: []*gl.Note{
				{Resolvable: true, Resolved: true},
			}},
			{ID: "d2", Notes: []*gl.Note{
				{Resolvable: true, Resolved: false},
			}},
		}
		data, _ := json.Marshal(discussions)
		respondJSON(w, http.StatusOK, string(data))
	})

	// MR 20 has 1 resolvable thread: all unresolved
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/20/discussions", func(w http.ResponseWriter, r *http.Request) {
		discussions := []*gl.Discussion{
			{ID: "d3", Notes: []*gl.Note{
				{Resolvable: true, Resolved: false},
				{Resolvable: false},
			}},
		}
		data, _ := json.Marshal(discussions)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "mr_discussion_health",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "MRs with unresolved threads | 2") {
		t.Error("expected 2 MRs with unresolved threads")
	}
	if !strings.Contains(text, "Total unresolved threads | 2") {
		t.Error("expected 2 total unresolved threads")
	}
}

// TestMRDiscussionHealth_NoOpenMRs verifies MRDiscussionHealth when no open MRs.
func TestMRDiscussionHealth_NoOpenMRs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "mr_discussion_health",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No open merge requests found") {
		t.Error("expected empty result message")
	}
}

// TestMRDiscussionHealth_MissingProjectID verifies MRDiscussionHealth when missing project ID.
func TestMRDiscussionHealth_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "mr_discussion_health",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// unassigned_items.

// TestUnassignedItems_FindsUnownedItems verifies UnassignedItems when finds unowned items.
func TestUnassignedItems_FindsUnownedItems(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "Orphan MR", SourceBranch: "fix/orphan", TargetBranch: "main",
				CreatedAt: &created,
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 5, Title: "Bug without owner", State: "opened", CreatedAt: &created, WebURL: "http://example.com/issues/5"},
			{IID: 6, Title: "Task without owner", State: "opened", CreatedAt: &created, WebURL: "http://example.com/issues/6"},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "unassigned_items",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Unassigned MRs | 1") {
		t.Error("expected 1 unassigned MR")
	}
	if !strings.Contains(text, "Unassigned issues | 2") {
		t.Error("expected 2 unassigned issues")
	}
}

// TestUnassignedItems_AllAssigned verifies UnassignedItems when all assigned.
func TestUnassignedItems_AllAssigned(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "unassigned_items",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "All open items have assignees") {
		t.Error("expected all-assigned message")
	}
}

// TestUnassignedItems_MissingProjectID verifies UnassignedItems when missing project ID.
func TestUnassignedItems_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "unassigned_items",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// stale_items_report.

// TestStaleItemsReport_FindsStaleItems verifies StaleItemsReport when finds stale items.
func TestStaleItemsReport_FindsStaleItems(t *testing.T) {
	staleDate := time.Now().Add(-30 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{
				IID: 1, Title: "Old MR", SourceBranch: "feat/old", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &staleDate,
			},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 10, Title: "Forgotten issue", State: "opened", CreatedAt: &staleDate, WebURL: "http://example.com/issues/10"},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "stale_items_report",
		Arguments: map[string]string{"project_id": "mygroup/myproject", "stale_days": "14"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Stale MRs | 1") {
		t.Error("expected 1 stale MR")
	}
	if !strings.Contains(text, "Stale issues | 1") {
		t.Error("expected 1 stale issue")
	}
	if !strings.Contains(text, "14+ days") {
		t.Error("expected 14 days in header")
	}
}

// TestStaleItemsReport_NoStaleItems verifies StaleItemsReport when no stale items.
func TestStaleItemsReport_NoStaleItems(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "stale_items_report",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No stale items found") {
		t.Error("expected no-stale-items message")
	}
}

// TestStaleItemsReport_MissingProjectID verifies StaleItemsReport when missing project ID.
func TestStaleItemsReport_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "stale_items_report",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestBranchMRSummary_APIError_ReturnsError verifies that branch_mr_summary returns an
// error when the MR list API fails.
func TestBranchMRSummary_APIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "branch_mr_summary",
		map[string]string{"project_id": "42", "target_branch": "main"})
}

// TestProjectActivityReport_EventsAPIErrorWithMergedMRs_RendersMergedMRsWithZeroEvents verifies that
// project_activity_report tolerates a failing events API (warn-and-continue)
// and still renders the recently-merged MRs section when merged MRs exist.
func TestProjectActivityReport_EventsAPIErrorWithMergedMRs_RendersMergedMRsWithZeroEvents(t *testing.T) {
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

// TestMRDiscussionHealth_APIError_ReturnsError verifies that mr_discussion_health returns
// an error when the MR list API fails.
func TestMRDiscussionHealth_APIError_ReturnsError(t *testing.T) {
	getPromptExpectError(t, notFoundHandler(), "mr_discussion_health", map[string]string{"project_id": "42"})
}

// TestMRDiscussionHealth_DiscussionsAPIError_StillRendersReport verifies that a failing
// discussion API for a single MR yields a zero-thread row instead of failing
// the whole prompt (warn-and-continue branch).
func TestMRDiscussionHealth_DiscussionsAPIError_StillRendersReport(t *testing.T) {
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

// TestProjectActivityReport_WritesTheSectionsItsDescriptionPromises verifies
// that the report carries the contributor breakdown and the daily activity
// chart its catalog description advertises.
//
// The description has always said "Shows daily activity chart and contributor
// breakdown" and the handler wrote neither. A description is what a model reads
// to choose a prompt, so the promise is part of the surface; both sections come
// out of the project events the handler already fetches, so keeping the promise
// costs no request.
func TestProjectActivityReport_WritesTheSectionsItsDescriptionPromises(t *testing.T) {
	day := time.Now().UTC()
	stamp := day.Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/events", func(w http.ResponseWriter, _ *http.Request) {
		events := []*gl.ProjectEvent{
			{ActionName: "pushed to", CreatedAt: stamp, AuthorUsername: "alice"},
			{ActionName: "opened", CreatedAt: stamp, AuthorUsername: "alice"},
			{ActionName: "opened", CreatedAt: stamp, AuthorUsername: "bob"},
			// GitLab sends the username inside the author object as well, and a
			// system event carries neither.
			{ActionName: "closed", CreatedAt: stamp, Author: gl.BasicUser{Username: "carol"}},
			{ActionName: "closed", CreatedAt: stamp},
		}
		data, _ := json.Marshal(events)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(pathIssues, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "project_activity_report", map[string]string{"project_id": "42"})

	for _, row := range []string{
		"## Contributors\n\n| Contributor | Events |\n|-------------|--------|\n",
		"| alice | 2 |\n",
		"| bob | 1 |\n",
		"| carol | 1 |\n",
		"| unknown | 1 |\n",
	} {
		t.Run("contributor breakdown: "+row, func(t *testing.T) {
			if !strings.Contains(text, row) {
				t.Errorf("the contributor breakdown is missing %q:\n%s", row, text)
			}
		})
	}

	t.Run("daily activity chart", func(t *testing.T) {
		want := "## Daily Activity\n\n```mermaid\nxychart-beta\n  title \"Daily Activity\"\n  x-axis [\"" +
			day.Format("01-02") + "\"]\n  y-axis \"Events\"\n  bar [5]\n```\n"
		if !strings.Contains(text, want) {
			t.Errorf("the daily activity chart is missing or differs:\nwant %q\ngot\n%s", want, text)
		}
	})
}

// TestUnassignedItems_NothingToReport_ClosesWithARuleNotASetextHeading
// verifies that a report whose last section is a sentence still ends with a
// thematic break.
//
// "---" written directly under a line of text is a setext heading in
// CommonMark: the rule is not drawn and the sentence above it becomes an H2, so
// "All open items have assignees. Great job!" was rendered as a heading and the
// closing instruction ran on from it. The reports with nothing to report were
// the only ones that reached this, which is why it survived.
func TestUnassignedItems_NothingToReport_ClosesWithARuleNotASetextHeading(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(pathIssues, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "unassigned_items", map[string]string{"project_id": "42"})

	want := "All open items have assignees. Great job!\n\n---\nPlease identify the most critical unassigned items"
	if !strings.Contains(text, want) {
		t.Errorf("the closing rule does not follow a blank line:\nwant %q\ngot\n%s", want, text)
	}
	if strings.Contains(text, "Great job!\n---") {
		t.Errorf("the closing rule still reads as a setext heading of the sentence above it:\n%s", text)
	}
}

// TestStaleItemsReport_NothingToReport_ClosesWithARuleNotASetextHeading
// verifies the same separation for the stale-items report, whose empty result
// is also a sentence.
func TestStaleItemsReport_NothingToReport_ClosesWithARuleNotASetextHeading(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMRs, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc(pathIssues, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "stale_items_report", map[string]string{"project_id": "42"})

	want := "No stale items found. The project is well-maintained!\n\n---\nPlease analyze stale items"
	if !strings.Contains(text, want) {
		t.Errorf("the closing rule does not follow a blank line:\nwant %q\ngot\n%s", want, text)
	}
}

// TestProjectActivityReport_AQuietPeriod_WritesNoneOfTheOptionalSections
// verifies that each section of the activity report is written only when it has
// something to report.
//
// All three are `len(...) > 0` guards no test ever saw false, so each could be
// read as `>= 0`: a project with no activity would publish an empty event
// breakdown, an empty "Recently Merged MRs" heading and a Mermaid chart with no
// bars. The summary table above them already carries the zeroes, so what these
// headings add is the impression that something was withheld.
func TestProjectActivityReport_AQuietPeriod_WritesNoneOfTheOptionalSections(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/events", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "project_activity_report", map[string]string{"project_id": "42"})

	if !strings.Contains(text, "| Events | 0 |") {
		t.Errorf("expected the summary row:\n%s", text)
	}
	for _, unwanted := range []string{"## Event Breakdown", "## Contributors", "## Recently Merged MRs", "## Daily Activity"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("nothing happened, yet the report wrote %q:\n%s", unwanted, text)
		}
	}
}

// TestProjectEventDays_ReadsEachTimestampGitLabSendsAndSkipsWhatItCannot pins
// what the activity chart does with the string a project event carries.
//
// A project event stamps its time as a string rather than as a time, and GitLab
// has sent both a full instant and a bare date for it. Only the instant was
// ever tested, so the fallback layout was dead and an event whose stamp parses
// as neither would have been counted under a day with no name.
func TestProjectEventDays_ReadsEachTimestampGitLabSendsAndSkipsWhatItCannot(t *testing.T) {
	days := projectEventDays([]*gl.ProjectEvent{
		{CreatedAt: "2025-01-02T09:00:00Z"},
		{CreatedAt: "2025-01-02"},
		{CreatedAt: "2025-01-03T09:00:00Z"},
		{CreatedAt: "last Tuesday"},
		{CreatedAt: ""},
	})

	if len(days) != 2 {
		t.Fatalf("grouped into %d day(s), want 2: %+v", len(days), days)
	}
	if days[0].date != "2025-01-02" || days[0].count != 2 {
		t.Errorf("first day = %+v, want two events on 2025-01-02", days[0])
	}
	if days[1].date != "2025-01-03" || days[1].count != 1 {
		t.Errorf("second day = %+v, want one event on 2025-01-03", days[1])
	}
}

// TestMRDiscussionHealth_ThreadsAndUnresolvedThreads_AreCountedSeparately pins
// the two per-merge-request counters and the summary row over them.
//
// The thread counter is an increment the existing fixture never drove past one,
// and the "MRs with unresolved threads" row is a `> 0` on a count that was
// never zero for one merge request and non-zero for another in the same run, so
// the summary could count every merge request that has any thread at all. The
// whole point of the report is to name the ones still waiting on somebody.
func TestMRDiscussionHealth_ThreadsAndUnresolvedThreadsAreCountedSeparately(t *testing.T) {
	mux := http.NewServeMux()
	// The third merge request carries no author, which is what GitLab sends for
	// one opened by an account since deleted: the author cell is then empty
	// rather than the handler failing on a nil pointer.
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[
			{"iid":1,"title":"Still discussed","author":{"username":"alice"}},
			{"iid":2,"title":"Settled","author":{"username":"bob"}},
			{"iid":3,"title":"Opened by a deleted account"}
		]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/1/discussions", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":"d1","notes":[
			{"id":1,"resolvable":true,"resolved":false},
			{"id":2,"resolvable":true,"resolved":true},
			{"id":3,"resolvable":false,"resolved":false}
		]}]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests/{iid}/discussions", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":"d2","notes":[{"id":4,"resolvable":true,"resolved":true}]}]`)
	})

	text := getPromptText(t, mux, "mr_discussion_health", map[string]string{"project_id": "42"})

	for _, want := range []string{
		"| MRs with unresolved threads | 1 |",
		"| Total unresolved threads | 1 |",
		"| !1 | Still discussed | @alice | 2 | 1 |",
		"| !2 | Settled | @bob | 1 | 0 |",
		"| !3 | Opened by a deleted account | @ | 1 | 0 |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}
}

// TestUnassignedItems_TheTwoSections_AppearOnlyForWhatIsActuallyUnassigned
// verifies the three conditions that decide what this report says.
//
// Each section is a `len(...) > 0` and the congratulation at the end is an
// `&&` over both counts; every fixture so far had them agree, so the sections
// could be written empty and the "All open items have assignees" line could be
// printed beside a list of unassigned merge requests — which is the one
// sentence in the report a reader would act on.
func TestUnassignedItems_TheTwoSectionsAppearOnlyForWhatIsActuallyUnassigned(t *testing.T) {
	created := time.Now().Add(-24 * time.Hour)
	unassignedMR := `[{"iid":1,"title":"Nobody owns this","source_branch":"a","target_branch":"main",` +
		`"author":{"username":"alice"},"created_at":"` + created.UTC().Format(time.RFC3339) + `"}]`

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, unassignedMR)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "unassigned_items", map[string]string{"project_id": "42"})

	if !strings.Contains(text, "## Unassigned Merge Requests") {
		t.Errorf("expected the merge request section:\n%s", text)
	}
	if strings.Contains(text, "## Unassigned Issues") {
		t.Errorf("no issue is unassigned, yet the section was written:\n%s", text)
	}
	if strings.Contains(text, "All open items have assignees") {
		t.Errorf("a merge request is unassigned, yet the report congratulated the project:\n%s", text)
	}

	t.Run("an unassigned issue and no unassigned MR writes only the issue section", func(t *testing.T) {
		issuesOnly := http.NewServeMux()
		issuesOnly.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		issuesOnly.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[{"iid":5,"title":"Nobody owns this either","created_at":"`+
				created.UTC().Format(time.RFC3339)+`"}]`)
		})

		got := getPromptText(t, issuesOnly, "unassigned_items", map[string]string{"project_id": "42"})
		if !strings.Contains(got, "## Unassigned Issues") {
			t.Errorf("expected the issue section:\n%s", got)
		}
		if strings.Contains(got, "## Unassigned Merge Requests") {
			t.Errorf("no merge request is unassigned:\n%s", got)
		}
		if strings.Contains(got, "All open items have assignees") {
			t.Errorf("an issue is unassigned, yet the report congratulated the project:\n%s", got)
		}
	})

	t.Run("everything assigned writes neither section", func(t *testing.T) {
		empty := http.NewServeMux()
		empty.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		empty.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})

		quiet := getPromptText(t, empty, "unassigned_items", map[string]string{"project_id": "42"})
		if !strings.Contains(quiet, "All open items have assignees") {
			t.Errorf("expected the congratulation:\n%s", quiet)
		}
		for _, unwanted := range []string{"## Unassigned Merge Requests", "## Unassigned Issues"} {
			if strings.Contains(quiet, unwanted) {
				t.Errorf("nothing is unassigned, yet the report wrote %q:\n%s", unwanted, quiet)
			}
		}
	})
}

// TestStaleItemsReport_TheStaleWindowAndItsSections pins the date this report
// asks GitLab for and the two sections it writes from the answer.
//
// The cutoff is `AddDate(0, 0, -staleDays)` and reaches GitLab only as a query
// parameter nothing has read, so without its minus the report asks for items
// untouched since a date in the future and finds everything stale. The sections
// and the closing congratulation are the same shape as the unassigned report's
// and were free in the same way.
func TestStaleItemsReport_TheStaleWindowAndItsSections(t *testing.T) {
	updated := time.Now().Add(-40 * 24 * time.Hour)
	staleMR := `[{"iid":1,"title":"Forgotten","source_branch":"a","target_branch":"main",` +
		`"author":{"username":"alice"},"created_at":"` + updated.UTC().Format(time.RFC3339) + `"}]`

	var updatedBefore atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		updatedBefore.Store(r.URL.Query().Get("updated_before"))
		respondJSON(w, http.StatusOK, staleMR)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "stale_items_report",
		map[string]string{"project_id": "42", "stale_days": "14"})

	t.Run("the cutoff is the asked number of days in the past", func(t *testing.T) {
		raw, _ := updatedBefore.Load().(string)
		if raw == "" {
			t.Fatal("the listing carried no updated_before at all")
		}
		asked, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatalf("updated_before %q does not parse: %v", raw, err)
		}
		want := time.Now().UTC().AddDate(0, 0, -14)
		if asked.After(want.Add(time.Minute)) || asked.Before(want.Add(-time.Minute)) {
			t.Errorf("updated_before = %s, want about %s (fourteen days ago)", asked.UTC(), want)
		}
	})

	t.Run("only the section with something in it is written", func(t *testing.T) {
		if !strings.Contains(text, "## Stale Merge Requests") {
			t.Errorf("expected the merge request section:\n%s", text)
		}
		if strings.Contains(text, "## Stale Issues") {
			t.Errorf("no issue is stale, yet the section was written:\n%s", text)
		}
		if strings.Contains(text, "No stale items found") {
			t.Errorf("a merge request is stale, yet the report said there were none:\n%s", text)
		}
	})
}

// TestStaleItemsReport_EachSection_FollowsItsOwnKind verifies that the stale
// report writes a section only for the kind of item that has one, and keeps its
// all-clear sentence for the case where neither does.
//
// Split from the window test above so each reads as one question; between them
// they take every side of the two `len(...) > 0` guards and of the `&&` over
// the two counts, which is what separates "nothing is stale" from "the merge
// requests are fine and the issues are not".
func TestStaleItemsReport_EachSectionFollowsItsOwnKind(t *testing.T) {
	updated := time.Now().Add(-40 * 24 * time.Hour)
	staleIssue := `[{"iid":6,"title":"Forgotten issue","created_at":"` + updated.UTC().Format(time.RFC3339) + `"}]`

	t.Run("a stale issue and no stale MR writes only the issue section", func(t *testing.T) {
		issuesOnly := http.NewServeMux()
		issuesOnly.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		issuesOnly.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, staleIssue)
		})

		got := getPromptText(t, issuesOnly, "stale_items_report", map[string]string{"project_id": "42"})
		if !strings.Contains(got, "## Stale Issues") {
			t.Errorf("expected the issue section:\n%s", got)
		}
		if strings.Contains(got, "## Stale Merge Requests") {
			t.Errorf("no merge request is stale:\n%s", got)
		}
		if strings.Contains(got, "No stale items found") {
			t.Errorf("an issue is stale, yet the report said there were none:\n%s", got)
		}
	})

	t.Run("nothing stale writes neither section", func(t *testing.T) {
		empty := http.NewServeMux()
		empty.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})
		empty.HandleFunc("GET /api/v4/projects/{project}/issues", func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, `[]`)
		})

		quiet := getPromptText(t, empty, "stale_items_report", map[string]string{"project_id": "42"})
		if !strings.Contains(quiet, "No stale items found") {
			t.Errorf("expected the all-clear sentence:\n%s", quiet)
		}
		for _, unwanted := range []string{"## Stale Merge Requests", "## Stale Issues"} {
			if strings.Contains(quiet, unwanted) {
				t.Errorf("nothing is stale, yet the report wrote %q:\n%s", unwanted, quiet)
			}
		}
	})
}

// prompt_milestone_label.go edge branch.

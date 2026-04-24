// prompt_project_reports_test.go contains unit tests for project report MCP prompts.

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

// branch_mr_summary.

// TestBranchMRSummary_ListsMRsByBranch verifies that BranchMRSummary handles the lists m rs by branch scenario correctly.
func TestBranchMRSummary_ListsMRsByBranch(t *testing.T) {
	created := time.Now().Add(-2 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("target_branch") != "release/1.0" {
			t.Errorf("expected target_branch=release/1.0, got %s", r.URL.Query().Get("target_branch"))
		}
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Fix auth", SourceBranch: "fix/auth", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &created, Draft: true},
			{IID: 2, Title: "Add cache", SourceBranch: "feat/cache", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "bob"}, CreatedAt: &created, HasConflicts: true},
			{IID: 3, Title: "Update docs", SourceBranch: "docs/update", TargetBranch: "release/1.0",
				Author: &gl.BasicUser{Username: "charlie"}, CreatedAt: &created},
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

// TestBranchMRSummary_MissingProjectID verifies that BranchMRSummary handles the missing project i d scenario correctly.
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

// TestBranchMRSummary_MissingTargetBranch verifies that BranchMRSummary handles the missing target branch scenario correctly.
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

// TestBranchMRSummary_EmptyResult verifies that BranchMRSummary handles the empty result scenario correctly.
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

// TestProjectActivityReport_EventBreakdown verifies that ProjectActivityReport handles the event breakdown scenario correctly.
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

// TestProjectActivityReport_MissingProjectID verifies that ProjectActivityReport handles the missing project i d scenario correctly.
func TestProjectActivityReport_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "project_activity_report",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// mr_review_status.

// TestMRReviewStatus_UnresolvedThreads verifies that MRReviewStatus handles the unresolved threads scenario correctly.
func TestMRReviewStatus_UnresolvedThreads(t *testing.T) {
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
		Name:      "mr_review_status",
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

// TestMRReviewStatus_NoOpenMRs verifies that MRReviewStatus handles the no open m rs scenario correctly.
func TestMRReviewStatus_NoOpenMRs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "mr_review_status",
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

// TestMRReviewStatus_MissingProjectID verifies that MRReviewStatus handles the missing project i d scenario correctly.
func TestMRReviewStatus_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "mr_review_status",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// unassigned_items.

// TestUnassignedItems_FindsUnownedItems verifies that UnassignedItems handles the finds unowned items scenario correctly.
func TestUnassignedItems_FindsUnownedItems(t *testing.T) {
	created := time.Now().Add(-48 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Orphan MR", SourceBranch: "fix/orphan", TargetBranch: "main",
				CreatedAt: &created},
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

// TestUnassignedItems_AllAssigned verifies that UnassignedItems handles the all assigned scenario correctly.
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

// TestUnassignedItems_MissingProjectID verifies that UnassignedItems handles the missing project i d scenario correctly.
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

// TestStaleItemsReport_FindsStaleItems verifies that StaleItemsReport handles the finds stale items scenario correctly.
func TestStaleItemsReport_FindsStaleItems(t *testing.T) {
	staleDate := time.Now().Add(-30 * 24 * time.Hour)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 1, Title: "Old MR", SourceBranch: "feat/old", TargetBranch: "main",
				Author: &gl.BasicUser{Username: "alice"}, CreatedAt: &staleDate},
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

// TestStaleItemsReport_NoStaleItems verifies that StaleItemsReport handles the no stale items scenario correctly.
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

// TestStaleItemsReport_MissingProjectID verifies that StaleItemsReport handles the missing project i d scenario correctly.
func TestStaleItemsReport_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: "stale_items_report",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

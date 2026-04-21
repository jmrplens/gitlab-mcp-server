// prompt_milestone_label_test.go contains unit tests for milestone and label MCP prompts.
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

// milestone_progress.

// TestMilestoneProgress_WithMilestones verifies that MilestoneProgress handles the with milestones scenario correctly.
func TestMilestoneProgress_WithMilestones(t *testing.T) {
	dueDate := gl.ISOTime(time.Now().Add(10 * 24 * time.Hour))
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/milestones", func(w http.ResponseWriter, r *http.Request) {
		milestones := []*gl.Milestone{
			{ID: 1, Title: "v1.0", State: "active", DueDate: &dueDate},
			{ID: 2, Title: "v2.0", State: "active"},
		}
		data, _ := json.Marshal(milestones)
		respondJSON(w, http.StatusOK, string(data))
	})

	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 1, Title: "Issue A", State: "closed"},
			{IID: 2, Title: "Issue B", State: "opened"},
			{IID: 3, Title: "Issue C", State: "closed"},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 10, Title: "MR A", State: "merged"},
			{IID: 11, Title: "MR B", State: "opened"},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "milestone_progress",
		Arguments: map[string]string{"project_id": "mygroup/myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "v1.0") {
		t.Error("expected milestone title v1.0")
	}
	if !strings.Contains(text, "v2.0") {
		t.Error("expected milestone title v2.0")
	}
	if !strings.Contains(text, "Closed issues | 2") {
		t.Error("expected 2 closed issues")
	}
	if !strings.Contains(text, "Open issues | 1") {
		t.Error("expected 1 open issue")
	}
	if !strings.Contains(text, "Merged MRs | 1") {
		t.Error("expected 1 merged MR")
	}
	if !strings.Contains(text, "days remaining") {
		t.Error("expected due date with days remaining")
	}
	if !strings.Contains(text, "█") {
		t.Error("expected progress bar")
	}
}

// TestMilestoneProgress_SpecificMilestone verifies that MilestoneProgress handles the specific milestone scenario correctly.
func TestMilestoneProgress_SpecificMilestone(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("title") != "v1.0" {
			t.Errorf("expected title filter v1.0, got %q", r.URL.Query().Get("title"))
		}
		milestones := []*gl.Milestone{{ID: 1, Title: "v1.0", State: "active"}}
		data, _ := json.Marshal(milestones)
		respondJSON(w, http.StatusOK, string(data))
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/issues", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "milestone_progress",
		Arguments: map[string]string{"project_id": "myproject", "milestone": "v1.0"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "v1.0") {
		t.Error("expected milestone title v1.0")
	}
}

// TestMilestoneProgress_EmptyMilestones verifies that MilestoneProgress handles the empty milestones scenario correctly.
func TestMilestoneProgress_EmptyMilestones(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "milestone_progress",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No active milestones found") {
		t.Error("expected empty milestone message")
	}
}

// TestMilestoneProgress_RequiresProjectID verifies that MilestoneProgress handles the requires project i d scenario correctly.
func TestMilestoneProgress_RequiresProjectID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "milestone_progress",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// label_distribution.

// TestLabelDistribution_WithLabels verifies that LabelDistribution handles the with labels scenario correctly.
func TestLabelDistribution_WithLabels(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, r *http.Request) {
		labels := []*gl.Label{
			{Name: "bug", OpenIssuesCount: 10, ClosedIssuesCount: 5, OpenMergeRequestsCount: 2},
			{Name: "feature", OpenIssuesCount: 8, ClosedIssuesCount: 3, OpenMergeRequestsCount: 4},
			{Name: "unused", OpenIssuesCount: 0, ClosedIssuesCount: 0, OpenMergeRequestsCount: 0},
		}
		data, _ := json.Marshal(labels)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "label_distribution",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "bug") {
		t.Error("expected label 'bug'")
	}
	if !strings.Contains(text, "feature") {
		t.Error("expected label 'feature'")
	}
	// unused label (all zeros) should be skipped
	if strings.Contains(text, "| unused |") {
		t.Error("unused label with zero counts should be excluded from table")
	}
	if !strings.Contains(text, "pie title Open Issues by Label") {
		t.Error("expected Mermaid pie chart")
	}
	if !strings.Contains(text, "**Total**") {
		t.Error("expected total row")
	}
}

// TestLabelDistribution_EmptyLabels verifies that LabelDistribution handles the empty labels scenario correctly.
func TestLabelDistribution_EmptyLabels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "label_distribution",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No labels found") {
		t.Error("expected empty labels message")
	}
}

// TestLabelDistribution_RequiresProjectID verifies that LabelDistribution handles the requires project i d scenario correctly.
func TestLabelDistribution_RequiresProjectID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "label_distribution",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// group_milestone_progress.

// TestGroupMilestoneProgress_WithMilestones verifies that GroupMilestoneProgress handles the with milestones scenario correctly.
func TestGroupMilestoneProgress_WithMilestones(t *testing.T) {
	dueDate := gl.ISOTime(time.Now().Add(-5 * 24 * time.Hour))
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/groups/{group}/milestones", func(w http.ResponseWriter, r *http.Request) {
		milestones := []*gl.GroupMilestone{
			{ID: 10, Title: "Sprint-1", State: "active", DueDate: &dueDate},
		}
		data, _ := json.Marshal(milestones)
		respondJSON(w, http.StatusOK, string(data))
	})

	mux.HandleFunc("GET /api/v4/groups/{group}/milestones/{milestone}/issues", func(w http.ResponseWriter, r *http.Request) {
		issues := []*gl.Issue{
			{IID: 1, Title: "Issue X", State: "closed"},
			{IID: 2, Title: "Issue Y", State: "opened"},
		}
		data, _ := json.Marshal(issues)
		respondJSON(w, http.StatusOK, string(data))
	})

	mux.HandleFunc("GET /api/v4/groups/{group}/milestones/{milestone}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		mrs := []*gl.BasicMergeRequest{
			{IID: 5, Title: "MR1", State: "merged"},
		}
		data, _ := json.Marshal(mrs)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_milestone_progress",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Sprint-1") {
		t.Error("expected milestone title Sprint-1")
	}
	if !strings.Contains(text, "Issues (closed/total) | 1/2") {
		t.Error("expected issues closed/total 1/2")
	}
	if !strings.Contains(text, "MRs (merged/total) | 1/1") {
		t.Error("expected MRs merged/total 1/1")
	}
	if !strings.Contains(text, "days overdue") {
		t.Error("expected overdue message for past due date")
	}
	if !strings.Contains(text, "█") {
		t.Error("expected progress bar")
	}
}

// TestGroupMilestoneProgress_EmptyMilestones verifies that GroupMilestoneProgress handles the empty milestones scenario correctly.
func TestGroupMilestoneProgress_EmptyMilestones(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/{group}/milestones", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_milestone_progress",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No active group milestones found") {
		t.Error("expected empty milestones message")
	}
}

// TestGroupMilestoneProgress_RequiresGroupID verifies that GroupMilestoneProgress handles the requires group i d scenario correctly.
func TestGroupMilestoneProgress_RequiresGroupID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_milestone_progress",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// project_contributors.

// TestProjectContributors_WithContributors verifies that ProjectContributors handles the with contributors scenario correctly.
func TestProjectContributors_WithContributors(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/{project}/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
		contributors := []*gl.Contributor{
			{Name: "Alice", Email: "alice@example.com", Commits: 50, Additions: 5000, Deletions: 1000},
			{Name: "Bob", Email: "bob@example.com", Commits: 20, Additions: 2000, Deletions: 500},
			{Name: "Charlie", Email: "charlie@example.com", Commits: 5, Additions: 300, Deletions: 100},
		}
		data, _ := json.Marshal(contributors)
		respondJSON(w, http.StatusOK, string(data))
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "project_contributors",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Alice") {
		t.Error("expected contributor Alice")
	}
	if !strings.Contains(text, "Bob") {
		t.Error("expected contributor Bob")
	}
	if !strings.Contains(text, "3 contributors") {
		t.Error("expected contributor count")
	}
	if !strings.Contains(text, "**Total**") {
		t.Error("expected total row")
	}
	if !strings.Contains(text, "pie title Commits by Contributor") {
		t.Error("expected Mermaid pie chart")
	}
	// Verify totals
	if !strings.Contains(text, "**75**") {
		t.Error("expected total commits 75")
	}
}

// TestProjectContributors_EmptyContributors verifies that ProjectContributors handles the empty contributors scenario correctly.
func TestProjectContributors_EmptyContributors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "project_contributors",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No contributors found") {
		t.Error("expected empty contributors message")
	}
}

// TestProjectContributors_RequiresProjectID verifies that ProjectContributors handles the requires project i d scenario correctly.
func TestProjectContributors_RequiresProjectID(t *testing.T) {
	mux := http.NewServeMux()
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "project_contributors",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

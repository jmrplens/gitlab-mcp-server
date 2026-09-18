// prompt_milestone_label_test.go contains unit tests for milestone and label MCP prompts.
package prompts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// milestone_progress.

// TestMilestoneProgress_WithMilestones verifies MilestoneProgress when with milestones.
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

// TestMilestoneProgress_SpecificMilestone verifies MilestoneProgress when specific milestone.
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

// TestMilestoneProgress_EmptyMilestones verifies MilestoneProgress when empty milestones.
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

// TestMilestoneProgress_RequiresProjectID verifies MilestoneProgress requires project ID.
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

// TestLabelDistribution_WithLabels verifies LabelDistribution when with labels.
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

// TestLabelDistribution_EmptyLabels verifies LabelDistribution when empty labels.
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

// TestLabelDistribution_RequiresProjectID verifies LabelDistribution requires project ID.
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

// TestGroupMilestoneProgress_WithMilestones verifies GroupMilestoneProgress when with milestones.
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

// TestGroupMilestoneProgress_EmptyMilestones verifies GroupMilestoneProgress when empty milestones.
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

// TestGroupMilestoneProgress_RequiresGroupID verifies GroupMilestoneProgress requires group ID.
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

// TestProjectContributors_WithContributors verifies ProjectContributors when with contributors.
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

// TestProjectContributors_EmptyContributors verifies ProjectContributors when empty contributors.
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

// TestProjectContributors_RequiresProjectID verifies ProjectContributors requires project ID.
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

// TestMilestoneProgress_APIError verifies that handleMilestoneProgress
// wraps and propagates errors from ListMilestones (covers the
// `if err != nil` branch at prompt_milestone_label.go:100).
func TestMilestoneProgress_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "milestone_progress",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err == nil {
		t.Fatal("expected error when GitLab API returns 403")
	}
	if !strings.Contains(err.Error(), "milestone_progress") {
		t.Errorf("expected wrapped 'milestone_progress' error, got: %v", err)
	}
}

// TestLabelDistribution_APIError verifies that handleLabelDistribution
// wraps and propagates errors from ListLabels (covers the
// `if err != nil` branch at prompt_milestone_label.go:172).
func TestLabelDistribution_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "label_distribution",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err == nil {
		t.Fatal("expected error when GitLab API returns 403")
	}
	if !strings.Contains(err.Error(), "label_distribution") {
		t.Errorf("expected wrapped 'label_distribution' error, got: %v", err)
	}
}

// TestGroupMilestoneProgress_APIError verifies that
// handleGroupMilestoneProgress wraps and propagates errors from
// ListGroupMilestones (covers the `if err != nil` branch at
// prompt_milestone_label.go:253).
func TestGroupMilestoneProgress_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/{group}/milestones", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "group_milestone_progress",
		Arguments: map[string]string{"group_id": "mygroup"},
	})
	if err == nil {
		t.Fatal("expected error when GitLab API returns 403")
	}
	if !strings.Contains(err.Error(), "group_milestone_progress") {
		t.Errorf("expected wrapped 'group_milestone_progress' error, got: %v", err)
	}
}

// TestProjectContributors_APIError verifies that handleProjectContributors
// wraps and propagates errors from Repositories.Contributors (covers the
// `if err != nil` branch at prompt_milestone_label.go:321).
func TestProjectContributors_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/repository/contributors", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	session := newMCPSession(t, mux)
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "project_contributors",
		Arguments: map[string]string{"project_id": "myproject"},
	})
	if err == nil {
		t.Fatal("expected error when GitLab API returns 403")
	}
	if !strings.Contains(err.Error(), "project_contributors") {
		t.Errorf("expected wrapped 'project_contributors' error, got: %v", err)
	}
}

// TestProjectContributors_PieChartCap_CapsPieChartSlices verifies that the contributor pie chart
// is capped at eight entries while the table still lists everyone.
func TestProjectContributors_PieChartCap_CapsPieChartSlices(t *testing.T) {
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

// TestProjectContributors_ContributorNameWithAQuote_StaysInsideTheChartString
// verifies that a name carrying a quotation mark cannot add entries to the pie
// chart it is rendered into.
//
// The entries were built with Go's %q, which writes a backslash Mermaid does
// not read and leaves the quote that closes the label. A commit author name is
// whatever a contributor put in their git config, so the quote is reachable by
// anybody who can land a commit; past it, the rest of the name is chart syntax.
func TestProjectContributors_ContributorNameWithAQuote_StaysInsideTheChartString(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/repository/contributors", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"name":"Al \"ice\" : 99\n    \"Evil","commits":4,"additions":1,"deletions":1}]`)
	})

	text := getPromptText(t, mux, "project_contributors", map[string]string{"project_id": "42"})

	want := "```mermaid\npie title Commits by Contributor\n    \"Al #quot;ice#quot; : 99     #quot;Evil\" : 4\n```\n"
	if !strings.Contains(text, want) {
		t.Errorf("the pie chart is not the contained one:\nwant %q\ngot\n%s", want, text)
	}
	if strings.Contains(text, `\"ice\"`) {
		t.Errorf("a Go string escape reached the Mermaid chart, where a backslash is literal:\n%s", text)
	}
}

// TestMilestoneProgress_DueDate_RendersInTheServerDisplayForm verifies that a
// milestone's due date is written by toolutil.FormatTime rather than by a
// layout of this package's own.
//
// A due date is a date GitLab sent, and a prompt has no more reason to spell it
// its own way than a card does; the escaping gate's bool-time rule names each
// hand-rolled layout for that reason.
func TestMilestoneProgress_DueDate_RendersInTheServerDisplayForm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeMilestones, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":1,"title":"v1.0","state":"active","due_date":"2099-03-21"}]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{id}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{id}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	})

	text := getPromptText(t, mux, "milestone_progress", map[string]string{"project_id": "42"})

	if !strings.Contains(text, "**Due Date**: 21 Mar 2099 (") {
		t.Errorf("the due date is not in the server's display form:\n%s", text)
	}
	if strings.Contains(text, "**Due Date**: 2099-03-21") {
		t.Errorf("the due date is still written with this package's own layout:\n%s", text)
	}
}

// TestCountIssueStatesAndCountMRStates_CountEachStateSeparately pins the four
// counters the milestone tables are built from.
//
// Both are increments in a two-way split, and every fixture so far gave a
// milestone one item of each kind, where an increment and a decrement differ
// only in sign and a row is present either way. A closed merge request is
// neither merged nor open and is the case that separates the switch from an
// else.
func TestCountIssueStatesAndCountMRStates_CountEachStateSeparately(t *testing.T) {
	t.Run("issues", func(t *testing.T) {
		closed, open := countIssueStates([]*gl.Issue{
			{IID: 1, State: "closed"},
			{IID: 2, State: "opened"},
			{IID: 3, State: "opened"},
		})
		if closed != 1 {
			t.Errorf("closed = %d, want 1", closed)
		}
		if open != 2 {
			t.Errorf("open = %d, want 2", open)
		}
	})

	t.Run("merge requests", func(t *testing.T) {
		merged, open := countMRStates([]*gl.BasicMergeRequest{
			{IID: 1, State: "merged"},
			{IID: 2, State: "opened"},
			{IID: 3, State: "opened"},
			{IID: 4, State: "closed"},
		})
		if merged != 1 {
			t.Errorf("merged = %d, want 1", merged)
		}
		if open != 2 {
			t.Errorf("open = %d, want 2 (a closed MR is neither merged nor open)", open)
		}
	})
}

// TestWriteDueDateSection_CountsTheDaysLeftAndTheDaysOverdue pins the three
// decisions this line is made of.
//
// The remaining days are an elapsed time divided by a day, the sign decides
// which of two sentences is written, and the overdue figure is the negation of
// a negative count. None was ever asserted with a value: the tests read the
// date beside it and stopped. A milestone reported as "119 days remaining" when
// it is five days overdue is the kind of thing a release plan is built on.
func TestWriteDueDateSection_CountsTheDaysLeftAndTheDaysOverdue(t *testing.T) {
	tests := []struct {
		name string
		due  *gl.ISOTime
		want string
		none bool
	}{
		{name: "no due date writes nothing", due: nil, none: true},
		{
			name: "five and a half days ahead is five days remaining",
			due:  isoTimePtr(time.Now().Add(5*24*time.Hour + 12*time.Hour)),
			want: "(5 days remaining)",
		},
		{
			name: "an hour ahead is still zero days remaining",
			due:  isoTimePtr(time.Now().Add(time.Hour)),
			want: "(0 days remaining)",
		},
		{
			name: "five and a half days past is five days overdue",
			due:  isoTimePtr(time.Now().Add(-5*24*time.Hour - 12*time.Hour)),
			want: "(**5 days overdue**)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeDueDateSection(&b, tt.due)
			got := b.String()
			if tt.none {
				if got != "" {
					t.Errorf("a milestone with no due date wrote %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("writeDueDateSection() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// isoTimePtr returns a pointer to the ISO time a fixture wants.
func isoTimePtr(at time.Time) *gl.ISOTime {
	iso := gl.ISOTime(at)
	return &iso
}

// TestMilestoneProgress_TheProgressBar_IsTheClosedShareOfEverythingInTheMilestone
// pins the two figures behind the bar.
//
// The total is the issues plus the merge requests and the completed count is
// the closed ones plus the merged ones; every fixture until now had enough
// zeroes in it that a sum and a difference agreed. The bar is the first thing
// in the section and is what a reader takes the milestone's state from.
func TestMilestoneProgress_TheProgressBarIsTheClosedShareOfTheMilestone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":7,"iid":1,"title":"v1.0","state":"active"}]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[
			{"iid":1,"state":"closed"},{"iid":2,"state":"closed"},{"iid":3,"state":"opened"}
		]`)
	})
	mux.HandleFunc("GET /api/v4/projects/{project}/milestones/{milestone}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":10,"state":"merged"}]`)
	})

	text := getPromptText(t, mux, "milestone_progress", map[string]string{"project_id": "42"})

	if !strings.Contains(text, "] 75%") {
		t.Errorf("three of four items are done, so the bar should read 75%%:\n%s", text)
	}
	for _, want := range []string{
		"| Total issues | 3 |",
		"| Closed issues | 2 |",
		"| Open issues | 1 |",
		"| Total MRs | 1 |",
		"| Merged MRs | 1 |",
		"| Open MRs | 0 |",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}
}

// TestGroupMilestoneProgress_TheProgressBar_CountsIssuesAndMergeRequestsTogether
// pins the same two figures for the group-level report.
//
// It is a second copy of the arithmetic, in a prompt that spans every project
// of a group, and it was as unasserted as the project one.
func TestGroupMilestoneProgress_TheProgressBarCountsIssuesAndMergeRequestsTogether(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/{group}/milestones", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":7,"iid":1,"title":"v1.0","state":"active"}]`)
	})
	mux.HandleFunc("GET /api/v4/groups/{group}/milestones/{milestone}/issues", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":1,"state":"closed"},{"iid":2,"state":"opened"},{"iid":3,"state":"opened"}]`)
	})
	mux.HandleFunc("GET /api/v4/groups/{group}/milestones/{milestone}/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"iid":10,"state":"merged"}]`)
	})

	text := getPromptText(t, mux, "group_milestone_progress", map[string]string{"group_id": "mygroup"})

	if !strings.Contains(text, "] 50%") {
		t.Errorf("two of four items are done, so the bar should read 50%%:\n%s", text)
	}
	if !strings.Contains(text, "| Issues (closed/total) | 1/3 |") {
		t.Errorf("expected one closed issue of three:\n%s", text)
	}
	if !strings.Contains(text, "| MRs (merged/total) | 1/1 |") {
		t.Errorf("expected one merged MR of one:\n%s", text)
	}
}

// TestLabelDistribution_TheOrderTheTotalsAndTheChart pins everything this
// report computes from the label counts GitLab sends.
//
// The ordering comparator adds three counts on each side, the per-row total
// adds the same three again and the footer adds those up once more; every
// fixture until now used labels whose counts were equal or zero, where a sum
// and a difference cannot be told apart and any order looks sorted. The chart
// is the other half: a label nobody has opened an issue under must not be a
// slice, and the chart is capped at eight so a project with a large taxonomy
// still gets a diagram Mermaid can draw.
func TestLabelDistribution_TheOrderTheTotalsAndTheChart(t *testing.T) {
	labels := []*gl.Label{
		{Name: "alpha", OpenIssuesCount: 1, ClosedIssuesCount: 9, OpenMergeRequestsCount: 0},
		{Name: "beta", OpenIssuesCount: 6, ClosedIssuesCount: 0, OpenMergeRequestsCount: 1},
		{Name: "gamma", OpenIssuesCount: 2, ClosedIssuesCount: 1, OpenMergeRequestsCount: 9},
		{Name: "unused", OpenIssuesCount: 0, ClosedIssuesCount: 0, OpenMergeRequestsCount: 0},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(labels)
		if err != nil {
			t.Errorf("marshaling the label fixture: %v", err)
			respondNotFound(w)
			return
		}
		respondJSON(w, http.StatusOK, string(data))
	})

	text := getPromptText(t, mux, "label_distribution", map[string]string{"project_id": "42"})

	t.Run("the rows are ordered by total usage", func(t *testing.T) {
		gamma := strings.Index(text, "| gamma |")
		alpha := strings.Index(text, "| alpha |")
		beta := strings.Index(text, "| beta |")
		if gamma < 0 || alpha < 0 || beta < 0 {
			t.Fatalf("a label is missing from the table:\n%s", text)
		}
		if gamma >= alpha || alpha >= beta {
			t.Errorf("expected gamma (12) then alpha (10) then beta (7):\n%s", text)
		}
	})

	t.Run("each row carries its own total", func(t *testing.T) {
		for _, want := range []string{
			"| gamma | 2 | 1 | 9 | 12 |",
			"| alpha | 1 | 9 | 0 | 10 |",
			"| beta | 6 | 0 | 1 | 7 |",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("expected %q in:\n%s", want, text)
			}
		}
	})

	t.Run("an unused label is left out of the table", func(t *testing.T) {
		if strings.Contains(text, "| unused |") {
			t.Errorf("a label with no issues or MRs should not have a row:\n%s", text)
		}
	})

	t.Run("the footer adds the columns up", func(t *testing.T) {
		if !strings.Contains(text, "| **Total** | **9** | **10** | **10** | **29** |") {
			t.Errorf("expected the column totals:\n%s", text)
		}
	})

	t.Run("only labels with open issues are charted", func(t *testing.T) {
		for _, want := range []string{`"gamma" : 2`, `"alpha" : 1`, `"beta" : 6`} {
			if !strings.Contains(text, want) {
				t.Errorf("expected %q in the chart:\n%s", want, text)
			}
		}
		if strings.Contains(text, `"unused"`) {
			t.Errorf("a label with no open issues should not be a slice:\n%s", text)
		}
	})
}

// TestLabelDistribution_TheChart_StopsAtEightSlices pins the cap on the pie.
//
// The cap is a `< 8` nothing ever reached, so it could be read as `<= 8` and a
// project with a wide taxonomy would get a ninth slice — which matters less for
// the diagram than for what the cap is: the one bound on how much of a
// GitLab-authored label list is copied into the model's instructions.
func TestLabelDistribution_TheChartStopsAtEightSlices(t *testing.T) {
	var labels []*gl.Label
	for i := 1; i <= 9; i++ {
		labels = append(labels, &gl.Label{
			Name:            fmt.Sprintf("label-%02d", i),
			OpenIssuesCount: int64(10 - i),
		})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(labels)
		if err != nil {
			t.Errorf("marshaling the label fixture: %v", err)
			respondNotFound(w)
			return
		}
		respondJSON(w, http.StatusOK, string(data))
	})

	text := getPromptText(t, mux, "label_distribution", map[string]string{"project_id": "42"})

	if got := strings.Count(text, " : "); got != 8 {
		t.Errorf("the chart carries %d slices, want 8:\n%s", got, text)
	}
	if strings.Contains(text, `"label-09" :`) {
		t.Errorf("the ninth label should be past the cap:\n%s", text)
	}
}

// TestLabelDistribution_NoLabelHasAnOpenIssue_HasNoChart verifies the chart is
// written only when there is a slice to draw.
//
// The guard is a `len(...) > 0` nothing saw false, so read as `>= 0` a project
// whose labels are all unused gets an empty "pie title" block: a fenced diagram
// with no data, which renders as an error rather than as the absence it is.
func TestLabelDistribution_NoLabelHasAnOpenIssue_HasNoChart(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/{project}/labels", func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[{"name":"stale","open_issues_count":0,"closed_issues_count":4,"open_merge_requests_count":0}]`)
	})

	text := getPromptText(t, mux, "label_distribution", map[string]string{"project_id": "42"})

	if !strings.Contains(text, "| stale | 0 | 4 | 0 | 4 |") {
		t.Errorf("expected the label row:\n%s", text)
	}
	if strings.Contains(text, "pie title Open Issues by Label") {
		t.Errorf("no label has an open issue, yet a chart was written:\n%s", text)
	}
}

// prompt_git_workflow.go error branches.

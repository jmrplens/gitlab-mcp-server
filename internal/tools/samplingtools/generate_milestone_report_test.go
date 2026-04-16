// generate_milestone_report_test.go contains unit tests for the samplingtools MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package samplingtools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatMilestoneForAnalysis verifies Markdown output from milestone data.
func TestFormatMilestoneForAnalysis(t *testing.T) {
	ms := milestones.Output{
		Title: "v1.0", State: "active", Description: "First release",
		StartDate: "2026-01-01", DueDate: "2026-03-01", Expired: false,
	}
	msIssues := milestones.MilestoneIssuesOutput{
		Issues: []milestones.IssueItem{
			{IID: 1, Title: "Bug fix", State: "closed"},
			{IID: 2, Title: "New feature", State: "opened"},
		},
	}
	msMRs := milestones.MilestoneMergeRequestsOutput{
		MergeRequests: []milestones.MergeRequestItem{
			{IID: 10, Title: "Fix bug", State: "merged", SourceBranch: "fix/bug", TargetBranch: "main"},
		},
	}
	result := FormatMilestoneForAnalysis(ms, msIssues, msMRs)
	checks := []struct {
		name, want string
	}{
		{"title", "# Milestone: v1.0"},
		{"state", "**State**: active"},
		{"description", "**Description**: First release"},
		{"start_date", "**Start Date**: 1 Jan 2026"},
		{"due_date", "**Due Date**: 1 Mar 2026"},
		{"expired", "**Expired**: false"},
		{"issues_section", "## Issues (2 total: 1 open, 1 closed)"},
		{"issue_entry", "#1 — Bug fix [closed]"},
		{"mrs_section", "## Merge Requests (1 total: 0 open, 1 merged)"},
		{"mr_entry", "!10 — Fix bug [merged] (fix/bug → main)"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatMilestoneForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatGenerateMilestoneReportMarkdown verifies milestone report rendering.
func TestFormatGenerateMilestoneReportMarkdown(t *testing.T) {
	r := GenerateMilestoneReportOutput{
		MilestoneIID: 5, Title: "v1.0",
		Report: "Milestone is on track", Model: "gpt-4o",
	}
	md := FormatGenerateMilestoneReportMarkdown(r)
	checks := []string{"## Milestone Report: v1.0", "Milestone is on track", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatGenerateMilestoneReportMarkdown missing %q", c)
		}
	}
}

// TestFormatGenerateMilestoneReportMarkdown_Truncated verifies truncation warning.
func TestFormatGenerateMilestoneReportMarkdown_Truncated(t *testing.T) {
	r := GenerateMilestoneReportOutput{Title: "x", Truncated: true}
	md := FormatGenerateMilestoneReportMarkdown(r)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestGenerateMilestoneReport_EmptyProjectID verifies project_id validation.
func TestGenerateMilestoneReport_EmptyProjectID(t *testing.T) {
	_, err := GenerateMilestoneReport(context.Background(), &mcp.CallToolRequest{}, nil, GenerateMilestoneReportInput{
		ProjectID: "", MilestoneIID: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestGenerateMilestoneReport_InvalidIID verifies milestone_iid validation.
func TestGenerateMilestoneReport_InvalidIID(t *testing.T) {
	_, err := GenerateMilestoneReport(context.Background(), &mcp.CallToolRequest{}, nil, GenerateMilestoneReportInput{
		ProjectID: "42", MilestoneIID: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "milestone_iid") {
		t.Errorf("error = %v, want milestone_iid validation error", err)
	}
}

// TestGenerateMilestoneReport_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestGenerateMilestoneReport_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := GenerateMilestoneReport(context.Background(), req, client, GenerateMilestoneReportInput{
		ProjectID: "42", MilestoneIID: 5,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestGenerateMilestoneReport_MilestoneNotFound verifies error wrapping on 404.
func TestGenerateMilestoneReport_MilestoneNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := GenerateMilestoneReport(ctx, req, client, GenerateMilestoneReportInput{
		ProjectID: "42", MilestoneIID: 999,
	})
	if err == nil || !strings.Contains(err.Error(), "fetching milestone") {
		t.Errorf("error = %v, want 'fetching milestone' context", err)
	}
}

// TestGenerateMilestoneReport_FullFlow verifies the complete milestone report flow.
// Milestones use resolveIID internally, so the mock must handle the IID lookup
// (ListMilestones with iids[] parameter) and then GetMilestone by global ID.
func TestGenerateMilestoneReport_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	// resolveIID: ListMilestones with iids[]=5 → returns milestone with global ID 999
	mux.HandleFunc("/api/v4/projects/42/milestones", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id": 999, "iid": 5, "title": "v1.0", "state": "active"}]`)
	})
	// GetMilestone by global ID
	mux.HandleFunc("/api/v4/projects/42/milestones/999", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 999, "iid": 5, "title": "v1.0", "state": "active",
			"description": "First release", "start_date": "2026-01-01", "due_date": "2026-03-01"
		}`)
	})
	// Milestone issues (also calls resolveIID, but /milestones matches above)
	mux.HandleFunc("/api/v4/projects/42/milestones/999/issues", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id": 1, "iid": 1, "title": "Bug fix", "state": "closed"}]`)
	})
	// Milestone MRs
	mux.HandleFunc("/api/v4/projects/42/milestones/999/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id": 10, "iid": 10, "title": "Fix bug", "state": "merged", "source_branch": "fix/bug", "target_branch": "main"}]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := GenerateMilestoneReport(ctx, req, client, GenerateMilestoneReportInput{
		ProjectID: "42", MilestoneIID: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MilestoneIID != 5 {
		t.Errorf("MilestoneIID = %d, want 5", out.MilestoneIID)
	}
	if out.Title != "v1.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.0")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Report == "" {
		t.Error("Report is empty")
	}
}

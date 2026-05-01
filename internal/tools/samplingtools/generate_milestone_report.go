package samplingtools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GenerateMilestoneReportInput defines parameters for LLM-assisted milestone reporting.
type GenerateMilestoneReportInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	MilestoneIID int64                `json:"milestone_iid" jsonschema:"Milestone IID (project-scoped),required"`
}

// GenerateMilestoneReportOutput holds the LLM-generated milestone report.
type GenerateMilestoneReportOutput struct {
	toolutil.HintableOutput
	MilestoneIID int64  `json:"milestone_iid"`
	Title        string `json:"title"`
	Report       string `json:"report"`
	Model        string `json:"model"`
	Truncated    bool   `json:"truncated"`
}

const generateMilestoneReportPrompt = `Generate a comprehensive milestone progress report from the data provided.

Requirements:
1. **Executive summary** — one paragraph overview of milestone health and progress
2. **Progress metrics** — percentage of issues/MRs completed vs total, open vs closed
3. **Key achievements** — notable completed work items
4. **At-risk items** — open issues/MRs that may delay the milestone, especially if due date is near
5. **Timeline assessment** — is the milestone on track based on start/due dates and remaining work?
6. **Recommendations** — suggest actions to keep the milestone on track

Use data-driven language with specific numbers. Output Markdown only.`

// GenerateMilestoneReport fetches milestone details, its issues and MRs,
// then delegates to the MCP sampling capability for report generation.
func GenerateMilestoneReport(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input GenerateMilestoneReportInput) (GenerateMilestoneReportOutput, error) {
	if input.ProjectID == "" {
		return GenerateMilestoneReportOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MilestoneIID <= 0 {
		return GenerateMilestoneReportOutput{}, errors.New("milestone_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 5, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return GenerateMilestoneReportOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 5, "Fetching milestone details...")

	milestone, err := milestones.Get(ctx, client, milestones.GetInput{
		ProjectID:    input.ProjectID,
		MilestoneIID: input.MilestoneIID,
	})
	if err != nil {
		return GenerateMilestoneReportOutput{}, fmt.Errorf("fetching milestone: %w", err)
	}

	tracker.Step(ctx, 3, 5, "Fetching milestone issues and merge requests...")

	msIssues, _ := milestones.GetIssues(ctx, client, milestones.GetIssuesInput{
		ProjectID:    input.ProjectID,
		MilestoneIID: input.MilestoneIID,
		PaginationInput: toolutil.PaginationInput{
			PerPage: 100,
		},
	})

	msMRs, _ := milestones.GetMergeRequests(ctx, client, milestones.GetMergeRequestsInput{
		ProjectID:    input.ProjectID,
		MilestoneIID: input.MilestoneIID,
		PaginationInput: toolutil.PaginationInput{
			PerPage: 100,
		},
	})

	data := FormatMilestoneForAnalysis(milestone, msIssues, msMRs)
	tracker.Step(ctx, 4, 5, "Requesting LLM report generation...")

	result, err := samplingClient.Analyze(ctx, generateMilestoneReportPrompt, data,
		sampling.WithMaxTokens(4096),
		sampling.WithTemperature(0.3),
		sampling.WithModelPriorities(0.4, 0.5, 0.5),
	)
	if err != nil {
		return GenerateMilestoneReportOutput{}, fmt.Errorf("LLM report generation: %w", err)
	}

	tracker.Step(ctx, 5, 5, "Report generated")

	return GenerateMilestoneReportOutput{
		MilestoneIID: input.MilestoneIID,
		Title:        milestone.Title,
		Report:       result.Content,
		Model:        result.Model,
		Truncated:    result.Truncated,
	}, nil
}

// FormatMilestoneForAnalysis builds a Markdown document from a milestone,
// its issues, and merge requests for LLM report generation.
func FormatMilestoneForAnalysis(ms milestones.Output, msIssues milestones.MilestoneIssuesOutput, msMRs milestones.MilestoneMergeRequestsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Milestone: %s\n\n", ms.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, ms.State)
	if ms.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, ms.Description)
	}
	if ms.StartDate != "" {
		fmt.Fprintf(&b, "- **Start Date**: %s\n", toolutil.FormatTime(ms.StartDate))
	}
	if ms.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(ms.DueDate))
	}
	fmt.Fprintf(&b, "- **Expired**: %v\n", ms.Expired)

	if len(msIssues.Issues) > 0 {
		open, closed := 0, 0
		for _, iss := range msIssues.Issues {
			if iss.State == "closed" {
				closed++
			} else {
				open++
			}
		}
		fmt.Fprintf(&b, "\n## Issues (%d total: %d open, %d closed)\n\n", len(msIssues.Issues), open, closed)
		for _, iss := range msIssues.Issues {
			fmt.Fprintf(&b, "- #%d — %s [%s]\n", iss.IID, iss.Title, iss.State)
		}
	}

	if len(msMRs.MergeRequests) > 0 {
		open, merged := 0, 0
		for _, mr := range msMRs.MergeRequests {
			if mr.State == "merged" {
				merged++
			} else {
				open++
			}
		}
		fmt.Fprintf(&b, "\n## Merge Requests (%d total: %d open, %d merged)\n\n", len(msMRs.MergeRequests), open, merged)
		for _, mr := range msMRs.MergeRequests {
			fmt.Fprintf(&b, "- !%d — %s [%s] (%s → %s)\n", mr.IID, mr.Title, mr.State, mr.SourceBranch, mr.TargetBranch)
		}
	}
	return b.String()
}

// FormatGenerateMilestoneReportMarkdown renders an LLM-generated milestone report.
func FormatGenerateMilestoneReportMarkdown(r GenerateMilestoneReportOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Milestone Report: %s\n\n", toolutil.EscapeMdHeading(r.Title))
	if r.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Report was truncated due to size limits.*\n\n")
	}
	b.WriteString(r.Report)
	b.WriteString("\n")
	if r.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", r.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_milestone_update` to adjust dates or status",
		"Use `gitlab_list_milestone_issues` to review open items",
	)
	return b.String()
}

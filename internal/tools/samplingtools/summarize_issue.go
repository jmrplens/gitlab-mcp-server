// summarize_issue.go implements the sampling-based issue summarization tool.

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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// SummarizeIssueInput defines parameters for LLM-assisted issue summarization.
type SummarizeIssueInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID"`
}

// SummarizeIssueOutput holds the LLM summary of an issue.
type SummarizeIssueOutput struct {
	toolutil.HintableOutput
	IssueIID  int64  `json:"issue_iid"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

// summarizeIssuePrompt is the system prompt sent to the LLM when summarizing
// an issue discussion, requesting a summary, key decisions, action items, and
// participant positions.
const summarizeIssuePrompt = `Summarize this GitLab issue discussion and provide:
1. **Issue summary** — what the issue is about and current status
2. **Key decisions** — important decisions made in the discussion
3. **Action items** — outstanding tasks or next steps
4. **Participants** — who contributed and their key positions

Be concise and focus on actionable information.`

// SummarizeIssue fetches an issue and its notes, then delegates to the MCP
// client's sampling capability for LLM-assisted summarization of the discussion.
// Returns [sampling.ErrSamplingNotSupported] if the client lacks sampling support.
func SummarizeIssue(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input SummarizeIssueInput) (SummarizeIssueOutput, error) {
	if input.ProjectID == "" {
		return SummarizeIssueOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return SummarizeIssueOutput{}, errors.New("issue_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return SummarizeIssueOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Fetching issue details and notes...")

	var data, title string

	// Try GraphQL aggregation (single request) with REST fallback.
	gqlResult, gqlErr := BuildIssueContext(ctx, client, string(input.ProjectID), input.IssueIID)
	if gqlErr == nil {
		data = gqlResult.Content
		title = gqlResult.Title
	} else {
		issue, err := issues.Get(ctx, client, issues.GetInput(input))
		if err != nil {
			return SummarizeIssueOutput{}, fmt.Errorf("fetching issue: %w", err)
		}
		title = issue.Title

		notes, err := issuenotes.List(ctx, client, issuenotes.ListInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
			PaginationInput: toolutil.PaginationInput{
				PerPage: 100,
			},
		})
		if err != nil {
			return SummarizeIssueOutput{}, fmt.Errorf("fetching issue notes: %w", err)
		}
		data = FormatIssueForSummary(issue, notes)
	}

	tracker.Step(ctx, 3, 4, "Requesting LLM summary...")

	result, err := samplingClient.Analyze(ctx, summarizeIssuePrompt, data,
		sampling.WithMaxTokens(2048),
		sampling.WithTemperature(0.3),
		sampling.WithModelPriorities(0.4, 0.6, 0.4),
	)
	if err != nil {
		return SummarizeIssueOutput{}, fmt.Errorf("LLM summary: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Summary complete")

	return SummarizeIssueOutput{
		IssueIID:  input.IssueIID,
		Title:     title,
		Summary:   result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatIssueForSummary builds a Markdown document from an issue and its notes,
// suitable for passing to an LLM for discussion summarization.
func FormatIssueForSummary(issue issues.Output, notes issuenotes.ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Issue #%d: %s\n\n", issue.IID, issue.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, issue.State)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, issue.Author)
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(issue.CreatedAt))
	if issue.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(issue.DueDate))
	}
	if len(issue.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", strings.Join(issue.Labels, ", "))
	}
	if len(issue.Assignees) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", strings.Join(issue.Assignees, ", "))
	}

	if issue.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", issue.Description)
	}

	if len(notes.Notes) > 0 {
		fmt.Fprintf(&b, "\n## Discussion (%d notes)\n\n", len(notes.Notes))
		for _, n := range notes.Notes {
			ts := n.CreatedAt
			if ts == "" {
				ts = "unknown"
			}
			fmt.Fprintf(&b, "**%s** (%s):\n%s\n\n---\n\n", n.Author, ts, n.Body)
		}
	}
	return b.String()
}

// FormatSummarizeIssueMarkdown renders an LLM-generated issue summary.
func FormatSummarizeIssueMarkdown(s SummarizeIssueOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Summary: #%d — %s\n\n", s.IssueIID, toolutil.EscapeMdHeading(s.Title))
	if s.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Summary was truncated due to size limits.*\n\n")
	}
	b.WriteString(s.Summary)
	b.WriteString("\n")
	if s.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", s.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_issue_update` to update status, labels, or assignee",
		"Use `gitlab_add_issue_note` to add follow-up comments",
	)
	return b.String()
}

// analyze_issue_scope.go implements the sampling-based issue scope analysis tool.

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

// AnalyzeIssueScopeInput defines parameters for LLM-assisted issue scope analysis.
type AnalyzeIssueScopeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	IssueIID  int64                `json:"issue_iid"  jsonschema:"Issue internal ID,required"`
}

// AnalyzeIssueScopeOutput holds the LLM analysis of an issue's scope and effort.
type AnalyzeIssueScopeOutput struct {
	toolutil.HintableOutput
	IssueIID  int64  `json:"issue_iid"`
	Title     string `json:"title"`
	Analysis  string `json:"analysis"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

const analyzeIssueScopePrompt = `Analyze the scope and effort of this GitLab issue and provide:
1. **Scope assessment** — what is the issue about and how well-defined is it
2. **Effort analysis** — estimated vs actual time spent, is the issue over/under its estimate
3. **Complexity indicators** — number of participants, discussion activity, linked MRs, labels
4. **Risk factors** — scope creep signals, unclear requirements, blocked dependencies
5. **Related work** — summarize linked/closing MRs and their status
6. **Recommendation** — should this issue be broken down, re-estimated, or is it well-scoped

Be data-driven, reference specific metrics from the data provided.`

// AnalyzeIssueScope fetches an issue with its time stats, participants, related MRs,
// and notes, then delegates to the MCP sampling capability for scope analysis.
func AnalyzeIssueScope(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input AnalyzeIssueScopeInput) (AnalyzeIssueScopeOutput, error) {
	if input.ProjectID == "" {
		return AnalyzeIssueScopeOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.IssueIID <= 0 {
		return AnalyzeIssueScopeOutput{}, errors.New("issue_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 6, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return AnalyzeIssueScopeOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 6, "Fetching issue details...")

	var data, title string

	// Try GraphQL aggregation (single request replaces 6 REST calls) with fallback.
	gqlResult, gqlErr := BuildIssueContext(ctx, client, string(input.ProjectID), input.IssueIID)
	if gqlErr == nil {
		data = gqlResult.Content
		title = gqlResult.Title
	} else {
		issue, err := issues.Get(ctx, client, issues.GetInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
		})
		if err != nil {
			return AnalyzeIssueScopeOutput{}, fmt.Errorf("fetching issue: %w", err)
		}
		title = issue.Title

		tracker.Step(ctx, 3, 6, "Fetching time stats, participants, and related MRs...")

		timeStats, _ := issues.GetTimeStats(ctx, client, issues.GetInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
		})

		participants, _ := issues.GetParticipants(ctx, client, issues.GetInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
		})

		closingMRs, _ := issues.ListMRsClosing(ctx, client, issues.ListMRsClosingInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
		})

		relatedMRs, _ := issues.ListMRsRelated(ctx, client, issues.ListMRsRelatedInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
		})

		tracker.Step(ctx, 4, 6, "Fetching issue notes...")

		notes, _ := issuenotes.List(ctx, client, issuenotes.ListInput{
			ProjectID: input.ProjectID,
			IssueIID:  input.IssueIID,
			PaginationInput: toolutil.PaginationInput{
				PerPage: 100,
			},
		})
		data = FormatIssueScopeForAnalysis(issue, timeStats, participants, closingMRs, relatedMRs, notes)
	}

	tracker.Step(ctx, 5, 6, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(ctx, analyzeIssueScopePrompt, data,
		sampling.WithTemperature(0.3),
		sampling.WithModelPriorities(0.3, 0.4, 0.6),
	)
	if err != nil {
		return AnalyzeIssueScopeOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 6, 6, "Analysis complete")

	return AnalyzeIssueScopeOutput{
		IssueIID:  input.IssueIID,
		Title:     title,
		Analysis:  result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// FormatIssueScopeForAnalysis builds a Markdown document from issue details,
// time stats, participants, related MRs, and notes for scope analysis.
func FormatIssueScopeForAnalysis(issue issues.Output, timeStats issues.TimeStatsOutput, participants issues.ParticipantsOutput, closingMRs, relatedMRs issues.RelatedMRsOutput, notes issuenotes.ListOutput) string {
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
	if issue.Weight > 0 {
		fmt.Fprintf(&b, "- **Weight**: %d\n", issue.Weight)
	}

	b.WriteString("\n## Time Tracking\n\n")
	if timeStats.HumanTimeEstimate != "" {
		fmt.Fprintf(&b, "- **Estimate**: %s\n", timeStats.HumanTimeEstimate)
	} else {
		b.WriteString("- **Estimate**: not set\n")
	}
	if timeStats.HumanTotalTimeSpent != "" {
		fmt.Fprintf(&b, "- **Time Spent**: %s\n", timeStats.HumanTotalTimeSpent)
	} else {
		b.WriteString("- **Time Spent**: none recorded\n")
	}

	if len(participants.Participants) > 0 {
		names := make([]string, len(participants.Participants))
		for i, p := range participants.Participants {
			names[i] = p.Username
		}
		fmt.Fprintf(&b, "\n## Participants (%d)\n\n%s\n", len(names), strings.Join(names, ", "))
	}

	if issue.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", issue.Description)
	}

	if len(closingMRs.MergeRequests) > 0 {
		fmt.Fprintf(&b, "\n## Closing MRs (%d)\n\n", len(closingMRs.MergeRequests))
		for _, mr := range closingMRs.MergeRequests {
			fmt.Fprintf(&b, "- !%d — %s [%s] (@%s)\n", mr.IID, mr.Title, mr.State, mr.Author)
		}
	}

	if len(relatedMRs.MergeRequests) > 0 {
		fmt.Fprintf(&b, "\n## Related MRs (%d)\n\n", len(relatedMRs.MergeRequests))
		for _, mr := range relatedMRs.MergeRequests {
			fmt.Fprintf(&b, "- !%d — %s [%s] (@%s)\n", mr.IID, mr.Title, mr.State, mr.Author)
		}
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

// FormatAnalyzeIssueScopeMarkdown renders an LLM-generated issue scope analysis.
func FormatAnalyzeIssueScopeMarkdown(a AnalyzeIssueScopeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Scope Analysis: #%d — %s\n\n", a.IssueIID, toolutil.EscapeMdHeading(a.Title))
	if a.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(a.Analysis)
	b.WriteString("\n")
	if a.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", a.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_issue_update` to refine scope, labels, or milestone",
		"Use `gitlab_add_issue_note` to document scope decisions",
	)
	return b.String()
}

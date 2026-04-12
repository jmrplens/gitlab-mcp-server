// analyze_mr_changes.go implements the sampling-based merge request changes analysis tool.

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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AnalyzeMRChangesInput defines parameters for LLM-assisted MR code review.
type AnalyzeMRChangesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID"`
}

// AnalyzeMRChangesOutput holds the LLM analysis result for an MR.
type AnalyzeMRChangesOutput struct {
	toolutil.HintableOutput
	MRIID     int64  `json:"mr_iid"`
	Title     string `json:"title"`
	Analysis  string `json:"analysis"`
	Model     string `json:"model"`
	Truncated bool   `json:"truncated"`
}

// analyzeMRPrompt is the system prompt sent to the LLM when analyzing merge
// request changes, requesting a summary, potential issues, and suggestions.
const analyzeMRPrompt = `Review this GitLab merge request and provide:
1. **Summary of changes** — what files were modified and why
2. **Potential issues or bugs** — logic errors, edge cases, security concerns
3. **Suggestions for improvement** — code quality, performance, maintainability

Be specific and reference file names and line context where applicable.`

// AnalyzeMRChanges fetches a merge request and its diffs, then delegates to
// the MCP client's sampling capability for LLM-assisted code review analysis.
// Returns [sampling.ErrSamplingNotSupported] if the client lacks sampling support.
func AnalyzeMRChanges(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input AnalyzeMRChangesInput) (AnalyzeMRChangesOutput, error) {
	if input.ProjectID == "" {
		return AnalyzeMRChangesOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return AnalyzeMRChangesOutput{}, errors.New("mr_iid must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 4, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return AnalyzeMRChangesOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 4, "Fetching merge request details and diffs...")

	var data, title string

	changes, err := mrchanges.Get(ctx, client, mrchanges.GetInput(input))
	if err != nil {
		return AnalyzeMRChangesOutput{}, fmt.Errorf("fetching MR changes: %w", err)
	}

	// Try GraphQL aggregation (single request replaces MR detail fetch) with fallback.
	gqlResult, gqlErr := BuildMRContext(ctx, client, string(input.ProjectID), input.MRIID)
	if gqlErr == nil {
		title = gqlResult.Title
		data = gqlResult.Content + formatChangesSection(changes)
	} else {
		var mr mergerequests.Output
		mr, err = mergerequests.Get(ctx, client, mergerequests.GetInput(input))
		if err != nil {
			return AnalyzeMRChangesOutput{}, fmt.Errorf("fetching MR: %w", err)
		}
		title = mr.Title
		data = FormatMRForAnalysis(mr, changes)
	}

	tracker.Step(ctx, 3, 4, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(ctx, analyzeMRPrompt, data)
	if err != nil {
		return AnalyzeMRChangesOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 4, 4, "Analysis complete")

	return AnalyzeMRChangesOutput{
		MRIID:     input.MRIID,
		Title:     title,
		Analysis:  result.Content,
		Model:     result.Model,
		Truncated: result.Truncated,
	}, nil
}

// formatChangesSection formats MR file changes as a Markdown section with diffs.
func formatChangesSection(changes mrchanges.Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Changed Files (%d)\n\n", len(changes.Changes))
	for _, c := range changes.Changes {
		action := "modified"
		if c.NewFile {
			action = "added"
		} else if c.DeletedFile {
			action = "deleted"
		} else if c.RenamedFile {
			action = fmt.Sprintf("renamed from %s", c.OldPath)
		}
		fmt.Fprintf(&b, "### %s (%s)\n\n```diff\n%s\n```\n\n", c.NewPath, action, c.Diff)
	}
	return b.String()
}

// FormatMRForAnalysis builds a Markdown document from a merge request and its
// file diffs, suitable for passing to an LLM for code review analysis.
// Also used by ReviewMRSecurity for security-focused reviews.
func FormatMRForAnalysis(mr mergerequests.Output, changes mrchanges.Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Merge Request !%d: %s\n\n", mr.IID, mr.Title)
	fmt.Fprintf(&b, toolutil.FmtMdState, mr.State)
	fmt.Fprintf(&b, "- **Source Branch**: %s → %s\n", mr.SourceBranch, mr.TargetBranch)
	fmt.Fprintf(&b, "- **Merge Status**: %s\n", mr.MergeStatus)

	if mr.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", mr.Description)
	}

	b.WriteString(formatChangesSection(changes))
	return b.String()
}

// FormatAnalyzeMRChangesMarkdown renders an LLM-generated MR analysis.
func FormatAnalyzeMRChangesMarkdown(a AnalyzeMRChangesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Analysis: !%d — %s\n\n", a.MRIID, toolutil.EscapeMdHeading(a.Title))
	if a.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(a.Analysis)
	b.WriteString("\n")
	if a.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", a.Model)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_add_mr_note` to comment on specific findings",
		"Use `gitlab_approve_merge_request` or `gitlab_update_merge_request` to act on the review",
	)
	return b.String()
}

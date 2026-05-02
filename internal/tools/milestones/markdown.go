package milestones

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdownString renders a ListOutput as a Markdown table string.
func FormatListMarkdownString(v ListOutput) string {
	var b strings.Builder
	if len(v.Milestones) == 0 {
		b.WriteString("No milestones found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Due Date | Expired |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, m := range v.Milestones {
		due := "—"
		if m.DueDate != "" {
			due = toolutil.FormatTime(m.DueDate)
		}
		expired := "No"
		if m.Expired {
			expired = "Yes"
		}
		fmt.Fprintf(&b, "| [%d](%s) | %s | %s | %s | %s |\n",
			m.IID, m.WebURL,
			toolutil.EscapeMdTableCell(m.Title),
			m.State,
			due,
			expired,
		)
	}
	toolutil.WritePagination(&b, v.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'milestone_get' with milestone_iid to see details",
		"Use action 'milestone_create' to create a new milestone",
	)
	return b.String()
}

// FormatListMarkdown returns a Markdown MCP tool result for a ListOutput.
func FormatListMarkdown(v ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(v))
}

// FormatMarkdown renders a single milestone as a Markdown string.
func FormatMarkdown(v Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Milestone: %s\n\n", toolutil.EscapeMdHeading(v.Title))
	fmt.Fprintf(&b, "- **ID**: %d (IID: %d)\n", v.ID, v.IID)
	fmt.Fprintf(&b, toolutil.FmtMdState, v.State)
	if v.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, v.Description)
	}
	if v.StartDate != "" {
		fmt.Fprintf(&b, "- **Start Date**: %s\n", toolutil.FormatTime(v.StartDate))
	}
	if v.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(v.DueDate))
	}
	fmt.Fprintf(&b, "- **Expired**: %v\n", v.Expired)
	if v.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, v.WebURL)
	}
	if v.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(v.CreatedAt))
	}
	if v.UpdatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdUpdated, toolutil.FormatTime(v.UpdatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'milestone_issues' to list issues in this milestone",
		"Use action 'milestone_merge_requests' to list MRs in this milestone",
	)
	return b.String()
}

// FormatIssuesMarkdownString renders milestone issues as a Markdown table string.
func FormatIssuesMarkdownString(v MilestoneIssuesOutput) string {
	var b strings.Builder
	if len(v.Issues) == 0 {
		b.WriteString("No issues found for this milestone.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Created |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, issue := range v.Issues {
		created := "—"
		if issue.CreatedAt != "" {
			created = issue.CreatedAt
		}
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s | %s |\n",
			issue.IID,
			issue.WebURL,
			toolutil.EscapeMdTableCell(issue.Title),
			issue.State,
			created,
		)
	}
	toolutil.WritePagination(&b, v.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_issue action 'get' with issue IID for full details",
		"Use action 'milestone_merge_requests' to view MRs in this milestone",
	)
	return b.String()
}

// FormatIssuesMarkdown returns a Markdown MCP tool result for milestone issues.
func FormatIssuesMarkdown(v MilestoneIssuesOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatIssuesMarkdownString(v))
}

// FormatMergeRequestsMarkdownString renders milestone merge requests as a Markdown table string.
func FormatMergeRequestsMarkdownString(v MilestoneMergeRequestsOutput) string {
	var b strings.Builder
	if len(v.MergeRequests) == 0 {
		b.WriteString("No merge requests found for this milestone.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Source | Target | Created |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, mr := range v.MergeRequests {
		created := "—"
		if mr.CreatedAt != "" {
			created = mr.CreatedAt
		}
		fmt.Fprintf(&b, "| [!%d](%s) | %s | %s | %s | %s | %s |\n",
			mr.IID,
			mr.WebURL,
			toolutil.EscapeMdTableCell(mr.Title),
			mr.State,
			toolutil.EscapeMdTableCell(mr.SourceBranch),
			toolutil.EscapeMdTableCell(mr.TargetBranch),
			created,
		)
	}
	toolutil.WritePagination(&b, v.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use gitlab_merge_request action 'get' with MR IID for full details",
		"Use action 'milestone_issues' to view issues in this milestone",
	)
	return b.String()
}

// FormatMergeRequestsMarkdown returns a Markdown MCP tool result for milestone merge requests.
func FormatMergeRequestsMarkdown(v MilestoneMergeRequestsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMergeRequestsMarkdownString(v))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatIssuesMarkdownString)
	toolutil.RegisterMarkdown(FormatMergeRequestsMarkdownString)
}

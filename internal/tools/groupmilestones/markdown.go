// markdown.go provides Markdown formatting functions for group milestone MCP tool output.
package groupmilestones

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMarkdown renders a single group milestone as a Markdown string.
func FormatMarkdown(v Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Milestone: %s\n\n", toolutil.EscapeMdHeading(v.Title))
	fmt.Fprintf(&b, "- **ID**: %d (IID: %d)\n", v.ID, v.IID)
	if v.GroupPath != "" {
		fmt.Fprintf(&b, "- **Group**: %s\n", v.GroupPath)
	} else {
		fmt.Fprintf(&b, "- **Group**: %d\n", v.GroupID)
	}
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
	if v.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(v.CreatedAt))
	}
	if v.UpdatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdUpdated, toolutil.FormatTime(v.UpdatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'group_milestone_update' to modify this milestone",
		"Use action 'group_milestone_issues' or 'group_milestone_merge_requests' to list associated items",
		"Use action 'group_milestone_delete' to remove this milestone",
	)
	return b.String()
}

// FormatListMarkdownString renders a paginated list of group milestones as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Milestones (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Milestones), out.Pagination)
	if len(out.Milestones) == 0 {
		b.WriteString("No group milestones found.\n")
		return b.String()
	}
	b.WriteString("| ID | IID | Title | State | Start Date | Due Date |\n")
	b.WriteString("|----|-----|-------|-------|------------|----------|\n")
	for _, m := range out.Milestones {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %s |\n",
			m.ID, m.IID, toolutil.EscapeMdTableCell(m.Title), m.State, m.StartDate, m.DueDate)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'group_milestone_get' with milestone_id for full details",
		"Use action 'group_milestone_create' to add a new group milestone",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of group milestones as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatIssuesMarkdownString renders a paginated list of milestone issues as a Markdown table string.
func FormatIssuesMarkdownString(out IssuesOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Milestone Issues (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString("No issues found for this milestone.\n")
		return b.String()
	}
	b.WriteString("| ID | IID | Title | State |\n")
	b.WriteString("|----|-----|-------|-------|\n")
	for _, issue := range out.Issues {
		fmt.Fprintf(&b, "| %d | %d | %s | %s |\n",
			issue.ID, issue.IID, toolutil.MdTitleLink(issue.Title, issue.WebURL), issue.State)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks, "Use `gitlab_issue_get` to view full issue details", "Filter by state to narrow down results")
	return b.String()
}

// FormatIssuesMarkdown renders a paginated list of milestone issues as an MCP Markdown result.
func FormatIssuesMarkdown(out IssuesOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatIssuesMarkdownString(out))
}

// FormatMergeRequestsMarkdownString renders a paginated list of milestone MRs as a Markdown table string.
func FormatMergeRequestsMarkdownString(out MergeRequestsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Milestone Merge Requests (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.MergeRequests), out.Pagination)
	if len(out.MergeRequests) == 0 {
		b.WriteString("No merge requests found for this milestone.\n")
		return b.String()
	}
	b.WriteString("| ID | IID | Title | State | Source | Target |\n")
	b.WriteString("|----|-----|-------|-------|--------|--------|\n")
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %s |\n",
			mr.ID, mr.IID, toolutil.MdTitleLink(mr.Title, mr.WebURL), mr.State,
			toolutil.EscapeMdTableCell(mr.SourceBranch), toolutil.EscapeMdTableCell(mr.TargetBranch))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks, "Use `gitlab_mr_get` to view full MR details", "Filter by state to see only open or merged MRs")
	return b.String()
}

// FormatMergeRequestsMarkdown renders a paginated list of milestone MRs as an MCP Markdown result.
func FormatMergeRequestsMarkdown(out MergeRequestsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMergeRequestsMarkdownString(out))
}

// FormatBurndownChartEventsMarkdownString renders burndown chart events as a Markdown table string.
func FormatBurndownChartEventsMarkdownString(out BurndownChartEventsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Burndown Chart Events (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Events), out.Pagination)
	if len(out.Events) == 0 {
		b.WriteString("No burndown chart events found.\n")
		return b.String()
	}
	b.WriteString("| Created At | Weight | Action |\n")
	b.WriteString("|------------|--------|--------|\n")
	for _, e := range out.Events {
		fmt.Fprintf(&b, "| %s | %d | %s |\n", toolutil.FormatTime(e.CreatedAt), e.Weight, e.Action)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Track milestone progress by comparing event weights over time")
	return b.String()
}

// FormatBurndownChartEventsMarkdown renders burndown chart events as an MCP Markdown result.
func FormatBurndownChartEventsMarkdown(out BurndownChartEventsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatBurndownChartEventsMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatIssuesMarkdownString)
	toolutil.RegisterMarkdown(FormatMergeRequestsMarkdownString)
	toolutil.RegisterMarkdown(FormatBurndownChartEventsMarkdownString)
}

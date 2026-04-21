// markdown.go provides Markdown formatting functions for issue MCP tool output.

package issues

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatTodoMarkdown renders a to-do item as a Markdown summary.
func FormatTodoMarkdown(t TodoOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Todo #%d\n\n", t.ID)
	fmt.Fprintf(&b, "- **Action**: %s\n", t.ActionName)
	fmt.Fprintf(&b, "- **Target Type**: %s\n", t.TargetType)
	if t.TargetTitle != "" {
		fmt.Fprintf(&b, toolutil.FmtMdTarget, t.TargetTitle)
	}
	fmt.Fprintf(&b, toolutil.FmtMdState, t.State)
	if t.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(t.CreatedAt))
	}
	if t.TargetURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURLNewline, t.TargetURL)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_todo_mark_done` to mark this todo as completed",
		"Use `gitlab_issue_get` to view the referenced issue",
	)
	return b.String()
}

// FormatListAllMarkdown renders a list of globally-scoped issues as a Markdown table.
func FormatListAllMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## All Issues (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString(msgNoIssuesFound)
		return b.String()
	}
	b.WriteString(tblHeaderIssues)
	b.WriteString(toolutil.TblSep5Col)
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s %s | %s | %s |\n", i.IID, i.WebURL, toolutil.EscapeMdTableCell(i.Title), toolutil.IssueStateEmoji(i.State), i.State, toolutil.EscapeMdTableCell(i.Author), toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_issue_get` to view issue details",
		"Use `gitlab_issue_update` to change state or labels",
	)
	return b.String()
}

// FormatTimeStatsMarkdown renders time tracking statistics as Markdown.
func FormatTimeStatsMarkdown(ts TimeStatsOutput) string {
	var b strings.Builder
	b.WriteString("## Time Tracking\n\n")
	if ts.HumanTimeEstimate != "" {
		fmt.Fprintf(&b, "- **Estimate**: %s\n", ts.HumanTimeEstimate)
	}
	if ts.HumanTotalTimeSpent != "" {
		fmt.Fprintf(&b, "- **Spent**: %s\n", ts.HumanTotalTimeSpent)
	}
	fmt.Fprintf(&b, "- **Estimate (seconds)**: %d\n", ts.TimeEstimate)
	fmt.Fprintf(&b, "- **Spent (seconds)**: %d\n", ts.TotalTimeSpent)
	toolutil.WriteHints(&b,
		"Use `gitlab_issue_update` to adjust time tracking",
	)
	return b.String()
}

// FormatParticipantsMarkdown renders an issue's participant list as Markdown.
func FormatParticipantsMarkdown(out ParticipantsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Participants (%d)\n\n", len(out.Participants))
	if len(out.Participants) == 0 {
		b.WriteString("No participants found.\n")
		return b.String()
	}
	b.WriteString("| Username | Name |\n")
	b.WriteString(toolutil.TblSep2Col)
	for _, p := range out.Participants {
		fmt.Fprintf(&b, "| @%s | %s |\n", toolutil.EscapeMdTableCell(p.Username), toolutil.EscapeMdTableCell(p.Name))
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_issue_get` to view the issue details",
		"Use `gitlab_issue_note_create` to notify participants",
	)
	return b.String()
}

// FormatRelatedMRsMarkdown renders a list of related merge requests as Markdown.
func FormatRelatedMRsMarkdown(out RelatedMRsOutput, heading string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", heading, len(out.MergeRequests))
	if len(out.MergeRequests) == 0 {
		b.WriteString("No merge requests found.\n")
		return b.String()
	}
	b.WriteString("| IID | Title | State | Author | Source → Target |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&b, "| !%d | %s | %s | @%s | %s → %s |\n", mr.IID, toolutil.EscapeMdTableCell(mr.Title), mr.State, toolutil.EscapeMdTableCell(mr.Author), mr.SourceBranch, mr.TargetBranch)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use `gitlab_mr_get` to view MR details",
		"Use `gitlab_mr_changes_get` to see MR diff",
	)
	return b.String()
}

// FormatMarkdown renders a single issue as a Markdown summary.
func FormatMarkdown(i Output) string {
	var b strings.Builder
	confidentialTag := ""
	if i.Confidential {
		confidentialTag = " " + toolutil.EmojiConfidential
	}
	fmt.Fprintf(&b, "## %s Issue #%d: %s%s\n\n", toolutil.IssueStateEmoji(i.State), i.IID, toolutil.EscapeMdHeading(i.Title), confidentialTag)
	if i.References != "" {
		fmt.Fprintf(&b, "- **Reference**: %s\n", i.References)
	}
	fmt.Fprintf(&b, "- **State**: %s %s\n", toolutil.IssueStateEmoji(i.State), i.State)
	if i.IssueType != "" && i.IssueType != "issue" {
		fmt.Fprintf(&b, "- **Type**: %s\n", i.IssueType)
	}
	if i.Confidential {
		fmt.Fprintf(&b, "- %s **Confidential**\n", toolutil.EmojiConfidential)
	}
	fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, i.Author)
	if len(i.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels**: %s\n", strings.Join(i.Labels, ", "))
	}
	if len(i.Assignees) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", strings.Join(prefixAt(i.Assignees), ", "))
	}
	if i.Milestone != "" {
		fmt.Fprintf(&b, "- **Milestone**: %s\n", i.Milestone)
	}
	if i.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(i.DueDate))
	}
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(i.CreatedAt))
	if i.State == "closed" && i.ClosedBy != "" {
		fmt.Fprintf(&b, "- **Closed By**: @%s", i.ClosedBy)
		if i.ClosedAt != "" {
			fmt.Fprintf(&b, " on %s", toolutil.FormatTime(i.ClosedAt))
		}
		b.WriteByte('\n')
	}
	if i.MergeRequestCount > 0 {
		fmt.Fprintf(&b, "- **Linked MRs**: %d\n", i.MergeRequestCount)
	}
	if i.TaskCompletionTotal > 0 {
		fmt.Fprintf(&b, "- **Tasks**: %d/%d completed\n", i.TaskCompletionCount, i.TaskCompletionTotal)
	}
	if i.UserNotesCount > 0 {
		fmt.Fprintf(&b, "- **Comments**: %d\n", i.UserNotesCount)
	}
	if i.Description != "" {
		fmt.Fprintf(&b, "\n### Description\n\n%s%s\n", toolutil.WrapGFMBody(i.Description), toolutil.RichContentHint(toolutil.DetectRichContent(i.Description), i.WebURL))
	}
	fmt.Fprintf(&b, toolutil.FmtMdURLNewline, i.WebURL)
	toolutil.WriteHints(&b,
		"Use gitlab_issue_note action 'list' to see comments on this issue",
		"Use action 'update' to change title, labels, assignees, or milestone",
		"Use action 'mrs_related' to find linked MRs",
	)
	return b.String()
}

// FormatListMarkdown renders a list of issues as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issues (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString(msgNoIssuesFound)
		return b.String()
	}
	b.WriteString(tblHeaderIssues)
	b.WriteString(toolutil.TblSep5Col)
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s %s | %s | %s |\n", i.IID, i.WebURL, toolutil.EscapeMdTableCell(i.Title), toolutil.IssueStateEmoji(i.State), i.State, toolutil.EscapeMdTableCell(i.Author), toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' with an issue_iid to see full details and description",
		"Use action 'create' to create a new issue",
		"Use gitlab_issue_note action 'create' to add a comment",
	)
	return b.String()
}

// FormatListGroupMarkdown renders a paginated list of group issues as a Markdown table.
func FormatListGroupMarkdown(out ListGroupOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Issues (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString(msgNoIssuesFound)
		return b.String()
	}
	b.WriteString(tblHeaderIssues)
	b.WriteString(toolutil.TblSep5Col)
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		fmt.Fprintf(&b, "| [#%d](%s) | %s | %s | %s | %s |\n", i.IID, i.WebURL, toolutil.EscapeMdTableCell(i.Title), i.State, toolutil.EscapeMdTableCell(i.Author), toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_issue_get` to view issue details",
		"Use `gitlab_issue_create` to open a new issue",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatTodoMarkdown)
	toolutil.RegisterMarkdown(FormatTimeStatsMarkdown)
	toolutil.RegisterMarkdown(FormatParticipantsMarkdown)
	toolutil.RegisterMarkdown(func(v RelatedMRsOutput) string { return FormatRelatedMRsMarkdown(v, "Related MRs") })
	toolutil.RegisterMarkdown(FormatListGroupMarkdown)
}

package issues

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatTodoMarkdown renders a to-do item as a Markdown summary.
func FormatTodoMarkdown(t TodoOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Todo #%d\n\n", t.ID)
	//gitlab:allow-unescaped t.ActionName: a to-do action, a gl.TodoAction GitLab picks from a fixed set.
	fmt.Fprintf(&b, "- **Action**: %s\n", t.ActionName)
	//gitlab:allow-unescaped t.TargetType: a to-do target type, a gl.TodoTargetType GitLab picks from a fixed set.
	fmt.Fprintf(&b, "- **Target Type**: %s\n", t.TargetType)
	if t.TargetTitle != "" {
		fmt.Fprintf(&b, toolutil.FmtMdTarget, toolutil.EscapeMdTableCell(t.TargetTitle))
	}
	//gitlab:allow-unescaped t.State: a to-do state GitLab picks from a fixed set (pending, done).
	fmt.Fprintf(&b, toolutil.FmtMdState, t.State)
	if t.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(t.CreatedAt))
	}
	if t.TargetURL != "" {
		toolutil.WriteMdURLNewline(&b, t.TargetURL)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_todo_mark_done` to mark this todo as completed",
		"Use `gitlab_issue_get` to view the referenced issue",
	)
	return b.String()
}

// formatIssueList renders an issue list under the given heading, closing with
// the caller's hints.
//
// The two list surfaces differ only in those two things; the table between
// them is the same. Rendering it once is what keeps them that way: the row
// format carries the issue link, the state emoji and the escaping, and a
// change applied to one copy and not the other would silently leave the
// project and global listings rendering differently.
func formatIssueList(out ListOutput, heading string, hints ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", heading, out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Issues), out.Pagination)
	if len(out.Issues) == 0 {
		b.WriteString(msgNoIssuesFound)
		return b.String()
	}
	b.WriteString(tblHeaderIssues)
	b.WriteString(toolutil.TblSep5Col)
	for _, i := range out.Issues {
		labels := strings.Join(i.Labels, ", ")
		//gitlab:allow-unescaped i.State: an issue state, one of GitLab's fixed set (opened, closed).
		fmt.Fprintf(&b, "| %s | %s | %s %s | %s | %s |\n", toolutil.MdTitleLink(fmt.Sprintf("#%d", i.IID), i.WebURL), toolutil.EscapeMdTableCell(i.Title), toolutil.IssueStateEmoji(i.State), i.State, toolutil.EscapeMdTableCell(AuthorName(i)), toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, append([]string{toolutil.HintPreserveLinks}, hints...)...)
	return b.String()
}

// FormatListAllMarkdown renders a list of globally-scoped issues as a Markdown table.
func FormatListAllMarkdown(out ListOutput) string {
	return formatIssueList(out, "All Issues",
		"Use `gitlab_issue_get` to view issue details",
		"Use `gitlab_issue_update` to change state or labels",
	)
}

// AuthorName returns the issue author's display username for Markdown, read from
// the full author object. It is exported so sibling packages (e.g.
// mergerequests) that embed [Output] in their own tables can render the author
// without reaching into the object.
func AuthorName(i Output) string {
	if i.Author != nil {
		return i.Author.Username
	}
	return ""
}

// assigneeUsernames returns the assignee usernames for Markdown rendering,
// read from the full assignee objects.
func assigneeUsernames(i Output) []string {
	names := make([]string, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		if a != nil {
			names = append(names, a.Username)
		}
	}
	return names
}

// closerName returns the username of the user that closed the issue for
// Markdown, read from the full closer object.
func closerName(i Output) string {
	if i.ClosedBy != nil {
		return i.ClosedBy.Username
	}
	return ""
}

// FormatTimeStatsMarkdown renders time tracking statistics as Markdown.
func FormatTimeStatsMarkdown(ts TimeStatsOutput) string {
	var b strings.Builder
	b.WriteString("## Time Tracking\n\n")
	if ts.HumanTimeEstimate != "" {
		//gitlab:allow-unescaped ts.HumanTimeEstimate: a duration GitLab renders from a count of seconds, "3d 4h 30m".
		fmt.Fprintf(&b, "- **Estimate**: %s\n", ts.HumanTimeEstimate)
	}
	if ts.HumanTotalTimeSpent != "" {
		//gitlab:allow-unescaped ts.HumanTotalTimeSpent: a duration GitLab renders from a count of seconds, "3d 4h 30m".
		fmt.Fprintf(&b, "- **Spent**: %s\n", ts.HumanTotalTimeSpent)
	}
	fmt.Fprintf(&b, "- **Estimate (seconds)**: %d\n", ts.TimeEstimate)
	fmt.Fprintf(&b, "- **Spent (seconds)**: %d\n", ts.TotalTimeSpent)
	toolutil.WriteHints(
		&b,
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
	toolutil.WriteHints(
		&b,
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
	b.WriteString("| IID | Title | State | Author | Source -> Target |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, mr := range out.MergeRequests {
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		//gitlab:allow-unescaped mr.State: a merge request state, one of GitLab's fixed set (opened, closed, locked, merged).
		fmt.Fprintf(&b, "| !%d | %s | %s | @%s | %s -> %s |\n", mr.IID, toolutil.EscapeMdTableCell(mr.Title), mr.State, toolutil.EscapeMdTableCell(author), toolutil.EscapeMdTableCell(mr.SourceBranch), toolutil.EscapeMdTableCell(mr.TargetBranch))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
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
	if i.References != nil && i.References.Full != "" {
		fmt.Fprintf(&b, "- **Reference**: %s\n", toolutil.EscapeMdTableCell(i.References.Full))
	}
	fmt.Fprintf(&b, "- **State**: %s %s\n", toolutil.IssueStateEmoji(i.State), i.State)
	//gitlab:allow-unescaped i.IssueType: an issue type GitLab picks from a fixed set (issue, incident, test_case, task).
	if i.IssueType != "" && i.IssueType != "issue" {
		fmt.Fprintf(&b, "- **Type**: %s\n", i.IssueType)
	}
	if i.Confidential {
		fmt.Fprintf(&b, "- %s **Confidential**\n", toolutil.EmojiConfidential)
	}
	fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, toolutil.EscapeMdTableCell(AuthorName(i)))
	if len(i.Labels) > 0 {
		// A label title is free text: GitLab's only rule on one is that it
		// carries no comma.
		fmt.Fprintf(&b, "- **Labels**: %s\n", toolutil.EscapeMdTableCell(strings.Join(i.Labels, ", ")))
	}
	if names := assigneeUsernames(i); len(names) > 0 {
		fmt.Fprintf(&b, "- **Assignees**: %s\n", toolutil.EscapeMdTableCell(strings.Join(prefixAt(names), ", ")))
	}
	if i.Milestone != nil && i.Milestone.Title != "" {
		fmt.Fprintf(&b, "- **Milestone**: %s\n", toolutil.EscapeMdTableCell(i.Milestone.Title))
	}
	if i.DueDate != "" {
		fmt.Fprintf(&b, "- **Due Date**: %s\n", toolutil.FormatTime(i.DueDate))
	}
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(i.CreatedAt))
	if i.State == "closed" && closerName(i) != "" {
		fmt.Fprintf(&b, "- **Closed By**: @%s", toolutil.EscapeMdTableCell(closerName(i)))
		if i.ClosedAt != "" {
			fmt.Fprintf(&b, " on %s", toolutil.FormatTime(i.ClosedAt))
		}
		b.WriteByte('\n')
	}
	if i.MergeRequestCount > 0 {
		fmt.Fprintf(&b, "- **Linked MRs**: %d\n", i.MergeRequestCount)
	}
	if i.TaskCompletionStatus != nil && i.TaskCompletionStatus.Count > 0 {
		fmt.Fprintf(&b, "- **Tasks**: %d/%d completed\n", i.TaskCompletionStatus.CompletedCount, i.TaskCompletionStatus.Count)
	}
	if i.UserNotesCount > 0 {
		fmt.Fprintf(&b, "- **Comments**: %d\n", i.UserNotesCount)
	}
	if i.Description != "" {
		fmt.Fprintf(&b, "\n### Description\n\n%s%s\n", toolutil.WrapGFMBody(i.Description), toolutil.RichContentHint(toolutil.DetectRichContent(i.Description), i.WebURL))
	}
	toolutil.WriteMdURLNewline(&b, i.WebURL)
	toolutil.WriteHints(
		&b,
		"Use gitlab_issue action 'note_list' to see comments on this issue",
		"Use action 'update' to change title, labels, assignees, or milestone",
		"Use action 'mrs_related' to find linked MRs",
	)
	return b.String()
}

// formatGetMarkdownResult renders a single issue. The canonical resource is
// not embedded here: the issue.get spec declares it and the dispatchers embed
// it, the same way as for every other get action.
func formatGetMarkdownResult(out getOutput) *mcp.CallToolResult {
	return toolutil.ToolResultAnnotated(FormatMarkdown(out.Output), toolutil.ContentDetail)
}

// FormatListMarkdown renders a list of issues as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return formatIssueList(out, "Issues",
		"Use action 'get' with an issue_iid to see full details and description",
		"Use action 'create' to create a new issue",
		"Use gitlab_issue action 'note_create' to add a comment",
	)
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
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", toolutil.MdTitleLink(fmt.Sprintf("#%d", i.IID), i.WebURL), toolutil.EscapeMdTableCell(i.Title), i.State, toolutil.EscapeMdTableCell(AuthorName(i)), toolutil.EscapeMdTableCell(labels))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_issue_get` to view issue details",
		"Use `gitlab_issue_create` to open a new issue",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatGetMarkdownResult)
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatTodoMarkdown)
	toolutil.RegisterMarkdown(FormatTimeStatsMarkdown)
	toolutil.RegisterMarkdown(FormatParticipantsMarkdown)
	toolutil.RegisterMarkdown(func(v RelatedMRsOutput) string { return FormatRelatedMRsMarkdown(v, "Related MRs") })
	toolutil.RegisterMarkdown(FormatListGroupMarkdown)
}

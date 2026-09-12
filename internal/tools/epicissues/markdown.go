package epicissues

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders the issues of one epic as a Markdown table: a
// collection of objects that share columns.
//
// The ID column carries the GraphQL global id, which is what the assign and
// remove actions take; the reference is linked to the issue, which is what the
// preserve-links hint is for and what the table used to promise without
// carrying a single link.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Issues) == 0 {
		return toolutil.EmptyMessage("issues in this epic")
	}
	var b strings.Builder
	// A cursor connection counts nothing it has not walked, so the heading
	// carries the count shown and the cursor line at the bottom says whether
	// more follow.
	toolutil.WriteListHeading(&b, "Epic Issues", len(out.Issues), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Title", "State", "Author", "Labels", "Created"))
	for _, issue := range out.Issues {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(issue.ID),
			toolutil.MdTitleLink(fmt.Sprintf("#%d", issue.IID), issue.WebURL),
			toolutil.EscapeMdTableCell(issue.Title),
			issueStateCell(issue.State),
			toolutil.MdUserHandle(issue.Author),
			toolutil.EscapeMdTableCell(strings.Join(issue.Labels, ", ")),
			toolutil.FormatTime(issue.CreatedAt),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Issues))
	toolutil.WriteHints(&b, toolutil.ListHints(
		toolutil.HintAction(actionEpicIssueAssign, "add an issue to this epic"),
		toolutil.HintAction(actionEpicIssueRemove, "unlink an issue from this epic"),
	)...)
	return b.String()
}

// issueStateCell renders an issue state with the emoji every issue row in the
// tree shows, and nothing when the query did not select one.
func issueStateCell(state string) string {
	if strings.TrimSpace(state) == "" {
		return ""
	}
	return toolutil.IssueStateEmoji(state) + " " + toolutil.EscapeMdTableCell(state)
}

// FormatAssignMarkdown renders an epic-issue assignment or removal as the card
// of the link it changed: the two global ids the mutation echoed, which are
// what every later call on this pair takes.
func FormatAssignMarkdown(out AssignOutput, action string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Epic Issue "+action)
	c.Code("Epic", out.EpicGID)
	c.Code("Issue", out.ChildGID)
	c.End(
		toolutil.HintAction(actionEpicIssueList, "view all issues in the epic"),
		toolutil.HintAction(actionEpicIssueRemove, "unlink an issue from the epic"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(func(v AssignOutput) string { return FormatAssignMarkdown(v, "assigned") })
}

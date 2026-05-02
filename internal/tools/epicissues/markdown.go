package epicissues

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a list of child issues as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	if len(out.Issues) == 0 {
		b.WriteString("## Epic Issues\n\nNo issues found in this epic.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "## Epic Issues (%d)\n\n", len(out.Issues))
	b.WriteString("| IID | Title | State | Author | Labels | Created |\n")
	b.WriteString(toolutil.TblSep6Col)
	for _, issue := range out.Issues {
		labels := ""
		if len(issue.Labels) > 0 {
			labels = strings.Join(issue.Labels, ", ")
		}
		fmt.Fprintf(&b, "| #%d | %s | %s | %s | %s | %s |\n",
			issue.IID,
			toolutil.EscapeMdTableCell(issue.Title),
			issue.State,
			toolutil.EscapeMdTableCell(issue.Author),
			toolutil.EscapeMdTableCell(labels),
			toolutil.FormatTime(issue.CreatedAt),
		)
	}
	pag := toolutil.FormatGraphQLPagination(out.Pagination, len(out.Issues))
	if pag != "" {
		b.WriteString("\n" + pag)
	}
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'epic_issue_assign' to add an issue to this epic",
		"Use action 'epic_issue_remove' to unlink an issue from this epic",
	)
	return b.String()
}

// FormatAssignMarkdown renders an epic-issue assignment or removal result.
func FormatAssignMarkdown(out AssignOutput, action string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Epic Issue %s\n\n", action)
	if out.EpicGID != "" {
		fmt.Fprintf(&b, "- **Epic**: %s\n", out.EpicGID)
	}
	if out.ChildGID != "" {
		fmt.Fprintf(&b, "- **Issue**: %s\n", out.ChildGID)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_epic_issue_list` to view all issues in the epic",
		"Use `gitlab_epic_issue_remove` to unlink an issue from the epic",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(func(v AssignOutput) string { return FormatAssignMarkdown(v, "assigned") })
}

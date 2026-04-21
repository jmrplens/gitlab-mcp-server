// markdown.go provides Markdown formatting functions for group issue board MCP tool output.

package groupboards

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatGroupBoardMarkdown formats a single group board as markdown.
func FormatGroupBoardMarkdown(out GroupBoardOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Board: %s (ID: %d)\n\n", toolutil.EscapeMdTableCell(out.Name), out.ID)
	if out.GroupName != "" {
		fmt.Fprintf(&b, "**Group**: %s (ID: %d)\n", out.GroupName, out.GroupID)
	}
	if out.MilestoneTitle != "" {
		fmt.Fprintf(&b, "**Milestone**: %s (ID: %d)\n", out.MilestoneTitle, out.MilestoneID)
	}
	if len(out.Labels) > 0 {
		fmt.Fprintf(&b, "**Labels**: %s\n", strings.Join(out.Labels, ", "))
	}
	if len(out.Lists) > 0 {
		b.WriteString("\n### Lists\n\n| ID | Label | Position | Max Issues | Max Weight |\n|---|---|---|---|---|\n")
		for _, l := range out.Lists {
			fmt.Fprintf(&b, "| %d | %s | %d | %d | %d |\n",
				l.ID, toolutil.EscapeMdTableCell(l.LabelName), l.Position, l.MaxIssueCount, l.MaxIssueWeight)
		}
	}
	toolutil.WriteHints(&b, "Use board list tools to manage columns in this board")
	return b.String()
}

// FormatListGroupBoardsMarkdown formats a paginated list of group boards.
func FormatListGroupBoardsMarkdown(out ListGroupBoardsOutput) string {
	var b strings.Builder
	b.WriteString("## Group Issue Boards\n\n")
	toolutil.WriteListSummary(&b, len(out.Boards), out.Pagination)
	b.WriteString("| ID | Name | Group | Milestone | Lists |\n|---|---|---|---|---|\n")
	for _, bd := range out.Boards {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %d |\n",
			bd.ID, toolutil.EscapeMdTableCell(bd.Name),
			toolutil.EscapeMdTableCell(bd.GroupName),
			toolutil.EscapeMdTableCell(bd.MilestoneTitle),
			len(bd.Lists))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_get_group_board` to view details of a specific board")
	return b.String()
}

// FormatBoardListMarkdown formats a single board list as markdown.
func FormatBoardListMarkdown(out BoardListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Board List (ID: %d)\n\n", out.ID)
	if out.LabelName != "" {
		fmt.Fprintf(&b, "**Label**: %s (ID: %d)\n", out.LabelName, out.LabelID)
	}
	fmt.Fprintf(&b, "**Position**: %d\n", out.Position)
	if out.MaxIssueCount > 0 {
		fmt.Fprintf(&b, "**Max Issue Count**: %d\n", out.MaxIssueCount)
	}
	if out.MaxIssueWeight > 0 {
		fmt.Fprintf(&b, "**Max Issue Weight**: %d\n", out.MaxIssueWeight)
	}
	if out.AssigneeUser != "" {
		fmt.Fprintf(&b, "**Assignee**: @%s (ID: %d)\n", out.AssigneeUser, out.AssigneeID)
	}
	if out.MilestoneTitle != "" {
		fmt.Fprintf(&b, "**Milestone**: %s (ID: %d)\n", out.MilestoneTitle, out.MilestoneID)
	}
	toolutil.WriteHints(&b, "Use `gitlab_update_group_board_list` to reorder or modify this list")
	return b.String()
}

// FormatListBoardListsMarkdown formats a paginated list of board lists.
func FormatListBoardListsMarkdown(out ListBoardListsOutput) string {
	var b strings.Builder
	b.WriteString("## Board Lists\n\n")
	toolutil.WriteListSummary(&b, len(out.Lists), out.Pagination)
	b.WriteString("| ID | Label | Position | Max Issues | Max Weight |\n|---|---|---|---|---|\n")
	for _, l := range out.Lists {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %d |\n",
			l.ID, toolutil.EscapeMdTableCell(l.LabelName), l.Position, l.MaxIssueCount, l.MaxIssueWeight)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_get_group_board_list` to view list details")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGroupBoardMarkdown)
	toolutil.RegisterMarkdown(FormatListGroupBoardsMarkdown)
	toolutil.RegisterMarkdown(FormatBoardListMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardListsMarkdown)
}

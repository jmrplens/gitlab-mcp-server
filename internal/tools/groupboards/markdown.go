package groupboards

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatGroupBoardMarkdown formats a single group board as markdown.
func FormatGroupBoardMarkdown(out GroupBoardOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Board: %s (ID: %d)\n\n", toolutil.EscapeMdTableCell(out.Name), out.ID)
	if out.Group != nil {
		fmt.Fprintf(&b, "**Group**: %s (ID: %d)\n", out.Group.Name, out.Group.ID)
	}
	if out.Milestone != nil {
		fmt.Fprintf(&b, "**Milestone**: %s (ID: %d)\n", out.Milestone.Title, out.Milestone.ID)
	}
	if len(out.Labels) > 0 {
		names := make([]string, 0, len(out.Labels))
		for _, l := range out.Labels {
			if l != nil {
				names = append(names, l.Name)
			}
		}
		fmt.Fprintf(&b, "**Labels**: %s\n", strings.Join(names, ", "))
	}
	if len(out.Lists) > 0 {
		b.WriteString("\n### Lists\n\n| ID | Label | Position | Max Issues | Max Weight |\n|---|---|---|---|---|\n")
		for _, l := range out.Lists {
			fmt.Fprintf(&b, "| %d | %s | %d | %d | %d |\n",
				l.ID, toolutil.EscapeMdTableCell(boardListLabelName(l)), l.Position, l.MaxIssueCount, l.MaxIssueWeight)
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
			toolutil.EscapeMdTableCell(groupName(bd.Group)),
			toolutil.EscapeMdTableCell(milestoneTitle(bd.Milestone)),
			len(bd.Lists))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_group_board_get` to view details of a specific board")
	return b.String()
}

// FormatBoardListMarkdown formats a single board list as markdown.
func FormatBoardListMarkdown(out BoardListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Board List (ID: %d)\n\n", out.ID)
	if out.Label != nil {
		fmt.Fprintf(&b, "**Label**: %s\n", out.Label.Name)
	}
	fmt.Fprintf(&b, "**Position**: %d\n", out.Position)
	if out.MaxIssueCount > 0 {
		fmt.Fprintf(&b, "**Max Issue Count**: %d\n", out.MaxIssueCount)
	}
	if out.MaxIssueWeight > 0 {
		fmt.Fprintf(&b, "**Max Issue Weight**: %d\n", out.MaxIssueWeight)
	}
	if out.Assignee != nil {
		fmt.Fprintf(&b, "**Assignee**: @%s (ID: %d)\n", out.Assignee.Username, out.Assignee.ID)
	}
	if out.Iteration != nil {
		fmt.Fprintf(&b, "**Iteration**: %s (ID: %d)\n", out.Iteration.Title, out.Iteration.ID)
	}
	if out.Milestone != nil {
		fmt.Fprintf(&b, "**Milestone**: %s (ID: %d)\n", out.Milestone.Title, out.Milestone.ID)
	}
	toolutil.WriteHints(&b, "Use `gitlab_group_board_list_update` to reorder or modify this list")
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
			l.ID, toolutil.EscapeMdTableCell(boardListLabelName(l)), l.Position, l.MaxIssueCount, l.MaxIssueWeight)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_group_board_list_get` to view list details")
	return b.String()
}

// boardListLabelName returns the list's label name, or "" when the list has no
// label scope (assignee/milestone/iteration lists).
func boardListLabelName(l BoardListOutput) string {
	if l.Label != nil {
		return l.Label.Name
	}
	return ""
}

// groupName returns the board group's name, or "" when absent.
func groupName(g *GroupRefOutput) string {
	if g != nil {
		return g.Name
	}
	return ""
}

// milestoneTitle returns the milestone title, or "" when absent.
func milestoneTitle(m *MilestoneOutput) string {
	if m != nil {
		return m.Title
	}
	return ""
}

func init() {
	toolutil.RegisterMarkdown(FormatGroupBoardMarkdown)
	toolutil.RegisterMarkdown(FormatListGroupBoardsMarkdown)
	toolutil.RegisterMarkdown(FormatBoardListMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardListsMarkdown)
}

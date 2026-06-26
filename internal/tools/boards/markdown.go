package boards

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// boardProjectLabel renders the most descriptive project reference, or "".
func boardProjectLabel(p *ProjectOutput) string {
	if p == nil {
		return ""
	}
	if p.PathWithNamespace != "" {
		return p.PathWithNamespace
	}
	return p.Name
}

// boardListLabelName returns the label name for a list, or "" when unset.
func boardListLabelName(l BoardListOutput) string {
	if l.Label != nil {
		return l.Label.Name
	}
	return ""
}

// FormatBoardMarkdown formats a single board as markdown.
func FormatBoardMarkdown(out BoardOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Board: %s\n\n", toolutil.EscapeMdTableCell(out.Name))
	if project := boardProjectLabel(out.Project); project != "" {
		fmt.Fprintf(&b, "**Project**: %s\n", project)
	}
	if out.Milestone != nil && out.Milestone.Title != "" {
		fmt.Fprintf(&b, "**Milestone**: %s\n", out.Milestone.Title)
	}
	if out.Assignee != nil && out.Assignee.Username != "" {
		fmt.Fprintf(&b, "**Assignee**: @%s\n", out.Assignee.Username)
	}
	if out.Weight > 0 {
		fmt.Fprintf(&b, "**Weight**: %d\n", out.Weight)
	}
	if len(out.Labels) > 0 {
		names := make([]string, 0, len(out.Labels))
		for _, lbl := range out.Labels {
			if lbl != nil {
				names = append(names, lbl.Name)
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "**Labels**: %s\n", strings.Join(names, ", "))
		}
	}
	fmt.Fprintf(&b, "**Hide Backlog**: %t | **Hide Closed**: %t\n", out.HideBacklogList, out.HideClosedList)
	if len(out.Lists) > 0 {
		b.WriteString("\n### Lists\n\n| Label | Position | Max Issues | Max Weight |\n|---|---|---|---|\n")
		for _, l := range out.Lists {
			label := toolutil.EscapeMdTableCell(boardListLabelName(l))
			if label == "" {
				label = fmt.Sprintf("#%d", l.ID)
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
				label, l.Position, l.MaxIssueCount, l.MaxIssueWeight)
		}
	}
	toolutil.WriteHints(
		&b,
		"Use action 'board_list_create' to add columns to this board",
		"Use action 'board_update' to modify board settings",
		"Use action 'board_delete' to remove this board",
	)
	return b.String()
}

// FormatListBoardsMarkdown formats a paginated list of boards.
func FormatListBoardsMarkdown(out ListBoardsOutput) string {
	var b strings.Builder
	b.WriteString("## Issue Boards\n\n")
	toolutil.WriteListSummary(&b, len(out.Boards), out.Pagination)
	b.WriteString("| Name | Project | Milestone | Assignee | Lists |\n|---|---|---|---|---|\n")
	for _, bd := range out.Boards {
		var milestone, assignee string
		if bd.Milestone != nil {
			milestone = bd.Milestone.Title
		}
		if bd.Assignee != nil {
			assignee = bd.Assignee.Username
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d |\n",
			toolutil.EscapeMdTableCell(bd.Name),
			toolutil.EscapeMdTableCell(boardProjectLabel(bd.Project)),
			toolutil.EscapeMdTableCell(milestone),
			toolutil.EscapeMdTableCell(assignee),
			len(bd.Lists))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'board_get' with board_id for full details",
		"Use action 'board_create' to add a new board",
	)
	return b.String()
}

// FormatBoardListMarkdown formats a single board list as markdown.
func FormatBoardListMarkdown(out BoardListOutput) string {
	var b strings.Builder
	if label := boardListLabelName(out); label != "" {
		fmt.Fprintf(&b, "## Board List: %s\n\n", toolutil.EscapeMdTableCell(label))
	} else {
		fmt.Fprintf(&b, "## Board List #%d\n\n", out.ID)
	}
	fmt.Fprintf(&b, "**Position**: %d\n", out.Position)
	if out.MaxIssueCount > 0 {
		fmt.Fprintf(&b, "**Max Issue Count**: %d\n", out.MaxIssueCount)
	}
	if out.MaxIssueWeight > 0 {
		fmt.Fprintf(&b, "**Max Issue Weight**: %d\n", out.MaxIssueWeight)
	}
	if out.Assignee != nil && out.Assignee.Username != "" {
		fmt.Fprintf(&b, "**Assignee**: @%s\n", out.Assignee.Username)
	}
	if out.Milestone != nil && out.Milestone.Title != "" {
		fmt.Fprintf(&b, "**Milestone**: %s\n", out.Milestone.Title)
	}
	toolutil.WriteHints(
		&b,
		"Use action 'board_list_update' to change position or limits",
		"Use action 'board_list_delete' to remove this list",
	)
	return b.String()
}

// FormatListBoardListsMarkdown formats a paginated list of board lists.
func FormatListBoardListsMarkdown(out ListBoardListsOutput) string {
	var b strings.Builder
	b.WriteString("## Board Lists\n\n")
	toolutil.WriteListSummary(&b, len(out.Lists), out.Pagination)
	b.WriteString("| Label | Position | Max Issues | Max Weight |\n|---|---|---|---|\n")
	for _, l := range out.Lists {
		label := toolutil.EscapeMdTableCell(boardListLabelName(l))
		if label == "" {
			label = fmt.Sprintf("#%d", l.ID)
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
			label, l.Position, l.MaxIssueCount, l.MaxIssueWeight)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'board_list_create' to add a new list",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatBoardMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardsMarkdown)
	toolutil.RegisterMarkdown(FormatBoardListMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardListsMarkdown)
}

package boards

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatBoardMarkdown formats a single board as markdown.
func FormatBoardMarkdown(out BoardOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Board: %s\n\n", toolutil.EscapeMdTableCell(out.Name))
	if out.ProjectPath != "" {
		fmt.Fprintf(&b, "**Project**: %s\n", out.ProjectPath)
	} else if out.ProjectName != "" {
		fmt.Fprintf(&b, "**Project**: %s\n", out.ProjectName)
	}
	if out.MilestoneTitle != "" {
		fmt.Fprintf(&b, "**Milestone**: %s\n", out.MilestoneTitle)
	}
	if out.AssigneeUser != "" {
		fmt.Fprintf(&b, "**Assignee**: @%s\n", out.AssigneeUser)
	}
	if out.Weight > 0 {
		fmt.Fprintf(&b, "**Weight**: %d\n", out.Weight)
	}
	if len(out.Labels) > 0 {
		fmt.Fprintf(&b, "**Labels**: %s\n", strings.Join(out.Labels, ", "))
	}
	fmt.Fprintf(&b, "**Hide Backlog**: %t | **Hide Closed**: %t\n", out.HideBacklogList, out.HideClosedList)
	if len(out.Lists) > 0 {
		b.WriteString("\n### Lists\n\n| Label | Position | Max Issues | Max Weight |\n|---|---|---|---|\n")
		for _, l := range out.Lists {
			label := toolutil.EscapeMdTableCell(l.LabelName)
			if label == "" {
				label = fmt.Sprintf("#%d", l.ID)
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
				label, l.Position, l.MaxIssueCount, l.MaxIssueWeight)
		}
	}
	toolutil.WriteHints(&b,
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
		project := bd.ProjectPath
		if project == "" {
			project = bd.ProjectName
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d |\n",
			toolutil.EscapeMdTableCell(bd.Name),
			toolutil.EscapeMdTableCell(project),
			toolutil.EscapeMdTableCell(bd.MilestoneTitle),
			toolutil.EscapeMdTableCell(bd.AssigneeUser),
			len(bd.Lists))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'board_get' with board_id for full details",
		"Use action 'board_create' to add a new board",
	)
	return b.String()
}

// FormatBoardListMarkdown formats a single board list as markdown.
func FormatBoardListMarkdown(out BoardListOutput) string {
	var b strings.Builder
	if out.LabelName != "" {
		fmt.Fprintf(&b, "## Board List: %s\n\n", toolutil.EscapeMdTableCell(out.LabelName))
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
	if out.AssigneeUser != "" {
		fmt.Fprintf(&b, "**Assignee**: @%s\n", out.AssigneeUser)
	}
	if out.MilestoneTitle != "" {
		fmt.Fprintf(&b, "**Milestone**: %s\n", out.MilestoneTitle)
	}
	toolutil.WriteHints(&b,
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
		label := toolutil.EscapeMdTableCell(l.LabelName)
		if label == "" {
			label = fmt.Sprintf("#%d", l.ID)
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n",
			label, l.Position, l.MaxIssueCount, l.MaxIssueWeight)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
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

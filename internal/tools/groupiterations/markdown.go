// markdown.go provides Markdown formatting functions for group iteration
// MCP tool output.
package groupiterations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats a list of group iterations.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Iterations) == 0 {
		return "No group iterations found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Group Iterations\n\n")
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("| ID | IID | Title | State | Start | Due | URL |\n")
	sb.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, it := range out.Iterations {
		state := iterationState(it.State)
		url := toolutil.EscapeMdTableCell(it.WebURL)
		if it.WebURL != "" {
			url = fmt.Sprintf("[%s](%s)", state, it.WebURL)
		}
		fmt.Fprintf(&sb, "| %d | %d | %s | %s | %s | %s | %s |\n",
			it.ID, it.IID, toolutil.EscapeMdTableCell(it.Title),
			state, toolutil.FormatTime(it.StartDate), toolutil.FormatTime(it.DueDate), url)
	}
	toolutil.WriteListSummary(&sb, len(out.Iterations), out.Pagination)
	toolutil.WritePagination(&sb, out.Pagination)
	return sb.String()
}

// FormatOutputMarkdown formats a single group iteration.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Iteration #%d — %s\n\n", out.IID, toolutil.EscapeMdTableCell(out.Title))
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "| IID | %d |\n", out.IID)
	fmt.Fprintf(&sb, "| Title | %s |\n", toolutil.EscapeMdTableCell(out.Title))
	fmt.Fprintf(&sb, "| State | %s |\n", iterationState(out.State))
	fmt.Fprintf(&sb, "| Group ID | %d |\n", out.GroupID)
	fmt.Fprintf(&sb, "| Start | %s |\n", toolutil.FormatTime(out.StartDate))
	fmt.Fprintf(&sb, "| Due | %s |\n", toolutil.FormatTime(out.DueDate))
	if out.WebURL != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdURL, out.WebURL)
	}
	fmt.Fprintf(&sb, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	if out.Description != "" {
		sb.WriteString("\n### Description\n\n")
		sb.WriteString(toolutil.WrapGFMBody(out.Description))
		sb.WriteByte('\n')
	}
	toolutil.WriteHints(&sb,
		"Use `gitlab_list_group_iterations` to view all iterations",
	)
	return sb.String()
}

func iterationState(s int64) string {
	switch s {
	case 1:
		return "opened"
	case 2:
		return "upcoming"
	case 3:
		return "current"
	case 4:
		return "closed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}

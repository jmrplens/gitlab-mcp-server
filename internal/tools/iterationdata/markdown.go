package iterationdata

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown formats a list of iterations with the provided title and empty text.
func FormatListMarkdown(title, emptyText string, iterations []Output, pagination toolutil.PaginationOutput) string {
	if len(iterations) == 0 {
		return emptyText + "\n"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "## %s\n\n", title)
	toolutil.WriteHints(&builder, toolutil.HintPreserveLinks)
	builder.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Title", "State", "Start", "Due", "URL"))
	for _, iteration := range iterations {
		state := StateName(iteration.State)
		url := toolutil.MdTitleLink(state, iteration.WebURL)
		fmt.Fprintf(&builder, "| %d | %d | %s | %s | %s | %s | %s |\n",
			iteration.ID, iteration.IID, toolutil.EscapeMdTableCell(iteration.Title),
			state, toolutil.FormatTime(iteration.StartDate), toolutil.FormatTime(iteration.DueDate), url)
	}
	toolutil.WriteListSummary(&builder, len(iterations), pagination)
	toolutil.WritePagination(&builder, pagination)
	return builder.String()
}

// FormatOutputMarkdown formats a single iteration and appends optional hints.
func FormatOutputMarkdown(output Output, hints ...string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "## Iteration #%d: %s\n\n", output.IID, toolutil.EscapeMdTableCell(output.Title))
	fmt.Fprintf(&builder, toolutil.FmtMdID, output.ID)
	toolutil.WriteMdFieldInt(&builder, "IID", output.IID)
	toolutil.WriteMdField(&builder, "Title", output.Title)
	toolutil.WriteMdField(&builder, "State", StateName(output.State))
	toolutil.WriteMdFieldInt(&builder, "Group ID", output.GroupID)
	toolutil.WriteMdFieldTime(&builder, "Start", output.StartDate)
	toolutil.WriteMdFieldTime(&builder, "Due", output.DueDate)
	if output.WebURL != "" {
		toolutil.WriteMdURL(&builder, output.WebURL)
	}
	toolutil.WriteMdFieldTime(&builder, "Created", output.CreatedAt)
	if output.Description != "" {
		builder.WriteString("\n### Description\n\n")
		builder.WriteString(toolutil.WrapGFMBody(output.Description))
		builder.WriteByte('\n')
	}
	if len(hints) > 0 {
		toolutil.WriteHints(&builder, hints...)
	}
	return builder.String()
}

// StateName maps GitLab iteration state integers to human-readable names.
func StateName(state int64) string {
	switch state {
	case 1:
		return "opened"
	case 2:
		return "upcoming"
	case 3:
		return "current"
	case 4:
		return "closed"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

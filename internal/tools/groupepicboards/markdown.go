package groupepicboards

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group epic board as a Markdown summary.
func FormatOutputMarkdown(b Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Epic Board #%d — %s\n\n", b.ID, toolutil.EscapeMdTableCell(b.Name))
	if len(b.Labels) > 0 {
		fmt.Fprintf(&sb, "- **Labels**: %s\n", strings.Join(b.Labels, ", "))
	}
	if len(b.Lists) > 0 {
		sb.WriteString("\n### Board Lists\n\n")
		sb.WriteString("| ID | Label | Position |\n")
		sb.WriteString(toolutil.TblSep3Col)
		for _, l := range b.Lists {
			fmt.Fprintf(&sb, "| %d | %s | %d |\n", l.ID, toolutil.EscapeMdTableCell(l.Label), l.Position)
		}
	}
	toolutil.WriteHints(&sb,
		"Use action 'list' to see all epic boards in a group",
	)
	return sb.String()
}

// FormatListMarkdown renders a list of group epic boards as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Epic Boards (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Boards), out.Pagination)
	if len(out.Boards) == 0 {
		b.WriteString("No epic boards found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Labels | Lists |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, board := range out.Boards {
		labels := ""
		if len(board.Labels) > 0 {
			labels = strings.Join(board.Labels, ", ")
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %d |\n",
			board.ID,
			toolutil.EscapeMdTableCell(board.Name),
			toolutil.EscapeMdTableCell(labels),
			len(board.Lists),
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with board_id to see board details and lists",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

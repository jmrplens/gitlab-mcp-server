// markdown.go provides Markdown formatting functions for issue note MCP tool output.

package issuenotes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single issue note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Note #%d\n\n", n.ID)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, n.Author)
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(n.CreatedAt))
	if n.System {
		b.WriteString("- **System note**\n")
	}
	if n.Internal {
		b.WriteString("- **Internal note**\n")
	}
	if n.Resolvable {
		resolved := "unresolved"
		if n.Resolved {
			resolved = "resolved"
		}
		fmt.Fprintf(&b, "- **Resolvable**: %s\n", resolved)
	}
	fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(n.Body))
	toolutil.WriteHints(&b,
		"Use action 'note_update' with note_id to edit this note",
		"Use action 'note_delete' with note_id to remove this note",
	)
	return b.String()
}

// FormatListMarkdown renders a list of issue notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Notes (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Notes), out.Pagination)
	if len(out.Notes) == 0 {
		b.WriteString("No issue notes found.\n")
		return b.String()
	}
	b.WriteString("| ID | Author | Created | System | Internal |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, n := range out.Notes {
		fmt.Fprintf(&b, "| %d | %s | %s | %v | %v |\n", n.ID, toolutil.EscapeMdTableCell(n.Author), toolutil.FormatTime(n.CreatedAt), n.System, n.Internal)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'note_get' with note_id to read a specific note",
		"Use action 'note_create' to add a new note to this issue",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

package epicnotes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single epic note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Epic Note #%d\n\n", n.ID)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, n.Author)
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(n.CreatedAt))
	if n.System {
		b.WriteString("- **System note**\n")
	}
	fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(n.Body))
	toolutil.WriteHints(&b,
		"Use action 'epic_note_update' with note_id to edit this note",
		"Use action 'epic_note_delete' with note_id to remove this note",
	)
	return b.String()
}

// FormatListMarkdown renders a list of epic notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Epic Notes (%d)\n\n", len(out.Notes))
	if len(out.Notes) == 0 {
		b.WriteString("No epic notes found.\n")
		return b.String()
	}
	b.WriteString("| ID | Author | Created | System |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, n := range out.Notes {
		fmt.Fprintf(&b, "| %d | %s | %s | %v |\n", n.ID, toolutil.EscapeMdTableCell(n.Author), toolutil.FormatTime(n.CreatedAt), n.System)
	}
	b.WriteString("\n")
	b.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Notes)))
	b.WriteString("\n")
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'epic_note_get' with note_id to read a specific note",
		"Use action 'epic_note_create' to add a new note to this epic",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

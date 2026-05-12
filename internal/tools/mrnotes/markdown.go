package mrnotes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single MR note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Note #%d\n\n", n.ID)
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
		if n.ResolvedBy != "" {
			fmt.Fprintf(&b, "- **Resolved By**: @%s\n", n.ResolvedBy)
		}
	}
	fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(n.Body))
	toolutil.WriteHints(&b,
		"Use the selected tool surface's merge-request note update action with the same project_id, merge_request_iid, and this note_id to edit this note",
		"Use the selected tool surface's merge-request note delete action with the same project_id, merge_request_iid, this note_id, and explicit confirm=true to remove this note",
	)
	return b.String()
}

// FormatListMarkdown renders a list of MR notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Notes (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Notes), out.Pagination)
	if len(out.Notes) == 0 {
		b.WriteString("No merge request notes found.\n")
		return b.String()
	}
	b.WriteString("| ID | Author | Created | System |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, n := range out.Notes {
		fmt.Fprintf(&b, "| %d | %s | %s | %v |\n", n.ID, toolutil.EscapeMdTableCell(n.Author), toolutil.FormatTime(n.CreatedAt), n.System)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's merge-request note get action with the same project_id, merge_request_iid, and note_id to read a specific note",
		"Use the selected tool surface's merge-request note create action with the same project_id and merge_request_iid to add a new note to this MR",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

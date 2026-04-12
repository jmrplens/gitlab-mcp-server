// markdown.go provides Markdown formatting functions for merge request discussion MCP tool output.

package mrdiscussions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatNoteMarkdown renders a single discussion note as Markdown.
func FormatNoteMarkdown(n NoteOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion Note #%d\n\n", n.ID)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, n.Author)
	fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(n.CreatedAt))
	fmt.Fprintf(&b, "- **Resolved**: %v\n", n.Resolved)
	fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(n.Body))
	toolutil.WriteHints(&b,
		"Use action 'discussion_note_update' with note_id to edit this note",
		"Use action 'discussion_resolve' with discussion_id to resolve this discussion",
	)
	return b.String()
}

// FormatOutputMarkdown renders a discussion thread with all its notes as Markdown.
func FormatOutputMarkdown(d Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion %s\n\n", toolutil.EscapeMdHeading(d.ID))
	fmt.Fprintf(&b, "- **Notes**: %d\n", len(d.Notes))
	fmt.Fprintf(&b, "- **Individual Note**: %v\n", d.IndividualNote)
	for i, n := range d.Notes {
		fmt.Fprintf(&b, "\n### Note %d (by %s)\n\n%s\n", i+1, n.Author, toolutil.WrapGFMBody(n.Body))
	}
	toolutil.WriteHints(&b,
		"Use action 'discussion_reply' to reply to this discussion",
		"Use action 'discussion_resolve' with discussion_id to resolve/unresolve",
	)
	return b.String()
}

// FormatListMarkdown renders a list of discussion threads as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MR Discussions (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Discussions), out.Pagination)
	if len(out.Discussions) == 0 {
		b.WriteString("No merge request discussions found.\n")
		return b.String()
	}
	b.WriteString("| ID | Notes | Individual |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, d := range out.Discussions {
		fmt.Fprintf(&b, toolutil.FmtRow3Str, toolutil.EscapeMdTableCell(d.ID), strconv.Itoa(len(d.Notes)), strconv.FormatBool(d.IndividualNote))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'discussion_get' with discussion_id to see full discussion notes",
		"Use action 'discussion_create' to start a new discussion on this MR",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatNoteMarkdown)
}

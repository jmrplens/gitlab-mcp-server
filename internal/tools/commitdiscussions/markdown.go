package commitdiscussions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// firstNoteAuthor returns the author username of a discussion's first note, or
// "" when the thread has no notes.
func firstNoteAuthor(d Output) string {
	if len(d.Notes) == 0 || d.Notes[0] == nil {
		return ""
	}
	return noteAuthorUsername(*d.Notes[0])
}

// noteAuthorUsername returns the note author's username, read from the
// canonical author object (nil-guarded).
func noteAuthorUsername(n NoteOutput) string {
	if n.Author != nil {
		return n.Author.Username
	}
	return ""
}

// FormatNoteMarkdownString renders a single commit discussion note as Markdown.
func FormatNoteMarkdownString(n NoteOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion Note #%d\n\n", n.ID)
	fmt.Fprintf(&b, toolutil.FmtMdAuthor, noteAuthorUsername(n))
	if n.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(n.CreatedAt))
	}
	fmt.Fprintf(&b, "- **Resolved**: %v\n", n.Resolved)
	fmt.Fprintf(&b, "\n%s\n", toolutil.WrapGFMBody(n.Body))
	toolutil.WriteHints(
		&b,
		"Use `gitlab_update_commit_discussion_note` with note_id to edit this note",
		"Use `gitlab_add_commit_discussion_note` with discussion_id to reply to this discussion",
	)
	return b.String()
}

// FormatMarkdownString renders a commit discussion thread with all its notes as
// Markdown.
func FormatMarkdownString(d Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion %s\n\n", toolutil.EscapeMdHeading(d.ID))
	fmt.Fprintf(&b, "- **Notes**: %d\n", len(d.Notes))
	fmt.Fprintf(&b, "- **Individual Note**: %v\n", d.IndividualNote)
	for i, n := range d.Notes {
		fmt.Fprintf(&b, "\n### Note %d (by %s)\n\n%s\n", i+1, toolutil.EscapeMdHeading(noteAuthorUsername(*n)), toolutil.WrapGFMBody(n.Body))
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_add_commit_discussion_note` to reply to this discussion",
		"Use `gitlab_update_commit_discussion_note` to edit a note",
	)
	return b.String()
}

// FormatListMarkdownString renders a list of commit discussion threads as a
// Markdown table.
func FormatListMarkdownString(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Commit Discussions (%d)\n\n", len(out.Discussions))
	toolutil.WriteListSummary(&b, len(out.Discussions), out.Pagination)
	if len(out.Discussions) == 0 {
		b.WriteString("No commit discussions found.\n")
		return b.String()
	}
	b.WriteString("| ID | Author | Notes |\n")
	b.WriteString(toolutil.TblSep3Col)
	for _, d := range out.Discussions {
		fmt.Fprintf(&b, toolutil.FmtRow3Str, toolutil.EscapeMdTableCell(d.ID), toolutil.EscapeMdTableCell(firstNoteAuthor(d)), strconv.Itoa(len(d.Notes)))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use `gitlab_get_commit_discussion` with discussion_id to view full discussion details",
		"Use `gitlab_create_commit_discussion` to start a new discussion on this commit",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatNoteMarkdownString)
}

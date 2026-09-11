package epicnotes

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// noteAuthorUsername returns the note author's username, read from the
// canonical author object (nil-guarded). Epic notes are served through the Work
// Items GraphQL API and their author is this package's own object rather than
// the shared note shape, so the accessor stays here; a note GitLab answered with
// no author renders no author row at all.
func noteAuthorUsername(n Output) string {
	if n.Author != nil {
		return n.Author.Username
	}
	return ""
}

// toNoteMarkdown maps an epic note onto the shared note view model, so the card
// is the one every note domain renders.
func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	return toolutil.NewNoteMarkdown(n.ID, n.Body, noteAuthorUsername(n), n.CreatedAt,
		toolutil.NoteMarkdownFlags{System: n.System}, "")
}

// FormatOutputMarkdown renders a single epic note as the shared note card: the
// author as a handle, the time, the system marker when it holds, and the body
// as the card's long text.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title: "Epic Note",
		Hints: []string{
			"Use action 'epic_note_update' with note_id to edit this note",
			"Use action 'epic_note_delete' with note_id to remove this note",
		},
	})
}

// FormatListMarkdown renders a list of epic notes as a Markdown table. The
// notes widget is a keyset connection, which counts nothing it has not walked,
// so the heading carries the count shown and the cursor line below the rows says
// whether more follow. The table carries no link, so its footer carries no
// instruction to keep them.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Notes) == 0 {
		return toolutil.EmptyMessage("epic notes")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Epic Notes", len(out.Notes), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Author", "Created", "System"))
	for _, n := range out.Notes {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(n.ID, 10),
			toolutil.EscapeMdTableCell(noteAuthorUsername(n)),
			toolutil.FormatTime(n.CreatedAt),
			toolutil.BoolEmoji(n.System),
		))
	}
	toolutil.WriteGraphQLPagination(&b, toolutil.GraphQLPaginationOutput{
		HasNextPage: out.Pagination.HasNextPage,
		EndCursor:   out.Pagination.EndCursor,
	}, len(out.Notes))
	toolutil.WriteHints(
		&b,
		"Use action 'epic_note_get' with note_id to read a specific note",
		"Use action 'epic_note_create' to add a new note to this epic",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

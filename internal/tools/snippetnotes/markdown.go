package snippetnotes

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// FormatOutputMarkdown renders a single snippet note as a Markdown summary.
// Its next steps name the canonical action IDs every surface resolves: the
// card was never served while the note type was shared, so its bare meta
// action names had reached no reader.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title: "Snippet Note",
		Hints: []string{
			toolutil.HintAction(actionSnippetNoteUpdate, "edit this note"),
			toolutil.HintAction(actionSnippetNoteDelete, "remove this note"),
		},
	})
}

// FormatListMarkdown renders a list of snippet notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatNoteListMarkdown(toolutil.NoteMarkdowns(out.Notes, toNoteMarkdown), out.Pagination, toolutil.NoteListMarkdownOptions{
		Title:        "Snippet Notes",
		EmptyMessage: "No snippet notes found.\n",
		Hints: toolutil.ListHints(
			toolutil.HintAction(actionSnippetNoteGet, "read one of these notes in full"),
			toolutil.HintAction(actionSnippetNoteCreate, "add a new note to this snippet"),
		),
	})
}

func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	flags := toolutil.NoteMarkdownFlags{System: n.System}
	return toolutil.NewNoteMarkdown(n.ID, n.Body, toolutil.NoteOutput(n).AuthorUsername(), n.CreatedAt, flags, "")
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

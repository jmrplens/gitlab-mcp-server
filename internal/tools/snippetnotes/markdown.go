package snippetnotes

import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

// FormatOutputMarkdown renders a single snippet note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title: "Snippet Note",
		Hints: []string{
			"Use action 'note_update' with note_id to edit this note",
			"Use action 'note_delete' with note_id to remove this note",
		},
	})
}

// FormatListMarkdown renders a list of snippet notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatNoteListMarkdown(toolutil.NoteMarkdowns(out.Notes, toNoteMarkdown), out.Pagination, toolutil.NoteListMarkdownOptions{
		Title:        "Snippet Notes",
		EmptyMessage: "No snippet notes found.\n",
		Hints: toolutil.ListHints(
			"Use action 'note_get' with note_id to read a specific note",
			"Use action 'note_create' to add a new note to this snippet",
		),
	})
}

func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	return toolutil.NewNoteMarkdown(n.ID, n.Body, n.Author, n.CreatedAt, n.System, false, false, false, "")
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

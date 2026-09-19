package mrnotes

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// FormatOutputMarkdown renders a single MR note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title:             "MR Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints: []string{
			toolutil.HintAction(actionMRNoteUpdate, "edit this note"),
			toolutil.HintAction(actionMRNoteDelete, "remove this note (it takes confirm=true)"),
		},
	})
}

// FormatListMarkdown renders a list of MR notes as a Markdown table.
//
// IncludeInternal matches issuenotes: a merge request carries internal notes
// too, and without the column a reader could not tell one from a note everybody
// can see. The table carries no link, so the hints carry no instruction to keep
// them — [toolutil.FormatNoteListMarkdown] drops that one — and they name the
// canonical action IDs every surface resolves rather than describing a tool.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatNoteListMarkdown(toolutil.NoteMarkdowns(out.Notes, toNoteMarkdown), out.Pagination, toolutil.NoteListMarkdownOptions{
		Title:           "MR Notes",
		EmptyMessage:    toolutil.EmptyMessage("merge request notes"),
		IncludeInternal: true,
		Hints: []string{
			toolutil.HintAction(actionMRNoteGet, "read one of these notes in full"),
			toolutil.HintAction(actionMRNoteCreate, "add a new note to this merge request"),
		},
	})
}

func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	flags := toolutil.NoteMarkdownFlags{System: n.System, Internal: n.Internal, Resolvable: n.Resolvable, Resolved: n.Resolved}
	return toolutil.NewNoteMarkdown(n.ID, n.Body, n.AuthorUsername(), n.CreatedAt, flags, n.ResolvedByUsername())
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

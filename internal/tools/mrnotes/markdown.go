package mrnotes

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

// FormatOutputMarkdown renders a single MR note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title:             "MR Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints: []string{
			"Use the selected tool surface's merge-request note update action with the same project_id, merge_request_iid, and this note_id to edit this note",
			"Use the selected tool surface's merge-request note delete action with the same project_id, merge_request_iid, this note_id, and explicit confirm=true to remove this note",
		},
	})
}

// FormatListMarkdown renders a list of MR notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatNoteListMarkdown(toolutil.NoteMarkdowns(out.Notes, toNoteMarkdown), out.Pagination, toolutil.NoteListMarkdownOptions{
		Title:        "MR Notes",
		EmptyMessage: "No merge request notes found.\n",
		Hints: toolutil.ListHints(
			"Use the selected tool surface's merge-request note get action with the same project_id, merge_request_iid, and note_id to read a specific note",
			"Use the selected tool surface's merge-request note create action with the same project_id and merge_request_iid to add a new note to this MR",
		),
	})
}

func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	flags := toolutil.NoteMarkdownFlags{System: n.System, Internal: n.Internal, Resolvable: n.Resolvable, Resolved: n.Resolved}
	return toolutil.NewNoteMarkdown(n.ID, n.Body, n.Author, n.CreatedAt, flags, n.ResolvedBy)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

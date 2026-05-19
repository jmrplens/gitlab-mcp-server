package issuenotes

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

// FormatOutputMarkdown renders a single issue note as a Markdown summary.
func FormatOutputMarkdown(n Output) string {
	return toolutil.FormatNoteMarkdown(toNoteMarkdown(n), toolutil.NoteMarkdownOptions{
		Title:             "Issue Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints: []string{
			"Use the selected tool surface's issue-note update action with the same project_id, issue_iid, and this note_id to edit this note",
			"Use the selected tool surface's issue-note delete action with the same project_id, issue_iid, this note_id, and explicit confirm=true to remove this note",
		},
	})
}

// FormatListMarkdown renders a list of issue notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatNoteListMarkdown(toolutil.NoteMarkdowns(out.Notes, toNoteMarkdown), out.Pagination, toolutil.NoteListMarkdownOptions{
		Title:           "Issue Notes",
		EmptyMessage:    "No issue notes found.\n",
		IncludeInternal: true,
		Hints: toolutil.ListHints(
			"Use the selected tool surface's issue-note get action with the same project_id, issue_iid, and note_id to read a specific note",
			"Use the selected tool surface's issue-note create action with the same project_id and issue_iid to add a new note to this issue",
		),
	})
}

func toNoteMarkdown(n Output) toolutil.NoteMarkdown {
	flags := toolutil.NoteMarkdownFlags{System: n.System, Internal: n.Internal, Resolvable: n.Resolvable, Resolved: n.Resolved}
	return toolutil.NewNoteMarkdown(n.ID, n.Body, n.Author, n.CreatedAt, flags, "")
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

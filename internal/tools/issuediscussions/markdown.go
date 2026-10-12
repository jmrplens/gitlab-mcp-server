package issuediscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// The next steps below name the canonical action IDs every surface resolves.
// The thread and note cards were never served while this package's types were
// aliases of the shared thread shapes, the commit discussion cards answering
// in their place, so their bare meta action names had reached no reader.

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatRESTDiscussionListMarkdown(
		out.Discussions, out.Pagination, toMarkdownDiscussion, "Issue Discussions", "No issue discussions found.\n",
		toolutil.HintAction(actionDiscussionGet, "see a full discussion"),
		toolutil.HintAction(actionDiscussionAddNote, "reply to a discussion"),
	)
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	return toolutil.FormatDiscussionMarkdown(
		toMarkdownDiscussion(out),
		toolutil.HintAction(actionDiscussionAddNote, "reply to this discussion"),
		toolutil.HintAction(actionDiscussionUpdateNote, "edit one of its notes"),
	)
}

// FormatNoteMarkdownString renders a note as the shared note card.
func FormatNoteMarkdownString(out NoteOutput) string {
	return toolutil.FormatNoteMarkdown(toMarkdownNote(out), toolutil.NoteMarkdownOptions{
		Title:             "Discussion Note",
		IncludeResolvable: true,
		Hints: []string{
			toolutil.HintAction(actionDiscussionUpdateNote, "edit this note"),
			toolutil.HintAction(actionDiscussionDeleteNote, "remove this note"),
		},
	})
}

func toMarkdownDiscussion(out Output) toolutil.DiscussionMarkdown {
	return toolutil.DiscussionThreadOutput(out).MarkdownDiscussion()
}

func toMarkdownNote(out NoteOutput) toolutil.DiscussionNoteMarkdown {
	return toolutil.DiscussionThreadNoteOutput(out).MarkdownNote()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatNoteMarkdownString)
}

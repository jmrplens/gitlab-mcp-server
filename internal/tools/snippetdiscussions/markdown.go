package snippetdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Snippet Discussions", "No snippet discussions found.\n", "Use `snippet.discussion_get` to view full discussion details", "Use `snippet.discussion_add_note` to reply to this discussion", "Use `snippet.discussion_update_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	discussions := toolutil.DiscussionThreadOutputMarkdowns(out.Discussions)
	return markdownRenderer.FormatRESTList(discussions, out.Pagination)
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	discussion := out.MarkdownDiscussion()
	return markdownRenderer.FormatDiscussion(discussion)
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	note := out.MarkdownNote()
	return markdownRenderer.FormatNote(note)
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

package snippetdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Snippet Discussions", "No snippet discussions found.\n", "Use `gitlab_get_snippet_discussion` to view full discussion details", "Use `gitlab_add_snippet_discussion_note` to reply to this discussion", "Use `gitlab_update_snippet_discussion_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	discussions := toolutil.DiscussionOutputMarkdowns(out.Discussions)
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

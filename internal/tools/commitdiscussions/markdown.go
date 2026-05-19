package commitdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Commit Discussions", "No commit discussions found.\n", "Use `gitlab_get_commit_discussion` to view full discussion details", "Use `gitlab_add_commit_discussion_note` to reply to this discussion", "Use `gitlab_update_commit_discussion_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatRESTList(toolutil.DiscussionOutputMarkdowns(out.Discussions), out.Pagination)
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	return markdownRenderer.FormatDiscussion(out.MarkdownDiscussion())
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	return markdownRenderer.FormatNote(out.MarkdownNote())
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

package epicdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Epic Discussions", "No epic discussions found.\n", "Use `gitlab_get_epic_discussion` to view full discussion details", "Use `gitlab_add_epic_discussion_note` to reply to this discussion", "Use `gitlab_update_epic_discussion_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatGraphQLList(toolutil.DiscussionMarkdowns(out.Discussions, toMarkdownDiscussion), out.Pagination)
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	discussion := toMarkdownDiscussion(out)
	return markdownRenderer.FormatDiscussion(discussion)
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	note := toMarkdownNote(out)
	return markdownRenderer.FormatNote(note)
}

func toMarkdownDiscussion(out Output) toolutil.DiscussionMarkdown {
	return toolutil.NewDiscussionMarkdown(out.ID, toolutil.DiscussionNoteMarkdowns(out.Notes, toMarkdownNote))
}

func toMarkdownNote(out NoteOutput) toolutil.DiscussionNoteMarkdown {
	return toolutil.NewDiscussionNoteMarkdown(out.ID, out.Body, out.Author, out.CreatedAt)
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

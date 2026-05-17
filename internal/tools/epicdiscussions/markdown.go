package epicdiscussions

import "github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatGraphQLDiscussionListMarkdown(out.Discussions, out.Pagination, toMarkdownDiscussion, "Epic Discussions", "No epic discussions found.\n", "Use `gitlab_get_epic_discussion` to view full discussion details")
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	return toolutil.FormatDiscussionMarkdown(toMarkdownDiscussion(out), "Use `gitlab_add_epic_discussion_note` to reply to this discussion")
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	return toolutil.FormatDiscussionNoteMarkdown(toMarkdownNote(out), "Use `gitlab_update_epic_discussion_note` to edit this note")
}

func toMarkdownDiscussion(out Output) toolutil.DiscussionMarkdown {
	return toolutil.NewDiscussionMarkdown(out.ID, toolutil.DiscussionNoteMarkdowns(out.Notes, toMarkdownNote))
}

func toMarkdownNote(out NoteOutput) toolutil.DiscussionNoteMarkdown {
	return toolutil.NewDiscussionNoteMarkdown(out.ID, out.Body, out.Author, out.CreatedAt)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatNoteMarkdownString)
}

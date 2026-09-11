package epicdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Epic Discussions", "No epic discussions found.\n", "Use `gitlab_get_epic_discussion` to view full discussion details", "Use `gitlab_add_epic_discussion_note` to reply to this discussion", "Use `gitlab_update_epic_discussion_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatGraphQLForwardList(toolutil.DiscussionMarkdowns(out.Discussions, toMarkdownDiscussion), out.Pagination)
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
	return toolutil.NewDiscussionMarkdown(out.ID, toolutil.NoteMarkdowns(out.Notes, toMarkdownNote))
}

// toMarkdownNote maps an epic discussion note onto the shared note view model,
// the system flag included: a system note is GitLab's own record of a state
// change rather than something a person wrote, and a card that does not mark it
// reads as if somebody had.
func toMarkdownNote(out NoteOutput) toolutil.DiscussionNoteMarkdown {
	return toolutil.NewNoteMarkdown(out.ID, out.Body, out.Author, out.CreatedAt,
		toolutil.NoteMarkdownFlags{System: out.System}, "")
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

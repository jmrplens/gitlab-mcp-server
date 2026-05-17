package issuediscussions

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats a list of discussions as Markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return toolutil.FormatRESTDiscussionListMarkdown(out.Discussions, out.Pagination, toMarkdownDiscussion, "Issue Discussions", "No issue discussions found.\n",
		"Use action 'discussion_get' with discussion_id to see full discussion",
		"Use action 'discussion_add_note' to reply to a discussion",
	)
}

// FormatMarkdown formats a single discussion as Markdown.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	return toolutil.FormatDiscussionMarkdown(toMarkdownDiscussion(out),
		"Use action 'discussion_add_note' to reply to this discussion",
		"Use action 'discussion_update_note' with note_id to edit a note",
	)
}

// FormatNoteMarkdown formats a single note as Markdown.
func FormatNoteMarkdown(out NoteOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatNoteMarkdownString(out))
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	return toolutil.FormatDiscussionNoteMarkdown(toMarkdownNote(out),
		"Use action 'discussion_update_note' with note_id to edit this note",
		"Use action 'discussion_delete_note' with note_id to remove this note",
	)
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

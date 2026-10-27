// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package snippetdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Snippet Discussions", "No snippet discussions found.\n", "Use `snippet.discussion_get` to view full discussion details", "Use `snippet.discussion_add_note` to reply to this discussion", "Use `snippet.discussion_update_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatRESTList(toolutil.DiscussionMarkdowns(out.Discussions, toMarkdownDiscussion), out.Pagination)
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	return markdownRenderer.FormatDiscussion(toMarkdownDiscussion(out))
}

// toMarkdownDiscussion builds the shared thread view model of a thread.
func toMarkdownDiscussion(out Output) toolutil.DiscussionMarkdown {
	return toolutil.DiscussionThreadOutput(out).MarkdownDiscussion()
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	note := toolutil.DiscussionThreadNoteOutput(out).MarkdownNote()
	return markdownRenderer.FormatNote(note)
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

package epicdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Epic Discussions", "No epic discussions found.\n", "Use `group.epic_discussion_get` to view full discussion details", "Use `group.epic_discussion_add_note` to reply to this discussion", "Use `group.epic_discussion_update_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatGraphQLForwardList(toolutil.DiscussionMarkdowns(out.Discussions, toMarkdownDiscussion), out.Pagination)
}

// quickActionsOnlyHints are the next steps of a body that held only quick
// actions: there is no thread or note to act on, only the epic the commands
// changed.
var quickActionsOnlyHints = []string{
	toolutil.HintAction(actionEpicGet, "see what the quick actions changed on the epic"),
	toolutil.HintAction(actionDiscussionList, "read the epic's threads"),
}

// FormatMarkdownString renders a discussion as Markdown. A thread-opening
// body of quick actions alone opens no thread and keeps no note, so it
// renders the commands alone rather than a thread with nothing in it. The
// test is the notes rather than the id: GitLab can answer a note it kept
// without naming the thread it opened, and that note is shown, with the
// commands beside it.
func FormatMarkdownString(out Output) string {
	if len(out.Notes) == 0 && out.QuickActionsStatus != nil {
		return toolutil.FormatQuickActionsOnlyMarkdown("Epic Discussion", *out.QuickActionsStatus, quickActionsOnlyHints...)
	}
	discussion := toMarkdownDiscussion(out)
	discussion.QuickActions = out.QuickActionsStatus
	return markdownRenderer.FormatDiscussion(discussion)
}

// FormatNoteMarkdownString renders a note as Markdown: the note card every
// note tool renders, with the internal flag shown when it holds, since an
// epic carries internal notes the way an issue does. A reply of quick actions
// alone keeps no note, so it renders the commands alone rather than a note
// numbered 0.
func FormatNoteMarkdownString(out NoteOutput) string {
	if out.ID == 0 && out.QuickActionsStatus != nil {
		return toolutil.FormatQuickActionsOnlyMarkdown("Epic Discussion Note", *out.QuickActionsStatus, quickActionsOnlyHints...)
	}
	note := toMarkdownNote(out)
	note.QuickActions = out.QuickActionsStatus
	return toolutil.FormatNoteMarkdown(note, toolutil.NoteMarkdownOptions{
		Title:             "Discussion Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints:             markdownRenderer.NoteHints,
	})
}

func toMarkdownDiscussion(out Output) toolutil.DiscussionMarkdown {
	return toolutil.NewDiscussionMarkdown(out.ID, toolutil.NoteMarkdowns(out.Notes, toMarkdownNote))
}

// toMarkdownNote maps an epic discussion note onto the shared note view model,
// the system, internal and resolution flags included: a system note is
// GitLab's own record of a state change rather than something a person wrote,
// and a card that does not mark it reads as if somebody had. The note card
// shows the internal flag; the thread card lists its notes by author and
// time, as every discussion family's does, and carries no flags.
func toMarkdownNote(out NoteOutput) toolutil.DiscussionNoteMarkdown {
	return toolutil.NewNoteMarkdown(out.ID, out.Body, out.Author, out.CreatedAt,
		toolutil.NoteMarkdownFlags{System: out.System, Internal: out.Internal, Resolvable: out.Resolvable, Resolved: out.Resolved},
		out.ResolvedBy)
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

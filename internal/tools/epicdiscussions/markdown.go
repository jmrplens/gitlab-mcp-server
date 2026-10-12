package epicdiscussions

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var markdownRenderer = toolutil.NewDiscussionRenderer("Epic Discussions", "No epic discussions found.\n", "Use `group.epic_discussion_get` to view full discussion details", "Use `group.epic_discussion_add_note` to reply to this discussion", "Use `group.epic_discussion_update_note` to edit this note")

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	return markdownRenderer.FormatGraphQLForwardList(toolutil.DiscussionMarkdowns(out.Discussions, toMarkdownDiscussion), out.Pagination)
}

// readThreadsHint is the next step to the epic's thread list, which every
// card here that has no note to act on offers.
const readThreadsHint = "read the epic's threads"

// quickActionsOnlyHints are the next steps of a body that held only quick
// actions: there is no thread or note to act on, only the epic the commands
// changed.
var quickActionsOnlyHints = []string{
	toolutil.HintAction(actionEpicGet, "see what the quick actions changed on the epic"),
	toolutil.HintAction(actionDiscussionList, readThreadsHint),
}

// resourceEventHints are the next steps of a thread GitLab synthesized from
// a resource event: its notes have no note ID, so no note action can address
// them, and what the event changed is on the epic.
var resourceEventHints = []string{
	toolutil.HintAction(actionEpicGet, "read the epic the event changed"),
	toolutil.HintAction(actionDiscussionList, readThreadsHint),
}

// FormatMarkdownString renders a discussion as Markdown. A thread-opening
// body of quick actions alone opens no thread and keeps no note, so it
// renders the commands alone rather than a thread with nothing in it. The
// test is the notes rather than the id: GitLab can answer a note it kept
// without naming the thread it opened, and that note is shown, with the
// commands beside it. A thread of resource events alone ends with the epic
// and its threads rather than a reply, as the event's note card does, since
// GitLab synthesized the thread from the event and none of its notes is one
// a note action can address (issue 1304).
func FormatMarkdownString(out Output) string {
	if len(out.Notes) == 0 && out.QuickActionsStatus != nil {
		return toolutil.FormatQuickActionsOnlyMarkdown("Epic Discussion", *out.QuickActionsStatus, quickActionsOnlyHints...)
	}
	discussion := toMarkdownDiscussion(out)
	discussion.QuickActions = out.QuickActionsStatus
	if resourceEventsOnly(out.Notes) {
		return toolutil.FormatDiscussionMarkdown(discussion, resourceEventHints...)
	}
	return markdownRenderer.FormatDiscussion(discussion)
}

// resourceEventsOnly reports whether a thread holds notes and every one of
// them is a note GitLab synthesized from a resource event.
func resourceEventsOnly(notes []NoteOutput) bool {
	if len(notes) == 0 {
		return false
	}
	for _, note := range notes {
		if !note.ResourceEvent {
			return false
		}
	}
	return true
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
	hints := markdownRenderer.NoteHints
	if out.ResourceEvent {
		hints = []string{toolutil.HintAction(actionDiscussionList, readThreadsHint)}
	}
	return toolutil.FormatNoteMarkdown(note, toolutil.NoteMarkdownOptions{
		Title:             "Discussion Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints:             hints,
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
	note := toolutil.NewNoteMarkdown(out.ID, out.Body, out.Author, out.CreatedAt,
		toolutil.NoteMarkdownFlags{System: out.System, Internal: out.Internal, Resolvable: out.Resolvable, Resolved: out.Resolved},
		out.ResolvedBy)
	if out.ResourceEvent {
		note.EventGID = out.GID
	}
	return note
}

func init() {
	toolutil.RegisterMarkdownTriple(FormatListMarkdownString, FormatMarkdownString, FormatNoteMarkdownString)
}

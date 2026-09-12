package mrdiscussions

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// toMarkdownNote builds the shared note view model for a thread note.
//
// A confidential note is internal as far as a reader is concerned — GitLab
// spells the same restriction both ways depending on the entity — so the two
// flags are folded into the one the card shows.
func toMarkdownNote(n NoteOutput) toolutil.NoteMarkdown {
	flags := toolutil.NoteMarkdownFlags{
		System:     n.System,
		Internal:   n.Internal || n.Confidential,
		Resolvable: n.Resolvable,
		Resolved:   n.Resolved,
	}
	return toolutil.NewNoteMarkdown(n.ID, n.Body, n.AuthorUsername(), n.CreatedAt, flags, n.ResolvedByUsername())
}

// toMarkdownDiscussion builds the shared thread view model, every note through
// [toMarkdownNote].
func toMarkdownDiscussion(d Output) toolutil.DiscussionMarkdown {
	notes := make([]toolutil.NoteMarkdown, 0, len(d.Notes))
	for _, n := range d.Notes {
		if n != nil {
			notes = append(notes, toMarkdownNote(*n))
		}
	}
	return toolutil.NewDiscussionMarkdown(d.ID, notes)
}

// FormatNoteMarkdown renders one discussion note as the note card every note
// tool in the tree renders: the author as a handle, the time, the flags that
// hold, the resolution state, and the body as quoted prose.
//
// It used to print "- **Resolved**: %v" on every note, resolvable or not, and
// showed neither the internal flag nor who resolved the thread.
func FormatNoteMarkdown(n NoteOutput) string {
	return toolutil.FormatNoteMarkdown(toMarkdownNote(n), toolutil.NoteMarkdownOptions{
		Title:             "Discussion Note",
		IncludeInternal:   true,
		IncludeResolvable: true,
		Hints: []string{
			toolutil.HintAction(actionDiscussionNoteUpdate, "edit this note"),
			toolutil.HintAction(actionDiscussionNoteDelete, "remove this note"),
			toolutil.HintAction(actionDiscussionResolve, "resolve or unresolve the thread it belongs to"),
		},
	})
}

// FormatOutputMarkdown renders one discussion thread through the renderer every
// discussion family shares: the thread's heading, then each note as a list item
// naming its author, time and ID, with the body quoted underneath.
func FormatOutputMarkdown(d Output) string {
	return toolutil.FormatDiscussionMarkdown(toMarkdownDiscussion(d),
		toolutil.HintAction(actionDiscussionReply, "reply to this discussion"),
		toolutil.HintAction(actionDiscussionResolve, "resolve or unresolve it"),
		toolutil.HintAction(actionDiscussionNoteUpdate, "edit one of its notes"),
	)
}

// FormatListMarkdown renders a page of merge request discussion threads through
// the same shared renderer, so a thread reads the same whether it was listed or
// fetched.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatRESTDiscussionListMarkdown(
		out.Discussions, out.Pagination, toMarkdownDiscussion,
		"MR Discussions", toolutil.EmptyMessage("merge request discussions"),
		toolutil.HintAction(actionDiscussionGet, "read one thread in full"),
		toolutil.HintAction(actionDiscussionCreate, "start a new discussion on this merge request"),
	)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatNoteMarkdown)
}

package commitdiscussions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The thread and note cards below are the ones every REST discussion domain
// renders. [Output] and [NoteOutput] are aliases of the shared thread shapes
// ([toolutil.DiscussionThreadOutput] and [toolutil.DiscussionThreadNoteOutput]),
// the Markdown registry keys on the type and keeps the first registration, and
// this package's init is the first to run, so a merge request, issue or snippet
// discussion is rendered by these two functions too. They therefore show every
// field the shared shape carries, the diff position included, and only their
// hints name the commit tools. The duplicate registrations are pinned by
// TestMarkdownRegistry_Registrations_HaveNoUndeclaredProblems and are retired by
// giving each domain a shape of its own, which is a change to the four packages
// that share these aliases rather than to a formatter.

// firstNoteAuthor returns the author username of a discussion's first note, or
// "" when the thread has no notes.
func firstNoteAuthor(d Output) string {
	if len(d.Notes) == 0 || d.Notes[0] == nil {
		return ""
	}
	return d.Notes[0].AuthorUsername()
}

// noteResolution is the word a resolvable note or thread's state reads as, the
// spelling the shared note card ([toolutil.FormatNoteMarkdown]) uses.
func noteResolution(resolved bool) string {
	if resolved {
		return "resolved"
	}
	return "unresolved"
}

// notePosition renders the diff position of a note anchored to a line of a
// file: the path as a code span and the line it hangs on. A note on a removed
// line carries the old path and line and no new ones, so both are read in turn;
// a note on the thread itself carries no position and renders nothing.
func notePosition(p *toolutil.NotePositionOutput) string {
	if p == nil {
		return ""
	}
	path, line := p.NewPath, p.NewLine
	if path == "" {
		path, line = p.OldPath, p.OldLine
	}
	span := toolutil.MdCodeSpan(path)
	if span == "" {
		return ""
	}
	if line > 0 {
		return span + ":" + strconv.FormatInt(line, 10)
	}
	return span
}

// FormatNoteMarkdownString renders a single discussion note as a card: the
// author as a handle, the time, the flags that hold, the resolution state of a
// note that can carry one, the diff position of a note on a line, and the body
// as the card's long text. A note that is not resolvable shows no resolution
// state, where the card used to print "Resolved: false" for every note in a
// domain whose notes cannot be resolved at all.
func FormatNoteMarkdownString(n NoteOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Discussion Note #%d", n.ID))
	c.Markdown("Author", toolutil.MdUserHandle(n.AuthorUsername()))
	c.Time("Created", n.CreatedAt)
	c.Flag("", "System note", n.System)
	c.Flag("", "Internal note", n.Internal)
	if n.Resolvable {
		c.Field("Resolvable", noteResolution(n.Resolved))
		c.Markdown("Resolved By", toolutil.MdUserHandle(n.ResolvedByUsername()))
	}
	c.Markdown("Position", notePosition(n.Position))
	c.Text("Body", n.Body)
	c.End(
		"Use `gitlab_update_commit_discussion_note` with note_id to edit this note",
		"Use `gitlab_add_commit_discussion_note` with discussion_id to reply to this discussion",
	)
	return b.String()
}

// FormatMarkdownString renders a discussion thread as a card: the thread's own
// fields first, then one section per note, each with the author, the time, the
// position of a diff note and the body quoted under its label.
func FormatMarkdownString(d Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Discussion "+d.ID)
	c.Count("Notes", int64(len(d.Notes)))
	c.Bool("Individual Note", d.IndividualNote)
	if d.Resolvable {
		c.Field("Resolvable", noteResolution(d.Resolved))
	}
	for _, n := range d.Notes {
		if n == nil {
			continue
		}
		s := c.Section(fmt.Sprintf("Note #%d", n.ID))
		s.Markdown("Author", toolutil.MdUserHandle(n.AuthorUsername()))
		s.Time("Created", n.CreatedAt)
		s.Flag("", "System note", n.System)
		s.Markdown("Position", notePosition(n.Position))
		s.Text("Body", n.Body)
	}
	c.End(
		"Use `gitlab_add_commit_discussion_note` to reply to this discussion",
		"Use `gitlab_update_commit_discussion_note` to edit a note",
	)
	return b.String()
}

// FormatListMarkdownString renders commit discussion threads as a Markdown
// table, the collection shape: one row per thread with the author of its first
// note and how many notes it holds. The heading counts what the response
// vouches for rather than the length of the page, and the pagination footer is
// written after the rows by the shared footer.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Discussions) == 0 {
		return toolutil.EmptyMessage("commit discussions")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Commit Discussions", len(out.Discussions), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Author", "Notes"))
	for _, d := range out.Discussions {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(d.ID),
			toolutil.EscapeMdTableCell(firstNoteAuthor(d)),
			strconv.Itoa(len(d.Notes)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use `gitlab_get_commit_discussion` with discussion_id to view full discussion details",
		"Use `gitlab_create_commit_discussion` to start a new discussion on this commit",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatNoteMarkdownString)
}

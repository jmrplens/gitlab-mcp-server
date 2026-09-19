package mrdraftnotes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The hints below name the canonical action IDs declared in action_specs.go,
// which is the one block in this package that spells them. A second block here
// is what let the two drift apart once.

// noteCellRunes is how much of a draft note's body a list row shows.
const noteCellRunes = 60

// positionText renders where in the diff a draft note is anchored: the file,
// the line when GitLab gave one, and the kind of position for anything that is
// not a plain text line.
//
// The line is only named when there is one: a note on a file, or on an image,
// carries no line number and "line 0" read as the top of the file.
func positionText(p *PositionOutput) string {
	if p == nil {
		return ""
	}
	path := p.NewPath
	if path == "" {
		path = p.OldPath
	}
	line := p.NewLine
	if line == 0 {
		line = p.OldLine
	}
	// A repository path is a committer's choice, and git allows every byte but
	// NUL and the separator inside a component.
	text := toolutil.MdCodeSpan(path)
	if line != 0 {
		text += " line " + strconv.FormatInt(line, 10)
	}
	if p.PositionType != "" && p.PositionType != "text" {
		text += " (" + toolutil.EscapeMdTableCell(p.PositionType) + ")"
	}
	return strings.TrimSpace(text)
}

// noteCell shortens a draft note's body for a list row, on rune boundaries: the
// byte slice this replaced cut a multi-byte character in half and put the
// fragment on the page.
func noteCell(note string) string {
	note = strings.ReplaceAll(toolutil.NormalizeText(note), "\n", " ")
	runes := []rune(note)
	if len(runes) > noteCellRunes {
		return string(runes[:noteCellRunes]) + "…"
	}
	return note
}

// FormatOutputMarkdown renders one draft note as the card of one object.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Draft Note #%d", out.ID))
	c.Int("ID", out.ID)
	c.Int("Author ID", out.AuthorID)
	// GitLab answers with the merge request's global database ID here, not the
	// project-scoped IID every draft-note action takes, and the row used to be
	// labeled "MR ID" as though it were the IID.
	c.Int("MR global ID (not the IID)", out.MergeRequestID)
	c.Code("Commit", out.CommitID)
	// GitLab builds a line code out of a file-path digest and two line numbers,
	// but the digest is not verified here, so the span is what shows it.
	c.Code("Line Code", out.LineCode)
	c.Code("Discussion", out.DiscussionID)
	c.Bool("Resolves the discussion", out.ResolveDiscussion)
	c.Markdown("Position", positionText(out.Position))
	c.Text("Note", out.Note)
	c.End(
		toolutil.HintAction(actionDraftNotePublish, "publish this draft note"),
		toolutil.HintAction(actionDraftNoteUpdate, "change it before publishing"),
		toolutil.HintAction(actionDraftNoteDelete, "discard it"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of draft notes as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.DraftNotes) == 0 {
		return toolutil.EmptyMessage("draft notes")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Draft Notes", len(out.DraftNotes), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Author ID", "Commit", "Note"))
	for _, d := range out.DraftNotes {
		commit := d.CommitID
		if len(commit) > 8 {
			commit = commit[:8]
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(d.ID, 10),
			strconv.FormatInt(d.AuthorID, 10),
			toolutil.MdCodeSpanCell(commit),
			toolutil.EscapeMdTableCell(noteCell(d.Note)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionDraftNoteGet, "read one draft note in full"),
		toolutil.HintAction(actionDraftNotePublishAll, "publish every draft at once"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

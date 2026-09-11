package mrdraftnotes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves: the
// draft note actions belong to the gitlab_mr_review catalog group, so their IDs
// carry the mr_review domain. The action_specs.go constants of the same shape
// spell that domain "mrdraftnotes" and are used for related-action metadata
// only; a hint naming one would name an action no surface can execute.
const (
	hintActionDraftNoteGet        = "mr_review.draft_note_get"
	hintActionDraftNoteUpdate     = "mr_review.draft_note_update"
	hintActionDraftNoteDelete     = "mr_review.draft_note_delete"
	hintActionDraftNotePublish    = "mr_review.draft_note_publish"
	hintActionDraftNotePublishAll = "mr_review.draft_note_publish_all"
)

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
		toolutil.HintAction(hintActionDraftNotePublish, "publish this draft note"),
		toolutil.HintAction(hintActionDraftNoteUpdate, "change it before publishing"),
		toolutil.HintAction(hintActionDraftNoteDelete, "discard it"),
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
		toolutil.HintAction(hintActionDraftNoteGet, "read one draft note in full"),
		toolutil.HintAction(hintActionDraftNotePublishAll, "publish every draft at once"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

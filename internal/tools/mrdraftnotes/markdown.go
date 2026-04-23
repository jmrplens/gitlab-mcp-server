// markdown.go provides Markdown formatting functions for merge request draft note MCP tool output.

package mrdraftnotes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single draft note as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Draft Note #%d\n\n", out.ID)
	fmt.Fprintf(&b, "- **Author ID**: %d\n", out.AuthorID)
	fmt.Fprintf(&b, "- **MR ID**: %d\n", out.MergeRequestID)
	if out.CommitID != "" {
		fmt.Fprintf(&b, "- **Commit**: `%s`\n", out.CommitID)
	}
	if out.DiscussionID != "" {
		fmt.Fprintf(&b, "- **Discussion**: %s\n", out.DiscussionID)
	}
	fmt.Fprintf(&b, "- **Resolve Discussion**: %v\n", out.ResolveDiscussion)
	fmt.Fprintf(&b, "\n### Body\n\n%s\n", toolutil.WrapGFMBody(out.Note))
	toolutil.WriteHints(&b,
		"Use action 'draft_note_publish' with draft_note_id to publish this note",
		"Use action 'draft_note_update' to modify before publishing",
		"Use action 'draft_note_delete' to discard this draft",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of draft notes as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Draft Notes (%d)\n\n", len(out.DraftNotes))
	toolutil.WriteListSummary(&b, len(out.DraftNotes), out.Pagination)
	if len(out.DraftNotes) == 0 {
		b.WriteString("No draft notes found.\n")
		return b.String()
	}
	b.WriteString("| ID | Author ID | Commit | Note (truncated) |\n")
	b.WriteString("| -- | --------- | ------ | ---------------- |\n")
	for _, d := range out.DraftNotes {
		note := toolutil.NormalizeText(d.Note)
		if len(note) > 60 {
			note = note[:57] + "..."
		}
		commit := d.CommitID
		if len(commit) > 8 {
			commit = commit[:8]
		}
		fmt.Fprintf(&b, "| %d | %d | %s | %s |\n", d.ID, d.AuthorID, commit, toolutil.EscapeMdTableCell(note))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'draft_note_get' with draft_note_id for full content",
		"Use action 'draft_note_publish_all' to publish all drafts at once",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

package mergetrains

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionListProject = "merge_train.list_project"
	actionListBranch  = "merge_train.list_branch"
	actionGet         = "merge_train.get"
	actionAdd         = "merge_train.add"
)

// userName returns the display username for a merge-train user sub-object, or
// "" when the user is absent.
func userName(u *toolutil.BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// mergeRequestCell renders the car's merge request as its reference linked to
// the merge request, followed by the title. A car GitLab sent no merge request
// for renders as nothing rather than as a link to "!0".
func mergeRequestCell(mr MergeRequestOutput) string {
	if mr.IID <= 0 && mr.Title == "" {
		return ""
	}
	link := toolutil.MdTitleLink(fmt.Sprintf("!%d", mr.IID), mr.WebURL)
	if mr.Title == "" {
		return link
	}
	return link + " - " + toolutil.EscapeMdTableCell(mr.Title)
}

// pipelineCell renders the car's pipeline as its number linked to the pipeline,
// with the status glyph and the status word GitLab sent. The status is what a
// reader of a merge train wants first — a car sits in the train until its
// pipeline finishes — and the card used to print the bare ID.
func pipelineCell(p *toolutil.PipelineOutput) string {
	if p == nil || p.ID <= 0 {
		return ""
	}
	cell := toolutil.MdTitleLink(fmt.Sprintf("#%d", p.ID), p.WebURL)
	if p.Status != "" {
		cell += " " + toolutil.PipelineStatusEmoji(p.Status) + " " + toolutil.EscapeMdTableCell(p.Status)
	}
	return cell
}

// durationCell renders a car's queue time in seconds, the unit GitLab counts
// it in.
func durationCell(seconds int64) string {
	return strconv.FormatInt(seconds, 10) + "s"
}

// FormatListMarkdown renders a page of merge train cars as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Trains) == 0 {
		return toolutil.EmptyMessage("merge trains")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Merge Trains", len(out.Trains), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "MR", "Title", "Target Branch", "Status", "Pipeline", "User", "Duration"))
	for _, t := range out.Trains {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.MdTitleLink(fmt.Sprintf("!%d", t.MergeRequest.IID), t.MergeRequest.WebURL),
			toolutil.EscapeMdTableCell(t.MergeRequest.Title),
			// A branch name is not an identifier: git check-ref-format permits
			// '|', '<' and '>'.
			toolutil.EscapeMdTableCell(t.TargetBranch),
			toolutil.EscapeMdTableCell(t.Status),
			pipelineCell(t.Pipeline),
			toolutil.MdUserHandle(userName(t.User)),
			durationCell(t.Duration),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, true,
		toolutil.HintAction(actionGet, "read one merge request's position on the train"),
		toolutil.HintAction(actionAdd, "add another merge request to the train"),
	)
	return sb.String()
}

// FormatOutputMarkdown renders one merge train car as the card of one object.
//
// It used to open a "| Property | Value |" table and then write a list row into
// it, which ended the table with no body and left every later "| Status | … |"
// on the page as literal pipes. A card row is a complete block wherever it
// lands, which is why this is a card and not a table.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Merge Train #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Status", out.Status)
	c.Field("Target Branch", out.TargetBranch)
	c.Markdown("Merge Request", mergeRequestCell(out.MergeRequest))
	c.Markdown("User", toolutil.MdUserHandle(userName(out.User)))
	c.Markdown("Pipeline", pipelineCell(out.Pipeline))
	c.Field("Duration", durationCell(out.Duration))
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.Time("Merged", out.MergedAt)
	c.End(
		toolutil.HintAction(actionListProject, "see every merge train in the project"),
		toolutil.HintAction(actionListBranch, "see the rest of this branch's train"),
		toolutil.HintAction(actionAdd, "add another merge request to the train"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

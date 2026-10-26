package boards

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionBoardCreate     = "project.board_create"
	actionBoardUpdate     = "project.board_update"
	actionBoardDelete     = "project.board_delete"
	actionBoardListCreate = "project.board_list_create"
	actionBoardListUpdate = "project.board_list_update"
	actionBoardListDelete = "project.board_list_delete"
)

// boardHeading composes a card heading from what names the object: its kind
// and ID always, and the title GitLab gave it when there is one, so a board
// with no name heads its card with the reference alone rather than with a
// colon and nothing after it.
func boardHeading(kind string, id int64, title string) string {
	if strings.TrimSpace(title) == "" {
		return fmt.Sprintf("%s #%d", kind, id)
	}
	return fmt.Sprintf("%s #%d: %s", kind, id, title)
}

// boardProjectLabel renders the most descriptive project reference, or "".
func boardProjectLabel(p *ProjectOutput) string {
	if p == nil {
		return ""
	}
	if p.PathWithNamespace != "" {
		return p.PathWithNamespace
	}
	return p.Name
}

// boardProjectURL is the project's own page, or "" when GitLab sent none.
func boardProjectURL(p *ProjectOutput) string {
	if p == nil {
		return ""
	}
	return p.WebURL
}

// boardListLabelName returns the label name for a list, or "" when unset.
func boardListLabelName(l BoardListOutput) string {
	if l.Label != nil {
		return l.Label.Name
	}
	return ""
}

// boardListCells is a list's row in a table of the board's columns, in the
// cells the group boards write theirs in ([toolutil.BoardColumnCells]).
func boardListCells(l BoardListOutput) []string {
	return toolutil.BoardColumnCells(l.ID, boardListScope(l), l.Position, l.MaxIssueCount, l.MaxIssueWeight, l.LimitMetric)
}

// boardListScope names what a list collects, the one thing the label column
// it replaced could not say: that column printed the list's own ID whenever
// there was no label, which named every scope "milestone" by omission. The
// cell is [toolutil.BoardColumnScope], the one the group boards write, given
// the milestone by title since this package mirrors a milestone shape of its
// own.
func boardListScope(l BoardListOutput) string {
	var milestone string
	if l.Milestone != nil {
		milestone = l.Milestone.Title
	}
	return toolutil.BoardColumnScope(l.Label, l.Assignee, milestone, l.Iteration)
}

// boardLabelNames lists a board's scope label names, skipping the nil entries
// GitLab's own arrays can carry.
func boardLabelNames(labels []*LabelDetailsOutput) []string {
	names := make([]string, 0, len(labels))
	for _, lbl := range labels {
		if lbl != nil && lbl.Name != "" {
			names = append(names, lbl.Name)
		}
	}
	return names
}

// writeBoardListsTable writes a board's columns as the nested collection they
// are, under a heading of the card's own.
func writeBoardListsTable(c *toolutil.Card, lists []BoardListOutput) {
	if len(lists) == 0 {
		return
	}
	t := c.Table("Lists", "ID", "Scope", "Position", "Max Issues", "Max Weight", toolutil.BoardListMetricLabel)
	for _, l := range lists {
		t.Row(boardListCells(l)...)
	}
}

// FormatBoardMarkdown renders one issue board as the card of one object: the
// identity, the scope GitLab filters the board by, the two column-visibility
// flags, and the board's own columns as a nested table.
//
// Every field used to be written as a bullet-less "**Label**: value" line with
// the value interpolated raw, so consecutive fields ran together into one
// paragraph and a board named across two lines added headings and list items
// of its own. The board's ID was nowhere on the card at all, which left the
// reader nothing to pass back to board_update or board_delete.
func FormatBoardMarkdown(out BoardOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, boardHeading("Board", out.ID, out.Name))
	c.Int("ID", out.ID)
	c.Link("Project", boardProjectLabel(out.Project), boardProjectURL(out.Project))
	if out.Group != nil {
		c.Link("Group", out.Group.Name, out.Group.WebURL)
	}
	if out.Milestone != nil {
		c.Link("Milestone", out.Milestone.Title, out.Milestone.WebURL)
	}
	if out.Assignee != nil {
		c.Markdown("Assignee", toolutil.MdUserLink(out.Assignee.Username, out.Assignee.WebURL))
	}
	// A board scoped to weight 0 says so; a board with no weight scope, which
	// GitLab sends as null, has no row.
	if out.Weight != nil {
		c.Int("Weight", *out.Weight)
	}
	c.Field("Labels", strings.Join(boardLabelNames(out.Labels), ", "))
	c.Bool("Hide Backlog", out.HideBacklogList)
	c.Bool("Hide Closed", out.HideClosedList)
	writeBoardListsTable(c, out.Lists)
	c.End(
		toolutil.HintAction(actionBoardListCreate, "add a column to this board"),
		toolutil.HintAction(actionBoardUpdate, "rename this board or toggle its Open and Closed lists, and on Premium and Ultimate change its scope"),
		toolutil.HintAction(actionBoardDelete, "remove this board"),
	)
	return b.String()
}

// writeColumnLimits closes a page of boards with the columns that carry a
// work-in-progress limit, through the section the group boards write in the
// same words.
func writeColumnLimits(b *strings.Builder, boards []BoardOutput) {
	var limits toolutil.BoardColumnLimits
	for _, bd := range boards {
		for _, l := range bd.Lists {
			limits.Add(bd.ID, l.ID, boardListScope(l), l.MaxIssueCount, l.MaxIssueWeight, l.LimitMetric)
		}
	}
	limits.Write(b)
}

// FormatListBoardsMarkdown renders a page of issue boards as a Markdown table:
// a collection of objects that share columns, followed by the columns of
// those boards that carry a work-in-progress limit.
func FormatListBoardsMarkdown(out ListBoardsOutput) string {
	if len(out.Boards) == 0 {
		return toolutil.EmptyMessage("issue boards")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Issue Boards", len(out.Boards), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Project", "Milestone", "Assignee", "Lists"))
	for _, bd := range out.Boards {
		var milestone, assignee string
		if bd.Milestone != nil {
			milestone = toolutil.MdTitleLink(bd.Milestone.Title, bd.Milestone.WebURL)
		}
		if bd.Assignee != nil {
			assignee = toolutil.MdUserLink(bd.Assignee.Username, bd.Assignee.WebURL)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(bd.ID, 10),
			toolutil.EscapeMdTableCell(bd.Name),
			toolutil.MdTitleLink(boardProjectLabel(bd.Project), boardProjectURL(bd.Project)),
			milestone,
			assignee,
			strconv.Itoa(len(bd.Lists)),
		))
	}
	writeColumnLimits(&b, out.Boards)
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionBoardGet, "read one board with every column, its scope and its position"),
		toolutil.HintAction(actionBoardCreate, "add a new board"),
	)
	return b.String()
}

// FormatBoardListMarkdown renders one board column as the card of one object.
func FormatBoardListMarkdown(out BoardListOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, boardHeading("Board List", out.ID, boardListLabelName(out)))
	c.Int("ID", out.ID)
	if out.Label != nil {
		c.Field("Label", out.Label.Name)
	}
	if out.Assignee != nil {
		c.Markdown("Assignee", toolutil.MdUserHandle(out.Assignee.Username))
	}
	if out.Milestone != nil {
		c.Link("Milestone", out.Milestone.Title, out.Milestone.WebURL)
	}
	if out.Iteration != nil {
		c.Link("Iteration", out.Iteration.Title, out.Iteration.WebURL)
	}
	c.Int("Position", out.Position)
	c.Count("Max Issue Count", out.MaxIssueCount)
	c.Count("Max Issue Weight", out.MaxIssueWeight)
	c.Field(toolutil.BoardListMetricLabel, out.LimitMetric)
	c.End(
		toolutil.HintAction(actionBoardListUpdate, "move this column to another position"),
		toolutil.HintAction(actionBoardListDelete, "remove this column"),
	)
	return b.String()
}

// FormatListBoardListsMarkdown renders a page of board columns as a Markdown
// table.
func FormatListBoardListsMarkdown(out ListBoardListsOutput) string {
	if len(out.Lists) == 0 {
		return toolutil.EmptyMessage("board lists")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Board Lists", len(out.Lists), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Scope", "Position", "Max Issues", "Max Weight", toolutil.BoardListMetricLabel))
	for _, l := range out.Lists {
		b.WriteString(toolutil.MarkdownTableRow(boardListCells(l)...))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionBoardListGet, "read one column"),
		toolutil.HintAction(actionBoardListCreate, "add a new column"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatBoardMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardsMarkdown)
	toolutil.RegisterMarkdown(FormatBoardListMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardListsMarkdown)
}

package groupboards

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionBoardGet        = "group.group_board_get"
	actionBoardCreate     = "group.group_board_create"
	actionBoardUpdate     = "group.group_board_update"
	actionBoardDelete     = "group.group_board_delete"
	actionBoardListLists  = "group.group_board_list_lists"
	actionBoardListGet    = "group.group_board_get_list"
	actionBoardListCreate = "group.group_board_create_list"
	actionBoardListUpdate = "group.group_board_update_list"
	actionBoardListDelete = "group.group_board_delete_list"
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

// boardListScope names what a column collects, which is the one thing a reader
// of a column wants: its label, or the assignee, milestone or iteration a
// Premium list is scoped to. A column with no scope of its own is the board's
// backlog or closed column and renders as nothing rather than as an invented
// scope.
func boardListScope(l BoardListOutput) string {
	switch {
	case boardListLabelName(l) != "":
		return toolutil.EscapeMdTableCell(boardListLabelName(l))
	case l.Assignee != nil && l.Assignee.Username != "":
		return toolutil.MdUserHandle(l.Assignee.Username)
	case l.Milestone != nil && l.Milestone.Title != "":
		return "Milestone: " + toolutil.EscapeMdTableCell(l.Milestone.Title)
	case l.Iteration != nil && l.Iteration.Title != "":
		return "Iteration: " + toolutil.EscapeMdTableCell(l.Iteration.Title)
	default:
		return ""
	}
}

// boardListLimit renders a column's issue or weight ceiling. Zero is GitLab
// saying the column has no limit, not a limit of nothing, so it renders as a
// dash rather than as the number 0.
func boardListLimit(v int64) string {
	if v == 0 {
		return "-"
	}
	return strconv.FormatInt(v, 10)
}

// boardLabelNames lists a board's scope label names, skipping the nil entries
// GitLab's own arrays can carry.
func boardLabelNames(labels []*LabelDetailsOutput) []string {
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		if l != nil && l.Name != "" {
			names = append(names, l.Name)
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
	t := c.Table("Lists", "ID", "Scope", "Position", "Max Issues", "Max Weight")
	for _, l := range lists {
		t.Row(
			strconv.FormatInt(l.ID, 10),
			boardListScope(l),
			strconv.FormatInt(l.Position, 10),
			boardListLimit(l.MaxIssueCount),
			boardListLimit(l.MaxIssueWeight),
		)
	}
}

// FormatGroupBoardMarkdown renders one group issue board as the card of one
// object.
//
// Every field used to be written as a bullet-less "**Label**: value" line with
// the value interpolated raw, so consecutive fields ran together into one
// paragraph and a board named across two lines added headings and list items
// of its own. The assignee, the weight and the two column-visibility flags
// GitLab sends on every group board were not shown at all.
func FormatGroupBoardMarkdown(out GroupBoardOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, boardHeading("Group Board", out.ID, out.Name))
	c.Int("ID", out.ID)
	if out.Group != nil {
		c.Link("Group", out.Group.Name, out.Group.WebURL)
	}
	if out.Milestone != nil {
		c.Link("Milestone", out.Milestone.Title, out.Milestone.WebURL)
	}
	if out.Assignee != nil {
		c.Markdown("Assignee", toolutil.MdUserLink(out.Assignee.Username, out.Assignee.WebURL))
	}
	c.Count("Weight", out.Weight)
	if names := boardLabelNames(out.Labels); len(names) > 0 {
		c.Field("Labels", strings.Join(names, ", "))
	}
	c.Bool("Hide Backlog", out.HideBacklogList)
	c.Bool("Hide Closed", out.HideClosedList)
	writeBoardListsTable(c, out.Lists)
	c.End(
		toolutil.HintAction(actionBoardListCreate, "add a column to this board"),
		toolutil.HintAction(actionBoardUpdate, "change this board's name or scope"),
		toolutil.HintAction(actionBoardDelete, "remove this board"),
	)
	return b.String()
}

// FormatListGroupBoardsMarkdown renders a page of group issue boards as a
// Markdown table: a collection of objects that share columns.
func FormatListGroupBoardsMarkdown(out ListGroupBoardsOutput) string {
	if len(out.Boards) == 0 {
		return toolutil.EmptyMessage("group issue boards")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Issue Boards", len(out.Boards), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Group", "Milestone", "Lists"))
	for _, bd := range out.Boards {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(bd.ID, 10),
			toolutil.EscapeMdTableCell(bd.Name),
			groupCell(bd.Group),
			milestoneCell(bd.Milestone),
			strconv.Itoa(len(bd.Lists)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionBoardGet, "read one board with its columns"),
		toolutil.HintAction(actionBoardCreate, "add a new board to the group"),
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
	if out.Iteration != nil {
		c.Link("Iteration", out.Iteration.Title, out.Iteration.WebURL)
	}
	if out.Milestone != nil {
		c.Link("Milestone", out.Milestone.Title, out.Milestone.WebURL)
	}
	c.Int("Position", out.Position)
	c.Count("Max Issue Count", out.MaxIssueCount)
	c.Count("Max Issue Weight", out.MaxIssueWeight)
	c.Field("Limit Metric", out.LimitMetric)
	c.End(
		toolutil.HintAction(actionBoardListUpdate, "change this column's position or limits"),
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
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Scope", "Position", "Max Issues", "Max Weight"))
	for _, l := range out.Lists {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(l.ID, 10),
			boardListScope(l),
			strconv.FormatInt(l.Position, 10),
			boardListLimit(l.MaxIssueCount),
			boardListLimit(l.MaxIssueWeight),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionBoardListGet, "read one column"),
		toolutil.HintAction(actionBoardListLists, "page through the rest of the board's columns"),
	)
	return b.String()
}

// boardListLabelName returns the list's label name, or "" when the list has no
// label scope (assignee/milestone/iteration lists).
func boardListLabelName(l BoardListOutput) string {
	if l.Label != nil {
		return l.Label.Name
	}
	return ""
}

// groupCell links a board's group to its page, or renders nothing when GitLab
// sent no group.
func groupCell(g *GroupRefOutput) string {
	if g == nil {
		return ""
	}
	return toolutil.MdTitleLink(g.Name, g.WebURL)
}

// milestoneCell links a board's milestone to its page, or renders nothing when
// the board is not scoped to one.
func milestoneCell(m *MilestoneOutput) string {
	if m == nil {
		return ""
	}
	return toolutil.MdTitleLink(m.Title, m.WebURL)
}

func init() {
	toolutil.RegisterMarkdown(FormatGroupBoardMarkdown)
	toolutil.RegisterMarkdown(FormatListGroupBoardsMarkdown)
	toolutil.RegisterMarkdown(FormatBoardListMarkdown)
	toolutil.RegisterMarkdown(FormatListBoardListsMarkdown)
}

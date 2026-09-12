package groupepicboards

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionEpicBoardList = "group.epic_board_list"
	actionEpicBoardGet  = "group.epic_board_get"
)

// labelNames extracts the label names from the board scope labels, skipping
// the nil entries GitLab's own arrays can carry.
func labelNames(labels []*LabelDetailsOutput) []string {
	if len(labels) == 0 {
		return nil
	}
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		if l == nil || l.Name == "" {
			continue
		}
		names = append(names, l.Name)
	}
	return names
}

// listScope names what a column collects: its label when it has one, and
// otherwise the list type GitLab gave it, which is what distinguishes the
// board's backlog and closed columns from each other. The column used to show
// nothing at all for either of them.
func listScope(l BoardListOutput) string {
	if l.Label != nil && l.Label.Name != "" {
		return toolutil.EscapeMdTableCell(l.Label.Name)
	}
	return toolutil.EscapeMdTableCell(l.ListType)
}

// collapsedCell renders a column's collapsed flag, and nothing when GitLab did
// not send it.
func collapsedCell(collapsed *bool) string {
	if collapsed == nil {
		return ""
	}
	return toolutil.BoolEmoji(*collapsed)
}

// FormatOutputMarkdown renders one group epic board as the card of one object:
// the identity, the group it belongs to, its scope labels, the two
// column-visibility flags, and its columns as a nested table.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Epic Board #%d: %s", out.ID, out.Name))
	c.Int("ID", out.ID)
	if out.Group != nil {
		c.Link("Group", out.Group.Name, out.Group.WebURL)
	}
	if names := labelNames(out.Labels); len(names) > 0 {
		c.Field("Labels", strings.Join(names, ", "))
	}
	c.Bool("Hide Backlog", out.HideBacklogList)
	c.Bool("Hide Closed", out.HideClosedList)
	if len(out.Lists) > 0 {
		t := c.Table("Board Lists", "ID", "Scope", "Type", "Position", "Collapsed")
		for _, l := range out.Lists {
			t.Row(
				strconv.FormatInt(l.ID, 10),
				listScope(l),
				toolutil.EscapeMdTableCell(l.ListType),
				strconv.FormatInt(l.Position, 10),
				collapsedCell(l.Collapsed),
			)
		}
	}
	c.End(toolutil.HintAction(actionEpicBoardList, "see every epic board in the group"))
	return b.String()
}

// FormatListMarkdown renders a page of group epic boards as a Markdown table:
// a collection of objects that share columns.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Boards) == 0 {
		return toolutil.EmptyMessage("epic boards")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Epic Boards", len(out.Boards), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Group", "Labels", "Lists"))
	for _, board := range out.Boards {
		var group string
		if board.Group != nil {
			group = toolutil.MdTitleLink(board.Group.Name, board.Group.WebURL)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(board.ID, 10),
			toolutil.EscapeMdTableCell(board.Name),
			group,
			toolutil.EscapeMdTableCell(strings.Join(labelNames(board.Labels), ", ")),
			strconv.Itoa(len(board.Lists)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionEpicBoardGet, "read one board with its columns"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

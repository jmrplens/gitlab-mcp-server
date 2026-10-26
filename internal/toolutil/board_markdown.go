package toolutil

import (
	"strconv"
	"strings"
)

// BoardListMetricLabel is what a board column's limit_metric is shown under:
// the row of the column's own card and the column of every table of columns,
// project and group boards alike, so the two packages name it in the same
// words (issue 1256). The cell beside it is [CellOr] with a dash, since a
// column with no limit, and every column on a Free instance, has no metric.
const BoardListMetricLabel = "Limit Metric"

// BoardColumnScope names what a board column collects, which is the one thing
// a reader of a column wants: its label, or the assignee, milestone or
// iteration a Premium column is scoped to, rendered as a table cell. The
// project and the group boards both name a column's scope through it, each
// from its own list output, so a scope reads the same on either. The milestone
// is passed by its title, the one field of it the cell shows, because each of
// the two packages mirrors a milestone shape of its own; it is "" when the
// column is scoped to none. A scope object GitLab sent without the name or
// title it is shown by is passed over as if it were absent.
//
// A column GitLab sends with none of the four is a status column: GitLab
// answers a board's lists without its backlog and closed columns, a column of
// any of those four scopes is deleted with its scope, and the REST List entity
// exposes no status. It renders as nothing rather than as an invented scope.
//
// It is a chain of ifs rather than a tagless switch so that each condition is
// a statement coverage can see: Go's coverage counts a block for each case
// body and none for the case expressions, which left every mutant of those
// expressions reported as not covered whatever the tests drove.
func BoardColumnScope(label *BoardLabelOutput, assignee *BoardListAssigneeOutput, milestone string, iteration *IterationOutput) string {
	if label != nil && label.Name != "" {
		return EscapeMdTableCell(label.Name)
	}
	if assignee != nil && assignee.Username != "" {
		return MdUserHandle(assignee.Username)
	}
	if milestone != "" {
		return "Milestone: " + EscapeMdTableCell(milestone)
	}
	if iteration != nil && iteration.Title != "" {
		return "Iteration: " + EscapeMdTableCell(iteration.Title)
	}
	return ""
}

// BoardColumnCells is one column's row in a table of a board's columns, under
// the ID, Scope, Position, Max Issues, Max Weight and [BoardListMetricLabel]
// headers the card of a board and the page of its columns both write: the
// column's own ID, its scope as [BoardColumnScope] renders it, its position,
// its two ceilings, and the metric they are counted by, a dash when GitLab
// sent none. The project and the group boards both write their rows through
// it, so a column reads the same on either.
func BoardColumnCells(id int64, scope string, position, maxIssues, maxWeight int64, metric string) []string {
	return []string{
		strconv.FormatInt(id, 10),
		scope,
		strconv.FormatInt(position, 10),
		boardColumnCeiling(maxIssues),
		boardColumnCeiling(maxWeight),
		CellOr(metric, "-"),
	}
}

// The heading of the section [BoardColumnLimits] writes, and the one sentence
// it is instead when no column on the page carries a limit.
const (
	boardColumnLimitsHeading = "### Column Limits\n\n"
	noBoardColumnLimits      = "No column on these boards has a work-in-progress limit.\n"
)

// BoardColumnLimits gathers, across a page of issue boards, the columns that
// carry a work-in-progress limit, and writes them as the section the page's
// table of boards is followed by. A board listing used to say only how many
// columns each board has, so a limit and the metric it is counted by showed
// only once a board or its columns were read, never on the listing (issue
// 1284). The project and the group board listings both write it, each adding
// the columns of its own output type, so the two name the section and its
// columns in the same words. The zero value is ready to use.
type BoardColumnLimits struct {
	rows []string
}

// Add records one column of board, list being the column's own ID, which is
// what the column actions take. It keeps the column only when it carries a
// ceiling, on the number of its issues or on their total weight: a metric
// with both ceilings at zero limits nothing, and the board read shows every
// column whether limited or not. scope is the column's scope as
// [BoardColumnScope] renders it. A ceiling the column does not set and a
// metric GitLab sent none of are written as dashes, as the board read writes
// them.
func (l *BoardColumnLimits) Add(board, list int64, scope string, maxIssues, maxWeight int64, metric string) {
	if maxIssues == 0 && maxWeight == 0 {
		return
	}
	l.rows = append(l.rows, MarkdownTableRow(
		strconv.FormatInt(board, 10),
		strconv.FormatInt(list, 10),
		scope,
		boardColumnCeiling(maxIssues),
		boardColumnCeiling(maxWeight),
		CellOr(metric, "-"),
	))
}

// Write writes the section after what b holds, separated from it by a blank
// line: the heading and a table of the columns [BoardColumnLimits.Add] kept,
// in the order they were added, or, when it kept none, one sentence saying so
// rather than a table header standing over nothing.
func (l *BoardColumnLimits) Write(b *strings.Builder) {
	endBlock(b)
	if len(l.rows) == 0 {
		b.WriteString(noBoardColumnLimits)
		return
	}
	b.WriteString(boardColumnLimitsHeading)
	b.WriteString(MarkdownTableHeader("Board", "List", "Scope", "Max Issues", "Max Weight", BoardListMetricLabel))
	for _, row := range l.rows {
		b.WriteString(row)
	}
}

// boardColumnCeiling renders a board column's issue or weight ceiling as a
// table cell. Zero is GitLab saying the column sets no such ceiling, not a
// ceiling of nothing, so it renders as a dash rather than as the number 0.
// [BoardColumnCells] and [BoardColumnLimits] both write it, so a ceiling
// reads the same in the board card, the list of columns and the section of
// limited columns.
func boardColumnCeiling(v int64) string {
	if v == 0 {
		return "-"
	}
	return strconv.FormatInt(v, 10)
}

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
// column whether limited or not. scope is the column's scope already rendered
// as a table cell, by the formatter that knows the column's output type. A
// ceiling the column does not set and a metric GitLab sent none of are
// written as dashes, as the board read writes them.
func (l *BoardColumnLimits) Add(board, list int64, scope string, maxIssues, maxWeight int64, metric string) {
	if maxIssues == 0 && maxWeight == 0 {
		return
	}
	l.rows = append(l.rows, MarkdownTableRow(
		strconv.FormatInt(board, 10),
		strconv.FormatInt(list, 10),
		scope,
		BoardColumnCeiling(maxIssues),
		BoardColumnCeiling(maxWeight),
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

// BoardColumnCeiling renders a board column's issue or weight ceiling as a
// table cell. Zero is GitLab saying the column sets no such ceiling, not a
// ceiling of nothing, so it renders as a dash rather than as the number 0.
// Every table of columns writes it, project and group boards alike, and
// [BoardColumnLimits] as well, so a ceiling reads the same wherever it is
// shown.
func BoardColumnCeiling(v int64) string {
	if v == 0 {
		return "-"
	}
	return strconv.FormatInt(v, 10)
}

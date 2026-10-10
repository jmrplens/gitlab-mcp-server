package toolutil

// BoardListMetricLabel is what a board column's limit_metric is shown under:
// the row of the column's own card and the column of every table of columns,
// project and group boards alike, so the two packages name it in the same
// words (issue 1256). The cell beside it is [CellOr] with a dash, since a
// column with no limit, and every column on a Free instance, has no metric.
const BoardListMetricLabel = "Limit Metric"

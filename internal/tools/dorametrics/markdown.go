package dorametrics

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// actionDeploymentList is the canonical catalog ID of the action whose result
// a reader correlates these metrics with.
const actionDeploymentList = "environment.deployment_list"

// FormatMarkdown renders a DORA metric series as a Markdown table: a
// collection of data points that share columns.
//
// The metric name comes from the caller because the output carries neither the
// metric nor the date window that was asked for, so the registered formatter
// can only render the series itself. Naming the metric and its unit in the
// value column needs both in Output.
func FormatMarkdown(out Output, metric string) string {
	if len(out.Metrics) == 0 {
		return toolutil.EmptyMessage("DORA metric data points")
	}
	title := "DORA Metrics"
	if metric != "" {
		title = "DORA Metrics: " + metric
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, title, len(out.Metrics), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Date", "Value"))
	for _, m := range out.Metrics {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.FormatTime(m.Date),
			metricValue(m.Value),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionDeploymentList, "correlate these metrics with deployment activity"),
	)
	return b.String()
}

// metricValue renders one data point. The four DORA metrics are counted in
// different units — deployments, seconds, a ratio — so a fixed four decimals
// stamped false precision on a count and on a duration alike.
func metricValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func init() {
	toolutil.RegisterMarkdown(func(v Output) string { return FormatMarkdown(v, "") })
}

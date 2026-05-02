package dorametrics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders DORA metrics as a Markdown table.
func FormatMarkdown(out Output, metric string) string {
	var sb strings.Builder
	title := "DORA Metrics"
	if metric != "" {
		title = fmt.Sprintf("DORA Metrics — %s", toolutil.EscapeMdTableCell(metric))
	}
	fmt.Fprintf(&sb, "## %s\n\n", title)
	if len(out.Metrics) == 0 {
		sb.WriteString("No metrics data available.\n")
		return sb.String()
	}
	sb.WriteString("| Date | Value |\n|------|-------|\n")
	for _, m := range out.Metrics {
		fmt.Fprintf(&sb, "| %s | %.4f |\n", m.Date, m.Value)
	}
	fmt.Fprintf(&sb, "\n**Total data points:** %d\n", len(out.Metrics))
	toolutil.WriteHints(&sb,
		"Use `gitlab_deployment_list` to correlate with deployment activity",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(func(v Output) string { return FormatMarkdown(v, "") })
}

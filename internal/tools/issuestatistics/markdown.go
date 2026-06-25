package issuestatistics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// init registers the [StatisticsOutput] Markdown formatter with the
// package's type-keyed formatter registry.
func init() {
	toolutil.RegisterMarkdown(func(out StatisticsOutput) string { return FormatMarkdown("All", out) })
}

// FormatMarkdown renders a [StatisticsOutput] value as a Markdown table
// with an "All / Opened / Closed" breakdown. The label is used as the
// heading prefix (for example, "Project" or "Group").
func FormatMarkdown(label string, out StatisticsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Issue Statistics\n\n| Status | Count |\n|--------|-------|\n| All | %d |\n| Opened | %d |\n| Closed | %d |\n",
		label, out.Statistics.Counts.All, out.Statistics.Counts.Opened, out.Statistics.Counts.Closed)
	toolutil.WriteHints(&b, "Use gitlab_issue action 'list' to see individual issues")
	return b.String()
}

package issuestatistics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(func(out StatisticsOutput) string { return FormatMarkdown("All", out) })
}

// FormatMarkdown formats issue statistics as markdown.
func FormatMarkdown(label string, out StatisticsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s Issue Statistics\n\n| Status | Count |\n|--------|-------|\n| All | %d |\n| Opened | %d |\n| Closed | %d |\n",
		label, out.All, out.Opened, out.Closed)
	toolutil.WriteHints(&b, "Use gitlab_issue action 'list' to see individual issues")
	return b.String()
}

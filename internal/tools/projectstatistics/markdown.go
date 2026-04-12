// markdown.go provides Markdown formatting functions for project statistics MCP tool output.

package projectstatistics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown formats project statistics as markdown.
func FormatMarkdown(out GetOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Statistics (Last 30 Days)\n\n**Total Fetches**: %d\n\n", out.TotalFetches)
	if len(out.Days) > 0 {
		sb.WriteString("| Date | Count |\n|------|-------|\n")
		for _, d := range out.Days {
			fmt.Fprintf(&sb, "| %s | %d |\n", toolutil.FormatTime(d.Date), d.Count)
		}
	}
	toolutil.WriteHints(&sb, "Use fetcher counts to track project activity trends")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}

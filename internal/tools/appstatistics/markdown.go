// markdown.go provides Markdown formatting functions for application statistics MCP tool output.
package appstatistics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatGetMarkdown formats application statistics as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	sb.WriteString("## Application Statistics\n\n")
	sb.WriteString("| Metric | Count |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Active Users | %d |\n", out.ActiveUsers)
	fmt.Fprintf(&sb, "| Users | %d |\n", out.Users)
	fmt.Fprintf(&sb, "| Projects | %d |\n", out.Projects)
	fmt.Fprintf(&sb, "| Groups | %d |\n", out.Groups)
	fmt.Fprintf(&sb, "| Issues | %d |\n", out.Issues)
	fmt.Fprintf(&sb, "| Merge Requests | %d |\n", out.MergeRequests)
	fmt.Fprintf(&sb, "| Notes | %d |\n", out.Notes)
	fmt.Fprintf(&sb, "| Forks | %d |\n", out.Forks)
	fmt.Fprintf(&sb, "| Snippets | %d |\n", out.Snippets)
	fmt.Fprintf(&sb, "| SSH Keys | %d |\n", out.SSHKeys)
	fmt.Fprintf(&sb, "| Milestones | %d |\n", out.Milestones)
	toolutil.WriteHints(&sb, "Use individual resource tools to explore specific statistics")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

package grouplabels

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMarkdown renders a single group label as a Markdown summary.
func FormatMarkdown(l Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Label: %s\n\n", toolutil.EscapeMdHeading(l.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, l.ID)
	fmt.Fprintf(&b, "- **Color**: %s\n", l.Color)
	if l.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, l.Description)
	}
	if l.Priority > 0 {
		fmt.Fprintf(&b, "- **Priority**: %d\n", l.Priority)
	}
	fmt.Fprintf(&b, "- **Project label**: %v\n", l.IsProjectLabel)
	fmt.Fprintf(&b, "- **Subscribed**: %v\n", l.Subscribed)
	if l.OpenIssuesCount > 0 || l.ClosedIssuesCount > 0 || l.OpenMergeRequestsCount > 0 {
		fmt.Fprintf(&b, "- **Issues**: %d open, %d closed\n", l.OpenIssuesCount, l.ClosedIssuesCount)
		fmt.Fprintf(&b, "- **Open MRs**: %d\n", l.OpenMergeRequestsCount)
	}
	toolutil.WriteHints(&b,
		"Use action 'group_label_update' to modify this label",
		"Use action 'group_label_delete' to remove this label",
		"Use action 'group_label_subscribe'/'group_label_unsubscribe' to follow/unfollow",
	)
	return b.String()
}

// FormatListMarkdownString renders a paginated list of group labels as a Markdown table string.
func FormatListMarkdownString(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Labels (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Labels), out.Pagination)
	if len(out.Labels) == 0 {
		b.WriteString("No group labels found.\n")
		return b.String()
	}
	b.WriteString("| Name | Color | Open Issues | Closed Issues | Open MRs |\n")
	b.WriteString("|------|-------|-------------|---------------|----------|\n")
	for _, l := range out.Labels {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n",
			toolutil.EscapeMdTableCell(l.Name), toolutil.EscapeMdTableCell(l.Color), l.OpenIssuesCount, l.ClosedIssuesCount, l.OpenMergeRequestsCount)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'group_label_get' with label name for full details",
		"Use action 'group_label_create' to add a new group label",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of group labels as an MCP Markdown result.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

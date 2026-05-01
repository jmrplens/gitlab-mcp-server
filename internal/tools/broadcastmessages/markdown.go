package broadcastmessages

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats broadcast messages list as markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString("# Broadcast Messages\n\n")
	toolutil.WriteListSummary(&sb, len(out.Messages), out.Pagination)
	if len(out.Messages) == 0 {
		sb.WriteString("No broadcast messages found.\n")
		return toolutil.ToolResultWithMarkdown(sb.String())
	}
	sb.WriteString("| ID | Message | Type | Active | Starts | Ends |\n")
	sb.WriteString("|---|---|---|---|---|---|\n")
	for _, m := range out.Messages {
		fmt.Fprintf(&sb, "| %d | %s | %s | %v | %s | %s |\n",
			m.ID, toolutil.EscapeMdTableCell(m.Message), m.BroadcastType, m.Active, m.StartsAt, m.EndsAt)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_broadcast_message` to view details of a specific message")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatMessageMarkdown formats a single broadcast message as markdown.
func FormatMessageMarkdown(item MessageItem) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Broadcast Message #%d\n\n", item.ID)
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Message | %s |\n", item.Message)
	fmt.Fprintf(&sb, "| Type | %s |\n", item.BroadcastType)
	fmt.Fprintf(&sb, "| Active | %v |\n", item.Active)
	fmt.Fprintf(&sb, "| Dismissable | %v |\n", item.Dismissable)
	if item.StartsAt != "" {
		fmt.Fprintf(&sb, "| Starts At | %s |\n", toolutil.FormatTime(item.StartsAt))
	}
	if item.EndsAt != "" {
		fmt.Fprintf(&sb, "| Ends At | %s |\n", toolutil.FormatTime(item.EndsAt))
	}
	if item.Theme != "" {
		fmt.Fprintf(&sb, "| Theme | %s |\n", item.Theme)
	}
	if item.TargetPath != "" {
		fmt.Fprintf(&sb, "| Target Path | %s |\n", item.TargetPath)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_broadcast_message` to modify this message")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatMessageMarkdown)
}

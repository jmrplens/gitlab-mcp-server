package broadcastmessages

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Message", "Type", "Active", "Starts", "Ends"))
	for _, m := range out.Messages {
		//gitlab:allow-unescaped m.BroadcastType: a broadcast type GitLab picks from a fixed set (banner, notification).
		//gitlab:allow-unescaped m.StartsAt: a timestamp toMessageItem formatted itself, with time.Time.Format as RFC 3339.
		//gitlab:allow-unescaped m.EndsAt: a timestamp toMessageItem formatted itself, with time.Time.Format as RFC 3339.
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
	sb.WriteString(toolutil.MarkdownTableHeader("Property", "Value"))
	fmt.Fprintf(&sb, "| Message | %s |\n", toolutil.EscapeMdTableCell(item.Message))
	//gitlab:allow-unescaped item.BroadcastType: a broadcast type GitLab picks from a fixed set (banner, notification).
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
		//gitlab:allow-unescaped item.Theme: a broadcast theme GitLab picks from a fixed set of color names (indigo, light-indigo, blue and the rest).
		fmt.Fprintf(&sb, "| Theme | %s |\n", item.Theme)
	}
	if item.TargetPath != "" {
		// A target path is a path glob an administrator types into the message.
		fmt.Fprintf(&sb, "| Target Path | %s |\n", toolutil.EscapeMdTableCell(item.TargetPath))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_broadcast_message` to modify this message")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func formatGetOutput(out GetOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func formatCreateOutput(out CreateOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func formatUpdateOutput(out UpdateOutput) *mcp.CallToolResult {
	return FormatMessageMarkdown(out.Message)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatMessageMarkdown)
	toolutil.RegisterMarkdownResult(formatGetOutput)
	toolutil.RegisterMarkdownResult(formatCreateOutput)
	toolutil.RegisterMarkdownResult(formatUpdateOutput)
}

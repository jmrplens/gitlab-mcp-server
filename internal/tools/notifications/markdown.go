// markdown.go provides Markdown formatting functions for notification settings MCP tool output.

package notifications

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMarkdown formats notification settings as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders notification settings as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Notification Settings\n\n")
	fmt.Fprintf(&b, "- **Level**: %s\n", out.Level)
	if out.NotificationEmail != "" {
		fmt.Fprintf(&b, toolutil.FmtMdEmail, out.NotificationEmail)
	}
	if out.Events != nil {
		b.WriteString("\n### Custom Events\n\n")
		b.WriteString(eventLine("Close Issue", out.Events.CloseIssue))
		b.WriteString(eventLine("Close MR", out.Events.CloseMergeRequest))
		b.WriteString(eventLine("Failed Pipeline", out.Events.FailedPipeline))
		b.WriteString(eventLine("Fixed Pipeline", out.Events.FixedPipeline))
		b.WriteString(eventLine("Issue Due", out.Events.IssueDue))
		b.WriteString(eventLine("Merge MR", out.Events.MergeMergeRequest))
		b.WriteString(eventLine("Merge When Pipeline Succeeds", out.Events.MergeWhenPipelineSucceeds))
		b.WriteString(eventLine("Moved Project", out.Events.MovedProject))
		b.WriteString(eventLine("New Issue", out.Events.NewIssue))
		b.WriteString(eventLine("New MR", out.Events.NewMergeRequest))
		b.WriteString(eventLine("New Epic", out.Events.NewEpic))
		b.WriteString(eventLine("New Note", out.Events.NewNote))
		b.WriteString(eventLine("Push to MR", out.Events.PushToMergeRequest))
		b.WriteString(eventLine("Reassign Issue", out.Events.ReassignIssue))
		b.WriteString(eventLine("Reassign MR", out.Events.ReassignMergeRequest))
		b.WriteString(eventLine("Reopen Issue", out.Events.ReopenIssue))
		b.WriteString(eventLine("Reopen MR", out.Events.ReopenMergeRequest))
		b.WriteString(eventLine("Success Pipeline", out.Events.SuccessPipeline))
	}
	toolutil.WriteHints(&b, "Use `gitlab_update_notification_settings` to change notification preferences")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

package topics

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of topics.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Topics) == 0 {
		return toolutil.ToolResultWithMarkdown("No topics found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Topics (%d)\n\n", len(out.Topics))
	toolutil.WriteListSummary(&sb, len(out.Topics), out.Pagination)
	sb.WriteString("| ID | Name | Title | Projects |\n")
	sb.WriteString("|----|------|-------|----------|\n")
	for _, t := range out.Topics {
		fmt.Fprintf(&sb, "| %d | %s | %s | %d |\n",
			t.ID,
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(t.Title),
			t.TotalProjectsCount)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_topic` to view details of a specific topic")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatTopicMarkdown formats a single topic.
func FormatTopicMarkdown(t TopicItem) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Topic: %s (ID: %d)\n\n", t.Name, t.ID)
	if t.Title != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdTitle, t.Title)
	}
	if t.Description != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdDescription, t.Description)
	}
	fmt.Fprintf(&sb, "- **Projects**: %d\n", t.TotalProjectsCount)
	if t.AvatarURL != "" {
		fmt.Fprintf(&sb, "- **Avatar**: %s\n", t.AvatarURL)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_topic` to modify this topic")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatTopicMarkdown)
}

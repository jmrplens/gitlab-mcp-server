// markdown.go provides Markdown formatting functions for freeze period MCP tool output.
package freezeperiods

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of freeze periods as Markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders freeze periods list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.FreezePeriods) == 0 {
		return "No freeze periods found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Freeze Periods (%d)\n\n", len(out.FreezePeriods))
	toolutil.WriteListSummary(&b, len(out.FreezePeriods), out.Pagination)
	for _, fp := range out.FreezePeriods {
		fmt.Fprintf(&b, "- **ID %d**: start=`%s` end=`%s` tz=%s\n", fp.ID, fp.FreezeStart, fp.FreezeEnd, fp.CronTimezone)
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b, "Use `gitlab_get_freeze_period` to view details of a specific freeze period")
	return b.String()
}

// FormatMarkdown formats a single freeze period as Markdown.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a single freeze period as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	b.WriteString("## Freeze Period\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **Start**: `%s`\n", out.FreezeStart)
	fmt.Fprintf(&b, "- **End**: `%s`\n", out.FreezeEnd)
	if out.CronTimezone != "" {
		fmt.Fprintf(&b, "- **Timezone**: %s\n", out.CronTimezone)
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b, "Use `gitlab_update_freeze_period` to modify this freeze period")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

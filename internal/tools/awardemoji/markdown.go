// markdown.go provides Markdown formatting functions for award emoji MCP tool output.
package awardemoji

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats award emoji list as a Markdown CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders award emoji list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.AwardEmoji) == 0 {
		return "No award emoji found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Award Emoji (%d)\n\n", len(out.AwardEmoji))
	toolutil.WriteListSummary(&b, len(out.AwardEmoji), out.Pagination)
	for _, e := range out.AwardEmoji {
		fmt.Fprintf(&b, "- :%s: by %s (ID: %d) — %s\n", e.Name, e.Username, e.ID, toolutil.FormatTime(e.CreatedAt))
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b, "Use action 'create' to add an emoji reaction, 'delete' to remove one")
	return b.String()
}

// FormatMarkdown formats a single award emoji as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a single award emoji as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Award Emoji\n\n")
	fmt.Fprintf(&b, "- **Name**: :%s:\n", out.Name)
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, "- **User**: %s (ID: %d)\n", out.Username, out.UserID)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b, "Use action 'delete' with award_id to remove this emoji")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

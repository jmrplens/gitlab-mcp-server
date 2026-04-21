// markdown.go provides Markdown formatting functions for badge MCP tool output.

package badges

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatBadgeListMarkdown formats a list of badges.
func FormatBadgeListMarkdown(badges []BadgeItem, title string, pagination toolutil.PaginationOutput) *mcp.CallToolResult {
	if len(badges) == 0 {
		return toolutil.ToolResultWithMarkdown("No badges found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s (%d)\n\n", title, len(badges))
	sb.WriteString("| ID | Name | Link URL | Image URL | Kind |\n")
	sb.WriteString("|----|------|----------|-----------|------|\n")
	for _, b := range badges {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n",
			b.ID,
			toolutil.EscapeMdTableCell(b.Name),
			toolutil.EscapeMdTableCell(b.LinkURL),
			toolutil.EscapeMdTableCell(b.ImageURL),
			b.Kind)
	}
	toolutil.WritePagination(&sb, pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_badge` to view details of a specific badge")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatBadgeMarkdown formats a single badge.
func FormatBadgeMarkdown(b BadgeItem) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Badge: %s (ID: %d)\n\n", b.Name, b.ID)
	fmt.Fprintf(&sb, "- **Link URL**: %s\n", b.LinkURL)
	fmt.Fprintf(&sb, "- **Image URL**: %s\n", b.ImageURL)
	if b.RenderedLinkURL != "" {
		fmt.Fprintf(&sb, "- **Rendered Link**: %s\n", b.RenderedLinkURL)
	}
	if b.RenderedImageURL != "" {
		fmt.Fprintf(&sb, "- **Rendered Image**: %s\n", b.RenderedImageURL)
	}
	if b.Kind != "" {
		fmt.Fprintf(&sb, "- **Kind**: %s\n", b.Kind)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_badge` to modify this badge")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatBadgeMarkdown)
}

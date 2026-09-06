package appearance

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatGetMarkdown formats appearance into markdown.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	a := out.Appearance
	var sb strings.Builder
	sb.WriteString("# Application Appearance\n\n")
	sb.WriteString(toolutil.MarkdownTableHeader("Property", "Value"))
	// Every field here is free text an administrator typed into the appearance
	// settings, and the two messages are rendered as Markdown by GitLab itself.
	fmt.Fprintf(&sb, "| Title | %s |\n", toolutil.EscapeMdTableCell(a.Title))
	fmt.Fprintf(&sb, "| Description | %s |\n", toolutil.EscapeMdTableCell(a.Description))
	if a.PWAName != "" {
		fmt.Fprintf(&sb, "| PWA Name | %s |\n", toolutil.EscapeMdTableCell(a.PWAName))
	}
	if a.PWAShortName != "" {
		fmt.Fprintf(&sb, "| PWA Short Name | %s |\n", toolutil.EscapeMdTableCell(a.PWAShortName))
	}
	if a.HeaderMessage != "" {
		fmt.Fprintf(&sb, "| Header Message | %s |\n", toolutil.EscapeMdTableCell(a.HeaderMessage))
	}
	if a.FooterMessage != "" {
		fmt.Fprintf(&sb, "| Footer Message | %s |\n", toolutil.EscapeMdTableCell(a.FooterMessage))
	}
	fmt.Fprintf(&sb, "| Email Header/Footer | %v |\n", a.EmailHeaderAndFooterEnabled)
	toolutil.WriteHints(&sb, "Use `gitlab_update_appearance` to modify appearance settings")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatUpdateMarkdown formats the updated appearance response.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	return FormatGetMarkdown(GetOutput(out))
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}

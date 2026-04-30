// markdown.go provides Markdown formatting functions for GitLab instance appearance MCP tool output.
package appearance

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatGetMarkdown formats appearance into markdown.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	a := out.Appearance
	var sb strings.Builder
	sb.WriteString("# Application Appearance\n\n")
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Title | %s |\n", a.Title)
	fmt.Fprintf(&sb, "| Description | %s |\n", a.Description)
	if a.PWAName != "" {
		fmt.Fprintf(&sb, "| PWA Name | %s |\n", a.PWAName)
	}
	if a.PWAShortName != "" {
		fmt.Fprintf(&sb, "| PWA Short Name | %s |\n", a.PWAShortName)
	}
	if a.HeaderMessage != "" {
		fmt.Fprintf(&sb, "| Header Message | %s |\n", a.HeaderMessage)
	}
	if a.FooterMessage != "" {
		fmt.Fprintf(&sb, "| Footer Message | %s |\n", a.FooterMessage)
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

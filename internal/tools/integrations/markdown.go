// markdown.go provides Markdown formatting functions for project integration MCP tool output.
package integrations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of integrations.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Integrations) == 0 {
		return toolutil.ToolResultWithMarkdown("No integrations found for this project.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Integrations (%d)\n\n", len(out.Integrations))
	sb.WriteString("| ID | Title | Slug | Active |\n")
	sb.WriteString("|----|-------|------|--------|\n")
	for _, i := range out.Integrations {
		active := "No"
		if i.Active {
			active = "Yes"
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", i.ID, toolutil.EscapeMdTableCell(i.Title), i.Slug, active)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_integration` to view details of a specific integration")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetMarkdown formats a single integration.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	i := out.Integration
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Integration: %s\n\n", i.Title)
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	fmt.Fprintf(&sb, "- **Slug**: %s\n", i.Slug)
	active := "No"
	if i.Active {
		active = "Yes"
	}
	fmt.Fprintf(&sb, "- **Active**: %s\n", active)
	if i.CreatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdCreated, toolutil.FormatTime(i.CreatedAt))
	}
	if i.UpdatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdUpdated, toolutil.FormatTime(i.UpdatedAt))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_integration` to modify this integration's settings")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
}

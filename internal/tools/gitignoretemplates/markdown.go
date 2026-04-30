// markdown.go provides Markdown formatting functions for gitignore template MCP tool output.
package gitignoretemplates

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Gitignore Templates\n\n")
	toolutil.WriteListSummary(&sb, len(out.Templates), out.Pagination)
	if len(out.Templates) == 0 {
		sb.WriteString("No templates found.\n")
		return sb.String()
	}
	sb.WriteString("| Key | Name |\n|---|---|\n")
	for _, t := range out.Templates {
		fmt.Fprintf(&sb, "| %s | %s |\n",
			toolutil.EscapeMdTableCell(t.Key), toolutil.EscapeMdTableCell(t.Name))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_gitignore_template` to view a specific template")
	return sb.String()
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Gitignore Template: %s\n\n", out.Name)
	sb.WriteString("```gitignore\n")
	sb.WriteString(out.Content)
	sb.WriteString("\n```\n")
	toolutil.WriteHints(&sb, "Copy this template to your `.gitignore` file and customize it")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

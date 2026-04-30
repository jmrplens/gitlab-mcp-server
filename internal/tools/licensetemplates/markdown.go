// markdown.go provides Markdown formatting functions for license template MCP tool output.
package licensetemplates

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## License Templates\n\n")
	toolutil.WriteListSummary(&sb, len(out.Licenses), out.Pagination)
	if len(out.Licenses) == 0 {
		sb.WriteString("No license templates found.\n")
		return sb.String()
	}
	sb.WriteString("| Key | Name | Featured |\n|---|---|---|\n")
	for _, l := range out.Licenses {
		fmt.Fprintf(&sb, "| %s | %s | %v |\n",
			toolutil.EscapeMdTableCell(l.Key), toolutil.EscapeMdTableCell(l.Name), l.Featured)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_license_template` to view a specific template")
	return sb.String()
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## License: %s\n\n", out.Name)
	if out.Description != "" {
		fmt.Fprintf(&sb, "**Description**: %s\n\n", out.Description)
	}
	if len(out.Permissions) > 0 {
		fmt.Fprintf(&sb, "**Permissions**: %s\n", strings.Join(out.Permissions, ", "))
	}
	if len(out.Conditions) > 0 {
		fmt.Fprintf(&sb, "**Conditions**: %s\n", strings.Join(out.Conditions, ", "))
	}
	if len(out.Limitations) > 0 {
		fmt.Fprintf(&sb, "**Limitations**: %s\n", strings.Join(out.Limitations, ", "))
	}
	if out.Content != "" {
		sb.WriteString("\n```\n")
		sb.WriteString(out.Content)
		sb.WriteString("\n```\n")
	}
	toolutil.WriteHints(&sb, "Copy this template to your LICENSE file and customize it")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

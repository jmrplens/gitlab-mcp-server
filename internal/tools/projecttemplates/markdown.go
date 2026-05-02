package projecttemplates

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats a list of project templates as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Project Templates\n\n")
	toolutil.WriteListSummary(&sb, len(out.Templates), out.Pagination)
	if len(out.Templates) == 0 {
		sb.WriteString("No templates found.\n")
		return sb.String()
	}
	sb.WriteString("| Key | Name | Popular |\n|-----|------|---------|\n")
	for _, t := range out.Templates {
		pop := ""
		if t.Popular {
			pop = "Yes"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(t.Key),
			toolutil.EscapeMdTableCell(t.Name),
			pop)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_get_project_template` to view a specific template")
	return sb.String()
}

// FormatGetMarkdown formats a single project template as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Template: %s\n\n", out.Name)
	fmt.Fprintf(&sb, "- **Key**: %s\n", out.Key)
	if out.Nickname != "" {
		fmt.Fprintf(&sb, "- **Nickname**: %s\n", out.Nickname)
	}
	if out.Popular {
		sb.WriteString("- **Popular**: Yes\n")
	}
	if out.Description != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdDescription, out.Description)
	}
	if len(out.Permissions) > 0 {
		fmt.Fprintf(&sb, "- **Permissions**: %s\n", strings.Join(out.Permissions, ", "))
	}
	if len(out.Conditions) > 0 {
		fmt.Fprintf(&sb, "- **Conditions**: %s\n", strings.Join(out.Conditions, ", "))
	}
	if len(out.Limitations) > 0 {
		fmt.Fprintf(&sb, "- **Limitations**: %s\n", strings.Join(out.Limitations, ", "))
	}
	if out.Content != "" {
		fmt.Fprintf(&sb, "\n### Content\n\n```\n%s\n```\n", out.Content)
	}
	toolutil.WriteHints(&sb, "Use this template when creating new project files")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

// markdown.go provides human-readable Markdown formatters for project aliases.
package projectaliases

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single project alias as Markdown.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Alias: %s\n\n", out.Name)
	fmt.Fprintf(&sb, "| Field | Value |\n")
	fmt.Fprintf(&sb, "|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Name | `%s` |\n", out.Name)
	fmt.Fprintf(&sb, "| Project ID | %d |\n", out.ProjectID)
	toolutil.WriteHints(&sb,
		"Use `gitlab_delete_project_alias` to remove this alias",
		"Use `gitlab_list_project_aliases` to view all aliases",
	)
	return sb.String()
}

// FormatListMarkdown formats a list of project aliases as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Aliases (%d)\n\n", len(out.Aliases))
	if len(out.Aliases) == 0 {
		sb.WriteString("No project aliases found.\n")
		return sb.String()
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	fmt.Fprintf(&sb, "| ID | Name | Project ID |\n")
	fmt.Fprintf(&sb, "|----|------|------------|\n")
	for _, a := range out.Aliases {
		fmt.Fprintf(&sb, "| %d | `%s` | %d |\n", a.ID, a.Name, a.ProjectID)
	}
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}

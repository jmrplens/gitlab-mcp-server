package groupvariables

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	return toolutil.FormatCICDVariableDetailMarkdown(toMarkdownVariable(v), "Group Variable", true)
}

// FormatListMarkdown renders a paginated list of group CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableCollectionMarkdown(out.Variables, out.Pagination, toMarkdownVariable, "Group CI/CD Variables", "No group CI/CD variables found.\n", true,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new group variable",
	)
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, v.Protected, v.Masked, v.Hidden, v.Raw, v.EnvironmentScope, v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

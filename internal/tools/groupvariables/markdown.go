package groupvariables

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single group CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	variable := toMarkdownVariable(v)
	return toolutil.FormatCICDVariableDetailMarkdown(variable, "Group Variable", true)
}

// FormatListMarkdown renders a paginated list of group CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	variables := out.Variables
	return toolutil.FormatCICDVariableCollectionMarkdown(
		variables, out.Pagination, toMarkdownVariable, "Group CI/CD Variables", "No group CI/CD variables found.\n", true,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new group variable",
	)
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	markdown := toolutil.CICDVariableMarkdown{}
	markdown.Key = v.Key
	markdown.Value = v.Value
	markdown.VariableType = v.VariableType
	markdown.Protected = v.Protected
	markdown.Masked = v.Masked
	markdown.Hidden = v.Hidden
	markdown.Raw = v.Raw
	markdown.EnvironmentScope = v.EnvironmentScope
	markdown.Description = v.Description
	return markdown
}

func init() {
	toolutil.RegisterMarkdownPair(FormatOutputMarkdown, FormatListMarkdown)
}

package civariables

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	return toolutil.FormatCICDVariableMarkdown(toMarkdownVariable(v), toolutil.CICDVariableMarkdownOptions{
		Title:                   "Variable",
		IncludeEnvironmentScope: true,
		Hints: []string{
			"Use action 'update' to change this variable",
			"Use action 'delete' to remove this variable",
		},
	})
}

// FormatListMarkdown renders a paginated list of CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableListMarkdown(toolutil.CICDVariableMarkdowns(out.Variables, toMarkdownVariable), out.Pagination, toolutil.CICDVariableListMarkdownOptions{
		Title:                   "CI/CD Variables",
		EmptyMessage:            "No CI/CD variables found.\n",
		IncludeEnvironmentScope: true,
		Hints: []string{
			"Use action 'get' with a key to see variable details",
			"Use action 'create' to add a new CI/CD variable",
		},
	})
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, v.Protected, v.Masked, v.Hidden, v.Raw, v.EnvironmentScope, v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

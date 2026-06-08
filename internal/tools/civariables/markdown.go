package civariables

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a single CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	return toolutil.FormatCICDVariableDetailMarkdown(toMarkdownVariable(v), "Variable", true)
}

// FormatListMarkdown renders a paginated list of CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableCollectionMarkdown(
		out.Variables, out.Pagination, toMarkdownVariable, "CI/CD Variables", "No CI/CD variables found.\n", true,
		"Use action 'get' with a key to see variable details",
		"Use action 'create' to add a new CI/CD variable",
	)
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	flags := toolutil.CICDVariableFlags{Protected: v.Protected, Masked: v.Masked, Hidden: v.Hidden, Raw: v.Raw}
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, flags, v.EnvironmentScope, v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

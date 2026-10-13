package civariables

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	return toolutil.FormatCICDVariableDetailMarkdown(toMarkdownVariable(v), "Variable", true,
		toolutil.HintAction("ci_variable.update", "change this variable"),
		toolutil.HintAction("ci_variable.delete", "remove this variable"),
	)
}

// FormatListMarkdown renders a paginated list of CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableCollectionMarkdown(
		out.Variables, out.Pagination, toMarkdownVariable, "CI/CD Variables", "No CI/CD variables found.\n", true,
		toolutil.HintAction("ci_variable.get", "see a variable's details, given its key"),
		toolutil.HintAction("ci_variable.create", "add a new CI/CD variable"),
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

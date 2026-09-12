package instancevariables

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single instance CI/CD variable as Markdown.
//
// Hidden is the second half of the guard and was missing: GitLab lets an
// instance variable be created with the value hidden from every later reader,
// and it sends that value back to an administrator's own request, so a card
// that asked only whether the variable was masked printed it. The shared
// renderer this delegates to has held both halves since hidden variables
// existed, and it reports the flag as well, which the card did not.
func FormatOutputMarkdown(v Output) string {
	return toolutil.FormatCICDVariableDetailMarkdown(toMarkdownVariable(v), "Instance Variable", false)
}

// FormatListMarkdown renders a paginated list of instance CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableCollectionMarkdown(
		out.Variables, out.Pagination, toMarkdownVariable, "Instance CI/CD Variables", "No instance CI/CD variables found.\n", false,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new instance variable",
	)
}

// toMarkdownVariable maps an instance variable onto the shared view model.
// Hidden travels with Masked because the shared renderer withholds the value
// for either: a variable created with masked_and_hidden is never shown again
// in GitLab's own UI, whether or not it is also masked.
func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	flags := toolutil.CICDVariableFlags{Protected: v.Protected, Masked: v.Masked, Hidden: v.Hidden, Raw: v.Raw}
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, flags, "", v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

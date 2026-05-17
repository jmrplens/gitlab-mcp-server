package gitignoretemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

var markdownRenderer = toolutil.TemplateRenderer{
	ListTitle:    "Gitignore Templates",
	EmptyMessage: "No templates found.\n",
	ListHint:     "Use `gitlab_get_gitignore_template` to view a specific template",
	DetailTitle:  "Gitignore Template",
	Language:     "gitignore",
	DetailHint:   "Copy this template to your `.gitignore` file and customize it",
}

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	return markdownRenderer.FormatList(out.Templates, out.Pagination)
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return markdownRenderer.FormatContent(out.Name, out.Content)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

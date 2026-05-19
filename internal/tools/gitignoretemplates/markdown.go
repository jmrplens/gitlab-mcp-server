package gitignoretemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var markdownRenderer = toolutil.NewTemplateRenderer("Gitignore Templates", "No templates found.\n", "Use `gitlab_get_gitignore_template` to view a specific template", "Gitignore Template", "gitignore", "Copy this template to your `.gitignore` file and customize it")

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	pagination := out.Pagination
	return markdownRenderer.FormatList(out.Templates, pagination)
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	content := out.Content
	return markdownRenderer.FormatContent(out.Name, content)
}

func init() {
	toolutil.RegisterMarkdownPair(FormatListMarkdown, FormatGetMarkdown)
}

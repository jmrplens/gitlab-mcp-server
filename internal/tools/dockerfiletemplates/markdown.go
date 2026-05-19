package dockerfiletemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var markdownRenderer = toolutil.NewTemplateRenderer("Dockerfile Templates", "No templates found.\n", "Use `gitlab_get_dockerfile_template` to view a specific template", "Dockerfile Template", "dockerfile", "Copy this template to your Dockerfile and customize it")

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	templates := out.Templates
	return markdownRenderer.FormatList(templates, out.Pagination)
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	name, content := out.Name, out.Content
	return markdownRenderer.FormatContent(name, content)
}

func init() {
	toolutil.RegisterMarkdownPair(FormatListMarkdown, FormatGetMarkdown)
}

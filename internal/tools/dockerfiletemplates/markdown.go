package dockerfiletemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

var markdownRenderer = toolutil.TemplateRenderer{
	ListTitle:    "Dockerfile Templates",
	EmptyMessage: "No templates found.\n",
	ListHint:     "Use `gitlab_get_dockerfile_template` to view a specific template",
	DetailTitle:  "Dockerfile Template",
	Language:     "dockerfile",
	DetailHint:   "Copy this template to your Dockerfile and customize it",
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

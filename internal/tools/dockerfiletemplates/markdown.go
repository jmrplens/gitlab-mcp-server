package dockerfiletemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatTemplateListMarkdown(toolutil.TemplateMarkdowns(out.Templates, toTemplateMarkdown), out.Pagination, toolutil.TemplateListMarkdownOptions{
		Title:        "Dockerfile Templates",
		EmptyMessage: "No templates found.\n",
		Hint:         "Use `gitlab_get_dockerfile_template` to view a specific template",
	})
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return toolutil.FormatTemplateContentMarkdown("Dockerfile Template", out.Name, "dockerfile", out.Content, "Copy this template to your Dockerfile and customize it")
}

func toTemplateMarkdown(template TemplateListItem) toolutil.TemplateMarkdown {
	return toolutil.NewTemplateMarkdown(template.Key, template.Name)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

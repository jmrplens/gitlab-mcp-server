package ciyamltemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatTemplateListMarkdown(toolutil.TemplateMarkdowns(out.Templates, toTemplateMarkdown), out.Pagination, toolutil.TemplateListMarkdownOptions{
		Title:        "CI YAML Templates",
		EmptyMessage: "No templates found.\n",
		Hint:         "Use `gitlab_get_ci_yaml_template` to view a specific template",
	})
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return toolutil.FormatTemplateContentMarkdown("CI YAML Template", out.Name, "yaml", out.Content, "Copy this template to your `.gitlab-ci.yml` file and customize it")
}

func toTemplateMarkdown(template TemplateListItem) toolutil.TemplateMarkdown {
	return toolutil.NewTemplateMarkdown(template.Key, template.Name)
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

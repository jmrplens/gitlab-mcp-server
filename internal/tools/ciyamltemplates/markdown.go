package ciyamltemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var markdownRenderer = toolutil.NewTemplateRenderer("CI YAML Templates", "No templates found.\n", "Use `gitlab_get_ci_yaml_template` to view a specific template", "CI YAML Template", "yaml", "Copy this template to your `.gitlab-ci.yml` file and customize it")

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	return markdownRenderer.FormatList(out.Templates, out.Pagination)
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return markdownRenderer.FormatContent(out.Name, out.Content)
}

func init() {
	toolutil.RegisterMarkdownPair(FormatListMarkdown, FormatGetMarkdown)
}

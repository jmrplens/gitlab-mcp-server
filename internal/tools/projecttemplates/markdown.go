package projecttemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats a list of project templates as markdown.
func FormatListMarkdown(out ListOutput) string {
	items := make([]toolutil.TemplateAttributeListMarkdownItem, 0, len(out.Templates))
	for _, template := range out.Templates {
		items = append(items, toolutil.TemplateAttributeListMarkdownItem{Key: template.Key, Name: template.Name, Attribute: popularLabel(template.Popular)})
	}
	return toolutil.FormatTemplateAttributeListMarkdown(items, toolutil.TemplateAttributeListMarkdownOptions{
		Title:           "Project Templates",
		EmptyMessage:    "No templates found.",
		AttributeHeader: "Popular",
		Pagination:      out.Pagination,
		Hints:           []string{"Use `gitlab_get_project_template` to view a specific template"},
	})
}

// FormatGetMarkdown formats a single project template as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return toolutil.FormatTemplateDetailMarkdown(toolutil.TemplateDetailMarkdown{
		Title:          "Project Template: " + out.Name,
		Key:            out.Key,
		Nickname:       out.Nickname,
		Popular:        out.Popular,
		Description:    out.Description,
		Permissions:    out.Permissions,
		Conditions:     out.Conditions,
		Limitations:    out.Limitations,
		Content:        out.Content,
		ContentHeading: "Content",
		Hints:          []string{"Use this template when creating new project files"},
	})
}

func popularLabel(popular bool) string {
	if popular {
		return "Yes"
	}
	return ""
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

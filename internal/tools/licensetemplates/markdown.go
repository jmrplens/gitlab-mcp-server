package licensetemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	items := make([]toolutil.TemplateAttributeListMarkdownItem, 0, len(out.Licenses))
	for _, license := range out.Licenses {
		items = append(items, toolutil.TemplateAttributeListMarkdownItem{Key: license.Key, Name: license.Name, Attribute: boolString(license.Featured)})
	}
	return toolutil.FormatTemplateAttributeListMarkdown(items, toolutil.TemplateAttributeListMarkdownOptions{
		Title:           "License Templates",
		EmptyMessage:    "No license templates found.",
		AttributeHeader: "Featured",
		Pagination:      out.Pagination,
		Hints:           []string{"Use `gitlab_get_license_template` to view a specific template"},
	})
}

// FormatGetMarkdown formats the get output as markdown.
func FormatGetMarkdown(out GetOutput) string {
	return toolutil.FormatTemplateDetailMarkdown(toolutil.TemplateDetailMarkdown{
		Title:       "License: " + out.Name,
		Description: out.Description,
		Permissions: out.Permissions,
		Conditions:  out.Conditions,
		Limitations: out.Limitations,
		Content:     out.Content,
		PlainFields: true,
		Hints:       []string{"Copy this template to your LICENSE file and customize it"},
	})
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

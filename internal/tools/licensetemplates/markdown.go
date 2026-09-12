package licensetemplates

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown formats the list output as markdown.
func FormatListMarkdown(out ListOutput) string {
	items := make([]toolutil.TemplateAttributeListMarkdownItem, 0, len(out.Licenses))
	for _, license := range out.Licenses {
		items = append(items, toolutil.TemplateAttributeListMarkdownItem{Key: license.Key, Name: license.Name, Attribute: toolutil.BoolEmoji(license.Popular)})
	}
	return toolutil.FormatTemplateAttributeListMarkdown(items, toolutil.TemplateAttributeListMarkdownOptions{
		Title:           "License Templates",
		EmptyMessage:    "No license templates found.",
		AttributeHeader: "Popular",
		Pagination:      out.Pagination,
		Hints:           []string{"Use `gitlab_get_license_template` to view a specific template"},
	})
}

// FormatGetMarkdown renders one license template as the shared template card.
//
// The key, the nickname and the popular flag are GitLab's own answer for this
// template and the card has had rows for all three since the shared renderer
// was written; this formatter used to drop them, so the key a reader needs to
// ask for the template again was nowhere on the page that named it.
func FormatGetMarkdown(out GetOutput) string {
	return toolutil.FormatTemplateDetailMarkdown(toolutil.TemplateDetailMarkdown{
		Title:       "License: " + out.Name,
		Key:         out.Key,
		Nickname:    out.Nickname,
		Popular:     out.Popular,
		Description: out.Description,
		Permissions: out.Permissions,
		Conditions:  out.Conditions,
		Limitations: out.Limitations,
		Content:     out.Content,
		Hints:       []string{"Copy this template to your LICENSE file and customize it"},
	})
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

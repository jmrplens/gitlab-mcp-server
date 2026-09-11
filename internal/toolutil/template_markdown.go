package toolutil

import (
	"strings"
)

// TemplateAttributeListMarkdownItem carries common list-row fields for
// template-style Markdown tables, with an optional third attribute column.
type TemplateAttributeListMarkdownItem struct {
	Key       string
	Name      string
	Attribute string
}

// TemplateAttributeListMarkdownOptions configures template-style list
// rendering. An empty AttributeHeader renders the two-column Key/Name table
// the plain template families use; a named one adds the third column.
type TemplateAttributeListMarkdownOptions struct {
	Title           string
	EmptyMessage    string
	AttributeHeader string
	Pagination      PaginationOutput
	Hints           []string
}

// FormatTemplateAttributeListMarkdown renders a template list as a table, in
// the two-column and the three-column shape alike, so the two families share
// one heading, one empty form and one footer. Templates carry no link, so
// the footer carries no instruction to keep them.
func FormatTemplateAttributeListMarkdown(items []TemplateAttributeListMarkdownItem, opts TemplateAttributeListMarkdownOptions) string {
	if len(items) == 0 {
		return emptyResult(opts.EmptyMessage)
	}
	var b strings.Builder
	WriteListHeading(&b, opts.Title, len(items), opts.Pagination)
	withAttribute := opts.AttributeHeader != ""
	if withAttribute {
		b.WriteString(MarkdownTableHeader("Key", "Name", opts.AttributeHeader))
	} else {
		b.WriteString(MarkdownTableHeader("Key", "Name"))
	}
	for _, item := range items {
		if withAttribute {
			b.WriteString(MarkdownTableRow(EscapeMdTableCell(item.Key), EscapeMdTableCell(item.Name), EscapeMdTableCell(item.Attribute)))
			continue
		}
		b.WriteString(MarkdownTableRow(EscapeMdTableCell(item.Key), EscapeMdTableCell(item.Name)))
	}
	WriteListFooter(&b, opts.Pagination, false, opts.Hints...)
	return b.String()
}

// TemplateDetailMarkdown carries common fields for template-style detail
// pages. Every family renders its fields the same way, as card rows: the
// switch that let the license family write bullet-less label lines is gone,
// since consecutive lines of that shape rendered as one run-on paragraph.
type TemplateDetailMarkdown struct {
	Title          string
	Key            string
	Nickname       string
	Popular        bool
	Description    string
	Permissions    []string
	Conditions     []string
	Limitations    []string
	Content        string
	ContentHeading string
	Hints          []string
}

// FormatTemplateDetailMarkdown renders a template detail page as a card: the
// identity rows, the description as the card's long text, the three lists
// joined on their rows, and the content in a fence under its heading when the
// caller named one. The project-template endpoints also serve issue and
// merge-request description templates, whose name is a file in
// .gitlab/issue_templates named by whoever pushed it, so the heading is
// escaped as every card heading is.
func FormatTemplateDetailMarkdown(detail TemplateDetailMarkdown) string {
	var b strings.Builder
	c := NewCard(&b, detail.Title)
	c.Field("Key", detail.Key)
	c.Field("Nickname", detail.Nickname)
	if detail.Popular {
		c.Bool("Popular", true)
	}
	c.Text("Description", detail.Description)
	// The permissions, conditions and limitations arrays come back as the API
	// sent them, and nothing this server controls constrains their contents;
	// the row escapes the joined value.
	c.Field("Permissions", strings.Join(detail.Permissions, ", "))
	c.Field("Conditions", strings.Join(detail.Conditions, ", "))
	c.Field("Limitations", strings.Join(detail.Limitations, ", "))
	c.Fence(detail.ContentHeading, "", detail.Content)
	c.End(detail.Hints...)
	return b.String()
}

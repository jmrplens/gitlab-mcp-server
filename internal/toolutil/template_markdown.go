package toolutil

import (
	"fmt"
	"strings"
)

// TemplateAttributeListMarkdownItem carries common list-row fields for
// template-style Markdown tables with a third attribute column.
type TemplateAttributeListMarkdownItem struct {
	Key       string
	Name      string
	Attribute string
}

// TemplateAttributeListMarkdownOptions configures template-style list rendering
// for tables that include a third attribute column.
type TemplateAttributeListMarkdownOptions struct {
	Title           string
	EmptyMessage    string
	AttributeHeader string
	Pagination      PaginationOutput
	Hints           []string
}

// FormatTemplateAttributeListMarkdown renders a common Key/Name/Attribute
// template list without changing the JSON schema used by existing template tools.
func FormatTemplateAttributeListMarkdown(items []TemplateAttributeListMarkdownItem, opts TemplateAttributeListMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, FmtMdH2, opts.Title)
	WriteListSummary(&b, len(items), opts.Pagination)
	if len(items) == 0 {
		b.WriteString(opts.EmptyMessage)
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(MarkdownTableHeader("Key", "Name", opts.AttributeHeader))
	for _, item := range items {
		b.WriteString(MarkdownTableRow(
			EscapeMdTableCell(item.Key),
			EscapeMdTableCell(item.Name),
			EscapeMdTableCell(item.Attribute),
		))
	}
	WritePagination(&b, opts.Pagination)
	WriteHints(&b, ListHints(opts.Hints...)...)
	return b.String()
}

// TemplateDetailMarkdown carries common fields for template-style detail pages.
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
	PlainFields    bool
	Hints          []string
}

// FormatTemplateDetailMarkdown renders a shared template detail layout.
func FormatTemplateDetailMarkdown(detail TemplateDetailMarkdown) string {
	var b strings.Builder
	fmt.Fprintf(&b, FmtMdH2, detail.Title)
	if detail.Key != "" {
		fmt.Fprintf(&b, "- **Key**: %s\n", detail.Key)
	}
	if detail.Nickname != "" {
		fmt.Fprintf(&b, "- **Nickname**: %s\n", detail.Nickname)
	}
	if detail.Popular {
		b.WriteString("- **Popular**: Yes\n")
	}
	if detail.Description != "" {
		writeTemplateDescription(&b, detail.Description, detail.PlainFields)
	}
	writeTemplateDetailList(&b, "Permissions", detail.Permissions, detail.PlainFields)
	writeTemplateDetailList(&b, "Conditions", detail.Conditions, detail.PlainFields)
	writeTemplateDetailList(&b, "Limitations", detail.Limitations, detail.PlainFields)
	if detail.Content != "" {
		if detail.ContentHeading != "" {
			fmt.Fprintf(&b, "\n### %s\n\n", detail.ContentHeading)
		} else {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "```\n%s\n```\n", detail.Content)
	}
	WriteHints(&b, detail.Hints...)
	return b.String()
}

func writeTemplateDescription(b *strings.Builder, description string, plain bool) {
	if plain {
		fmt.Fprintf(b, "**Description**: %s\n\n", description)
		return
	}
	fmt.Fprintf(b, FmtMdDescription, description)
}

func writeTemplateDetailList(b *strings.Builder, label string, values []string, plain bool) {
	if len(values) == 0 {
		return
	}
	if plain {
		fmt.Fprintf(b, "**%s**: %s\n", label, strings.Join(values, ", "))
		return
	}
	fmt.Fprintf(b, "- **%s**: %s\n", label, strings.Join(values, ", "))
}

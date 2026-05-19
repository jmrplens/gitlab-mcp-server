package toolutil

import (
	"fmt"
	"strings"
)

// LabelMarkdown holds the common fields rendered for project and group labels.
type LabelMarkdown struct {
	ID                     int64
	Name                   string
	Color                  string
	Description            string
	OpenIssuesCount        int64
	ClosedIssuesCount      int64
	OpenMergeRequestsCount int64
	Priority               int64
	PrioritySpecified      bool
	IsProjectLabel         bool
	Subscribed             bool
}

// LabelMarkdownOptions controls label detail and list Markdown copy.
type LabelMarkdownOptions struct {
	DetailTitle       string
	ListTitle         string
	EmptyListText     string
	DetailHints       []string
	ListHints         []string
	EscapeDescription bool
}

// FormatLabelMarkdown renders a project or group label as a Markdown summary.
func FormatLabelMarkdown(label LabelMarkdown, opts LabelMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s: %s\n\n", opts.DetailTitle, EscapeMdHeading(label.Name))
	fmt.Fprintf(&b, FmtMdID, label.ID)
	fmt.Fprintf(&b, "- **Color**: %s\n", label.Color)
	if label.Description != "" {
		description := label.Description
		if opts.EscapeDescription {
			description = EscapeMdTableCell(description)
		}
		fmt.Fprintf(&b, FmtMdDescription, description)
	}
	if label.PrioritySpecified || label.Priority != 0 {
		fmt.Fprintf(&b, "- **Priority**: %d\n", label.Priority)
	}
	fmt.Fprintf(&b, "- **Project label**: %v\n", label.IsProjectLabel)
	fmt.Fprintf(&b, "- **Subscribed**: %v\n", label.Subscribed)
	if label.OpenIssuesCount > 0 || label.ClosedIssuesCount > 0 || label.OpenMergeRequestsCount > 0 {
		fmt.Fprintf(&b, "- **Issues**: %d open, %d closed\n", label.OpenIssuesCount, label.ClosedIssuesCount)
		fmt.Fprintf(&b, "- **Open MRs**: %d\n", label.OpenMergeRequestsCount)
	}
	WriteHints(&b, opts.DetailHints...)
	return b.String()
}

// FormatLabelListMarkdown renders project or group labels as a paginated table.
func FormatLabelListMarkdown(labels []LabelMarkdown, pagination PaginationOutput, opts LabelMarkdownOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n\n", opts.ListTitle, pagination.TotalItems)
	WriteListSummary(&b, len(labels), pagination)
	if len(labels) == 0 {
		b.WriteString(opts.EmptyListText)
		b.WriteString("\n")
		WriteHints(&b, opts.ListHints...)
		return b.String()
	}
	b.WriteString("| Name | Color | Open Issues | Closed Issues | Open MRs |\n")
	b.WriteString("|------|-------|-------------|---------------|----------|\n")
	for _, label := range labels {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d |\n",
			EscapeMdTableCell(label.Name), EscapeMdTableCell(label.Color), label.OpenIssuesCount, label.ClosedIssuesCount, label.OpenMergeRequestsCount)
	}
	WritePagination(&b, pagination)
	WriteHints(&b, opts.ListHints...)
	return b.String()
}

// FormatLabelListMarkdownFunc renders labels after mapping domain-specific outputs to the shared Markdown view.
func FormatLabelListMarkdownFunc[T any](labels []T, pagination PaginationOutput, opts LabelMarkdownOptions, convert func(T) LabelMarkdown) string {
	items := make([]LabelMarkdown, len(labels))
	for i, label := range labels {
		items[i] = convert(label)
	}
	return FormatLabelListMarkdown(items, pagination, opts)
}

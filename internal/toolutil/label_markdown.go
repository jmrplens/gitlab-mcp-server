package toolutil

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// LabelMarkdown holds the common fields rendered for project and group labels.
//
// Archived is here because GitLab archives a label rather than deleting it,
// and a card that says nothing about it shows an archived label exactly as it
// shows a live one: the label carried the flag on its output type and no
// renderer read it.
//
// CountsSent says whether GitLab sent the three usage counts beside it.
// lib/api/entities/label.rb exposes them only under options[:with_counts],
// which GitLab sets from the with_counts parameter of the two label listings
// and on no other route, so a label from any other answer carries none, and
// rendering its zeros would read as a label nothing uses (issue 1174).
type LabelMarkdown struct {
	ID                     int64
	Name                   string
	Color                  string
	Description            string
	CountsSent             bool
	OpenIssuesCount        int64
	ClosedIssuesCount      int64
	OpenMergeRequestsCount int64
	Priority               int64
	PrioritySpecified      bool
	IsProjectLabel         bool
	Subscribed             bool
	Archived               bool
}

// LabelMarkdownOptions controls label detail and list Markdown copy. The
// description is rendered one way for every caller, as a card row escaped
// through the card, so there is no switch for it: the option that let one
// scope escape and the other not made one entity answer the same question
// two ways.
type LabelMarkdownOptions struct {
	DetailTitle   string
	ListTitle     string
	EmptyListText string
	DetailHints   []string
	ListHints     []string
}

// FormatLabelMarkdown renders a project or group label as a card: the
// identity rows, the description as the card's long text, the priority when
// GitLab sent one, the two flags as glyphs, and the counters when GitLab sent
// them, zeros included, since a label a listing counted and nothing uses is
// an answer of its own.
func FormatLabelMarkdown(label LabelMarkdown, opts LabelMarkdownOptions) string {
	var b strings.Builder
	c := NewCard(&b, opts.DetailTitle+": "+label.Name)
	c.Int("ID", label.ID)
	c.Field("Color", label.Color)
	c.Text("Description", label.Description)
	if label.PrioritySpecified || label.Priority != 0 {
		c.Int("Priority", label.Priority)
	}
	c.Bool("Project label", label.IsProjectLabel)
	c.Bool("Subscribed", label.Subscribed)
	c.Flag(EmojiArchived, "Archived", label.Archived)
	if label.CountsSent {
		c.Field("Issues", fmt.Sprintf("%d open, %d closed", label.OpenIssuesCount, label.ClosedIssuesCount))
		c.Int("Open MRs", label.OpenMergeRequestsCount)
	}
	c.End(opts.DetailHints...)
	return b.String()
}

// formatLabelListMarkdown renders project or group labels as a paginated
// table, one row per label with its scope. Labels carry no link, so the
// footer carries no instruction to keep them, and an empty list is the
// configured message alone.
//
// The three count columns appear only on a page whose labels GitLab counted,
// which is a listing asked for with with_counts; on every other page they
// were a column of zeros GitLab never sent.
func formatLabelListMarkdown(labels []LabelMarkdown, pagination PaginationOutput, opts LabelMarkdownOptions) string {
	if len(labels) == 0 {
		return emptyResult(opts.EmptyListText)
	}
	var b strings.Builder
	WriteListHeading(&b, opts.ListTitle, len(labels), pagination)
	counted := slices.ContainsFunc(labels, func(label LabelMarkdown) bool { return label.CountsSent })
	columns := []string{"Name", "Color", "Scope"}
	if counted {
		columns = append(columns, "Open Issues", "Closed Issues", "Open MRs")
	}
	b.WriteString(MarkdownTableHeader(columns...))
	for _, label := range labels {
		cells := []string{
			labelNameCell(label),
			EscapeMdTableCell(label.Color),
			labelScope(label.IsProjectLabel),
		}
		if counted {
			cells = append(cells,
				strconv.FormatInt(label.OpenIssuesCount, 10),
				strconv.FormatInt(label.ClosedIssuesCount, 10),
				strconv.FormatInt(label.OpenMergeRequestsCount, 10),
			)
		}
		b.WriteString(MarkdownTableRow(cells...))
	}
	WriteListFooter(&b, pagination, false, opts.ListHints...)
	return b.String()
}

// labelNameCell renders a label's name, marked with the archive glyph when
// GitLab has archived it: an archived label is still returned by a list and
// still shown on an issue, and the row used to read exactly like a live one.
func labelNameCell(label LabelMarkdown) string {
	name := EscapeMdTableCell(label.Name)
	if !label.Archived {
		return name
	}
	return EmojiArchived + " " + name
}

// labelScope names where a label is defined, which a list mixing a project's
// own labels with the ones it inherits from its groups otherwise leaves the
// reader to guess.
func labelScope(project bool) string {
	if project {
		return "project"
	}
	return "group"
}

// FormatLabelListMarkdownFunc renders labels after mapping domain-specific outputs to the shared Markdown view.
func FormatLabelListMarkdownFunc[T any](labels []T, pagination PaginationOutput, opts LabelMarkdownOptions, convert func(T) LabelMarkdown) string {
	return formatLabelListMarkdown(mapSlice(labels, convert), pagination, opts)
}

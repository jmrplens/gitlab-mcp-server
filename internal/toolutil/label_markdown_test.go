package toolutil

import (
	"strings"
	"testing"
)

// TestFormatLabelMarkdown_AllBranches verifies the label card byte for byte
// with every optional row present: the heading escaped by the card, the
// description escaped on its row, the priority, the two flags as glyphs, the
// counters, and the hints last.
func TestFormatLabelMarkdown_AllBranches(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{
		ID:                     42,
		Name:                   "bug|fix",
		Color:                  "#ff0000",
		Description:            "one|two",
		CountsSent:             true,
		OpenIssuesCount:        3,
		ClosedIssuesCount:      2,
		OpenMergeRequestsCount: 1,
		Priority:               7,
		PrioritySpecified:      true,
		IsProjectLabel:         true,
		Subscribed:             true,
	}, LabelMarkdownOptions{
		DetailTitle: "Label",
		DetailHints: []string{"Use action 'label_update'"},
	})

	want := "## Label: bug|fix\n\n" +
		"- **ID**: 42\n" +
		"- **Color**: #ff0000\n" +
		"- **Description**: one&#124;two\n" +
		"- **Priority**: 7\n" +
		"- **Project label**: " + EmojiSuccess + "\n" +
		"- **Subscribed**: " + EmojiSuccess + "\n" +
		"- **Issues**: 3 open, 2 closed\n" +
		"- **Open MRs**: 1\n" +
		hintsSection("Use action 'label_update'")
	if got != want {
		t.Errorf("label card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelMarkdown_ZeroPrioritySpecified verifies GitLab priority 0 is
// rendered when the API explicitly returns it, and omitted when it did not.
func TestFormatLabelMarkdown_ZeroPrioritySpecified(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{ID: 1, Name: "zero", Color: "#111111", PrioritySpecified: true}, LabelMarkdownOptions{DetailTitle: "Label"})
	want := "## Label: zero\n\n" +
		"- **ID**: 1\n" +
		"- **Color**: #111111\n" +
		"- **Priority**: 0\n" +
		"- **Project label**: " + EmojiCross + "\n" +
		"- **Subscribed**: " + EmojiCross + "\n"
	if got != want {
		t.Errorf("label card with an explicit zero priority:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelMarkdown_NonZeroPriorityNotMarkedSpecified_IsRendered verifies
// a priority GitLab sent is rendered even by a caller that did not mark it
// specified: only zero is ambiguous, so only zero needs the mark.
func TestFormatLabelMarkdown_NonZeroPriorityNotMarkedSpecified_IsRendered(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{ID: 1, Name: "p", Color: "#111111", Priority: 3}, LabelMarkdownOptions{DetailTitle: "Label"})
	if !strings.Contains(got, "- **Priority**: 3\n") {
		t.Errorf("label card = %q, want the priority row", got)
	}
}

// TestFormatLabelMarkdown_Minimal_OmitsOptionalRowsAndEscapesTheDescription
// verifies optional rows are omitted and that a one-line description is
// escaped on its row: the description question has one answer for every
// caller, since the switch that let one scope leave it raw is gone.
func TestFormatLabelMarkdown_Minimal_OmitsOptionalRowsAndEscapesTheDescription(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{
		Name:        "docs",
		Color:       "#00ff00",
		Description: "plain|pipe",
	}, LabelMarkdownOptions{DetailTitle: "Group Label"})

	want := "## Group Label: docs\n\n" +
		"- **ID**: 0\n" +
		"- **Color**: #00ff00\n" +
		"- **Description**: plain&#124;pipe\n" +
		"- **Project label**: " + EmojiCross + "\n" +
		"- **Subscribed**: " + EmojiCross + "\n"
	if got != want {
		t.Errorf("minimal label card:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelMarkdown_MultiLineDescription_QuotesUnderTheLabel verifies
// that a description spanning several lines is quoted under its label and
// indented into the item, so a heading or a bullet typed into it cannot add
// structure to the card.
func TestFormatLabelMarkdown_MultiLineDescription_QuotesUnderTheLabel(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{
		Name:        "docs",
		Color:       "#00ff00",
		Description: "line one\n## not a heading\n- not an item",
	}, LabelMarkdownOptions{DetailTitle: "Label"})

	want := "## Label: docs\n\n" +
		"- **ID**: 0\n" +
		"- **Color**: #00ff00\n" +
		"- **Description**:\n" +
		"  > line one\n" +
		"  > ## not a heading\n" +
		"  > - not an item\n" +
		"- **Project label**: " + EmojiCross + "\n" +
		"- **Subscribed**: " + EmojiCross + "\n"
	if got != want {
		t.Errorf("label card with a multi-line description:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelMarkdown_Counters_RenderExactlyWhenGitLabSentThem verifies
// the counter rows follow whether GitLab sent the counts, not their values.
//
// GitLab sends a label's counts only to a listing that asked with_counts=true,
// and the card used to decide by whether any count was above zero: a label
// such a listing counted and nothing uses lost its answer, while the zeros of
// a label whose route sends no counts at all were the only thing keeping the
// rows away (issue 1174). Counts sent as zeros render as zeros, and counts the
// view carries without GitLab having sent them render nothing.
func TestFormatLabelMarkdown_Counters_RenderExactlyWhenGitLabSentThem(t *testing.T) {
	head := "## Label: bug\n\n- **ID**: 0\n- **Color**: #ff0000\n- **Project label**: " + EmojiCross + "\n- **Subscribed**: " + EmojiCross + "\n"
	for _, tc := range []struct {
		name  string
		label LabelMarkdown
		want  string
	}{
		{
			name:  "sent as zeros",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", CountsSent: true},
			want:  head + "- **Issues**: 0 open, 0 closed\n- **Open MRs**: 0\n",
		},
		{
			name:  "sent with values",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", CountsSent: true, OpenIssuesCount: 4, ClosedIssuesCount: 9, OpenMergeRequestsCount: 2},
			want:  head + "- **Issues**: 4 open, 9 closed\n- **Open MRs**: 2\n",
		},
		{
			name:  "not sent",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", OpenIssuesCount: 4, ClosedIssuesCount: 9, OpenMergeRequestsCount: 2},
			want:  head,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatLabelMarkdown(tc.label, LabelMarkdownOptions{DetailTitle: "Label"}); got != tc.want {
				t.Errorf("label card:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// labelListRow is the domain row the list tests map to the shared view, the
// shape FormatLabelListMarkdownFunc is handed by the label packages.
type labelListRow struct {
	Name                   string
	Color                  string
	CountsSent             bool
	OpenIssuesCount        int64
	ClosedIssuesCount      int64
	OpenMergeRequestsCount int64
	IsProjectLabel         bool
	Archived               bool
}

// formatLabelListRows renders rows through FormatLabelListMarkdownFunc with the
// project copy and a one-page pagination block of their own size.
func formatLabelListRows(rows []labelListRow) string {
	return FormatLabelListMarkdownFunc(rows, PaginationOutput{Page: 1, PerPage: 20, TotalItems: int64(len(rows)), TotalPages: 1}, LabelMarkdownOptions{
		ListTitle: "Labels",
		ListHints: []string{HintPreserveLinks, "Use action 'label_get'"},
	}, func(label labelListRow) LabelMarkdown {
		return LabelMarkdown{Name: label.Name, Color: label.Color, CountsSent: label.CountsSent, OpenIssuesCount: label.OpenIssuesCount, ClosedIssuesCount: label.ClosedIssuesCount, OpenMergeRequestsCount: label.OpenMergeRequestsCount, IsProjectLabel: label.IsProjectLabel, Archived: label.Archived}
	})
}

// TestFormatLabelListMarkdownFunc_WithLabels verifies the list byte for
// byte: the heading with GitLab's total, the table with the scope column and
// the counts GitLab sent, zeros included, the footer, and the caller's hint
// without a link hint, since labels carry no link.
//
// The archived row is in the table because GitLab archives a label rather
// than deleting it: it keeps coming back in every list and stays on the
// issues that carry it, so a row that reads exactly like a live one tells a
// model to keep using a label nobody may apply any more.
func TestFormatLabelListMarkdownFunc_WithLabels(t *testing.T) {
	got := formatLabelListRows([]labelListRow{
		{Name: "bug|fix", Color: "#ff0000", CountsSent: true, OpenIssuesCount: 3, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, IsProjectLabel: true},
		{Name: "inherited", Color: "#00ff00", CountsSent: true},
		{Name: "retired", Color: "#0000ff", CountsSent: true, IsProjectLabel: true, Archived: true},
	})

	want := "## Labels (3)\n\n" +
		"| Name | Color | Scope | Open Issues | Closed Issues | Open MRs |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| bug&#124;fix | #ff0000 | project | 3 | 2 | 1 |\n" +
		"| inherited | #00ff00 | group | 0 | 0 | 0 |\n" +
		"| " + EmojiArchived + " retired | #0000ff | project | 0 | 0 | 0 |\n" +
		"\nPage 1 of 1 | 3 items total | 20 per page\n" +
		hintsSection("Use action 'label_get'")
	if got != want {
		t.Errorf("label list:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelListMarkdownFunc_CountsNotSent_HasNoCountColumns verifies a
// page GitLab sent no counts for, which is every listing not asked with
// with_counts, carries neither the three count headings nor a count cell.
//
// Those columns used to be printed on every page, so a listing that asked
// for no counts showed 0 | 0 | 0 for every label, which reads as labels
// nothing uses (issue 1174). The rows carry counts the view was handed
// without GitLab having sent them, so a table that printed them would fail.
func TestFormatLabelListMarkdownFunc_CountsNotSent_HasNoCountColumns(t *testing.T) {
	got := formatLabelListRows([]labelListRow{
		{Name: "bug", Color: "#ff0000", OpenIssuesCount: 3, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, IsProjectLabel: true},
		{Name: "inherited", Color: "#00ff00"},
	})

	want := "## Labels (2)\n\n" +
		"| Name | Color | Scope |\n" +
		"| --- | --- | --- |\n" +
		"| bug | #ff0000 | project |\n" +
		"| inherited | #00ff00 | group |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		hintsSection("Use action 'label_get'")
	if got != want {
		t.Errorf("label list without counts:\n got %q\nwant %q", got, want)
	}
}

// TestFormatLabelListMarkdown_Empty verifies an empty list is the configured
// message alone, with its newline, and neither a heading nor a hint.
func TestFormatLabelListMarkdown_Empty(t *testing.T) {
	got := formatLabelListMarkdown(nil, PaginationOutput{}, LabelMarkdownOptions{
		ListTitle:     "Group Labels",
		EmptyListText: "No group labels found.",
		ListHints:     []string{"Use action 'group_label_create'"},
	})

	if want := "No group labels found.\n"; got != want {
		t.Errorf("empty label list = %q, want %q", got, want)
	}
}

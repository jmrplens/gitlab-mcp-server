package toolutil

import (
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

// TestFormatLabelMarkdown_CountersRenderWhenOnlyOneIsSet verifies the counter
// block appears when any single counter is non-zero rather than only when they
// all are.
//
// The ordinary shape of a label is exactly this: issues open and nothing
// closed, or merge requests and no issues at all. A label whose counters are
// all zero and one whose counters are all set both agree whatever the three
// checks are joined by, so neither says which join the renderer uses.
func TestFormatLabelMarkdown_CountersRenderWhenOnlyOneIsSet(t *testing.T) {
	head := "## Label: bug\n\n- **ID**: 0\n- **Color**: #ff0000\n- **Project label**: " + EmojiCross + "\n- **Subscribed**: " + EmojiCross + "\n"
	for _, tc := range []struct {
		name  string
		label LabelMarkdown
		want  string
	}{
		{
			name:  "open issues only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", OpenIssuesCount: 4},
			want:  head + "- **Issues**: 4 open, 0 closed\n- **Open MRs**: 0\n",
		},
		{
			name:  "closed issues only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", ClosedIssuesCount: 9},
			want:  head + "- **Issues**: 0 open, 9 closed\n- **Open MRs**: 0\n",
		},
		{
			name:  "open merge requests only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", OpenMergeRequestsCount: 2},
			want:  head + "- **Issues**: 0 open, 0 closed\n- **Open MRs**: 2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatLabelMarkdown(tc.label, LabelMarkdownOptions{DetailTitle: "Label"}); got != tc.want {
				t.Errorf("label card:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestFormatLabelListMarkdownFunc_WithLabels verifies the list byte for
// byte: the heading with GitLab's total, the table with the scope column,
// the footer, and the caller's hint without a link hint, since labels carry
// no link.
func TestFormatLabelListMarkdownFunc_WithLabels(t *testing.T) {
	type labelOutput struct {
		Name                   string
		Color                  string
		OpenIssuesCount        int64
		ClosedIssuesCount      int64
		OpenMergeRequestsCount int64
		IsProjectLabel         bool
	}
	got := FormatLabelListMarkdownFunc([]labelOutput{
		{Name: "bug|fix", Color: "#ff0000", OpenIssuesCount: 3, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1, IsProjectLabel: true},
		{Name: "inherited", Color: "#00ff00"},
	}, PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1}, LabelMarkdownOptions{
		ListTitle: "Labels",
		ListHints: []string{HintPreserveLinks, "Use action 'label_get'"},
	}, func(label labelOutput) LabelMarkdown {
		return LabelMarkdown{Name: label.Name, Color: label.Color, OpenIssuesCount: label.OpenIssuesCount, ClosedIssuesCount: label.ClosedIssuesCount, OpenMergeRequestsCount: label.OpenMergeRequestsCount, IsProjectLabel: label.IsProjectLabel}
	})

	want := "## Labels (2)\n\n" +
		"| Name | Color | Scope | Open Issues | Closed Issues | Open MRs |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| bug&#124;fix | #ff0000 | project | 3 | 2 | 1 |\n" +
		"| inherited | #00ff00 | group | 0 | 0 | 0 |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		hintsSection("Use action 'label_get'")
	if got != want {
		t.Errorf("label list:\n got %q\nwant %q", got, want)
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

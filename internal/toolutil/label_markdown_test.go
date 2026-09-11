package toolutil

import (
	"strings"
	"testing"
)

// TestFormatLabelMarkdown_AllBranches verifies label detail Markdown renders
// optional fields, counters, escaped descriptions, and hints.
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
		DetailTitle:       "Label",
		DetailHints:       []string{"Use action 'label_update'"},
		EscapeDescription: true,
	})

	for _, want := range []string{
		"## Label: bug|fix",
		"- **ID**: 42",
		"- **Color**: #ff0000",
		"- **Description**: one&#124;two",
		"- **Priority**: 7",
		"- **Project label**: true",
		"- **Subscribed**: true",
		"- **Issues**: 3 open, 2 closed",
		"- **Open MRs**: 1",
		"Use action 'label_update'",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Fatalf("FormatLabelMarkdown() missing %q in:\n%s", want, got)
			}
		})
	}
}

// TestFormatLabelMarkdown_ZeroPrioritySpecified verifies GitLab priority 0 is
// rendered when the API explicitly returns it.
func TestFormatLabelMarkdown_ZeroPrioritySpecified(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{ID: 1, Name: "zero", Color: "#111111", PrioritySpecified: true}, LabelMarkdownOptions{DetailTitle: "Label"})

	if !strings.Contains(got, "- **Priority**: 0") {
		t.Fatalf("FormatLabelMarkdown() did not render explicit zero priority:\n%s", got)
	}
}

// TestFormatLabelMarkdown_MinimalUnescaped verifies optional fields are omitted
// and descriptions can be left unchanged for callers that already handle them.
func TestFormatLabelMarkdown_MinimalUnescaped(t *testing.T) {
	got := FormatLabelMarkdown(LabelMarkdown{
		Name:        "docs",
		Color:       "#00ff00",
		Description: "plain|pipe",
	}, LabelMarkdownOptions{DetailTitle: "Group Label"})

	for _, unwanted := range []string{"- **Priority**", "- **Issues**", "- **Open MRs**", "&#124;"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(got, unwanted) {
				t.Fatalf("FormatLabelMarkdown() unexpectedly contains %q in:\n%s", unwanted, got)
			}
		})
	}
	if !strings.Contains(got, "- **Description**: plain|pipe") {
		t.Fatalf("FormatLabelMarkdown() did not preserve unescaped description:\n%s", got)
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
	for _, tc := range []struct {
		name  string
		label LabelMarkdown
		want  []string
	}{
		{
			name:  "open issues only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", OpenIssuesCount: 4},
			want:  []string{"- **Issues**: 4 open, 0 closed", "- **Open MRs**: 0"},
		},
		{
			name:  "closed issues only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", ClosedIssuesCount: 9},
			want:  []string{"- **Issues**: 0 open, 9 closed", "- **Open MRs**: 0"},
		},
		{
			name:  "open merge requests only",
			label: LabelMarkdown{Name: "bug", Color: "#ff0000", OpenMergeRequestsCount: 2},
			want:  []string{"- **Issues**: 0 open, 0 closed", "- **Open MRs**: 2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLabelMarkdown(tc.label, LabelMarkdownOptions{DetailTitle: "Label"})

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("FormatLabelMarkdown() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

// TestFormatLabelListMarkdownFunc_WithLabels verifies list Markdown maps domain
// labels, renders rows, pagination, escaped table cells, and hints.
func TestFormatLabelListMarkdownFunc_WithLabels(t *testing.T) {
	type labelOutput struct {
		Name                   string
		Color                  string
		OpenIssuesCount        int64
		ClosedIssuesCount      int64
		OpenMergeRequestsCount int64
	}
	got := FormatLabelListMarkdownFunc([]labelOutput{{
		Name:                   "bug|fix",
		Color:                  "#ff0000",
		OpenIssuesCount:        3,
		ClosedIssuesCount:      2,
		OpenMergeRequestsCount: 1,
	}}, PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1}, LabelMarkdownOptions{
		ListTitle: "Labels",
		ListHints: []string{"Use action 'label_get'"},
	}, func(label labelOutput) LabelMarkdown {
		return LabelMarkdown{Name: label.Name, Color: label.Color, OpenIssuesCount: label.OpenIssuesCount, ClosedIssuesCount: label.ClosedIssuesCount, OpenMergeRequestsCount: label.OpenMergeRequestsCount}
	})

	for _, want := range []string{
		"## Labels (1)",
		"| Name | Color | Open Issues | Closed Issues | Open MRs |",
		"| bug&#124;fix | #ff0000 | 3 | 2 | 1 |",
		"Page 1 of 1",
		"Use action 'label_get'",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Fatalf("FormatLabelListMarkdown() missing %q in:\n%s", want, got)
			}
		})
	}
}

// TestFormatLabelListMarkdown_Empty verifies empty lists return the configured
// empty text and keep follow-up hints without rendering a table.
func TestFormatLabelListMarkdown_Empty(t *testing.T) {
	got := FormatLabelListMarkdown(nil, PaginationOutput{}, LabelMarkdownOptions{
		ListTitle:     "Group Labels",
		EmptyListText: "No group labels found.",
		ListHints:     []string{"Use action 'group_label_create'"},
	})

	if !strings.Contains(got, "No group labels found.\n") || !strings.Contains(got, "Use action 'group_label_create'") {
		t.Fatalf("FormatLabelListMarkdown() = %q", got)
	}
}

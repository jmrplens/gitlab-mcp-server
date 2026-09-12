package toolutil

import (
	"testing"
)

// TestFormatTemplateAttributeListMarkdown_Cases_WholeOutput verifies the
// template list in both its shapes byte for byte: the three-column table
// when an attribute header is named, the two-column table when it is not,
// the heading counting what was shown, the footer when there is one, and the
// caller's hint without a link hint, since templates carry no link. An empty
// list is the configured message alone.
func TestFormatTemplateAttributeListMarkdown_Cases_WholeOutput(t *testing.T) {
	item := TemplateAttributeListMarkdownItem{Key: "mit", Name: "MIT | License", Attribute: "Yes"}
	cases := []struct {
		name  string
		items []TemplateAttributeListMarkdownItem
		opts  TemplateAttributeListMarkdownOptions
		want  string
	}{
		{
			name:  "three columns",
			items: []TemplateAttributeListMarkdownItem{item},
			opts:  TemplateAttributeListMarkdownOptions{Title: "Templates", EmptyMessage: "No templates found.", AttributeHeader: "Popular", Hints: []string{"Use a get action for details"}},
			want: "## Templates (1)\n\n| Key | Name | Popular |\n| --- | --- | --- |\n| mit | MIT &#124; License | Yes |\n" +
				hintsSection("Use a get action for details"),
		},
		{
			name:  "two columns with a footer",
			items: []TemplateAttributeListMarkdownItem{{Key: "Go", Name: "Go template"}},
			opts: TemplateAttributeListMarkdownOptions{
				Title: "CI YAML Templates", EmptyMessage: "No templates found.\n",
				Pagination: PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1},
				Hints:      []string{HintPreserveLinks, "Use the key to fetch full template content"},
			},
			want: "## CI YAML Templates (1)\n\n| Key | Name |\n| --- | --- |\n| Go | Go template |\n\nPage 1 of 1 | 1 items total | 20 per page\n" +
				hintsSection("Use the key to fetch full template content"),
		},
		{
			name: "empty",
			opts: TemplateAttributeListMarkdownOptions{Title: "Templates", EmptyMessage: "No templates found.", AttributeHeader: "Popular"},
			want: "No templates found.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTemplateAttributeListMarkdown(tc.items, tc.opts); got != tc.want {
				t.Errorf("template list:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestFormatTemplateDetailMarkdown verifies the template card with every
// optional row present, byte for byte, and the heading alone for a template
// that carries nothing else.
func TestFormatTemplateDetailMarkdown(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:          "Project Template: MIT",
		Key:            "mit",
		Nickname:       "MIT",
		Popular:        true,
		Description:    "A permissive license",
		Permissions:    []string{"commercial-use", "a|b"},
		Conditions:     []string{"include-copyright"},
		Limitations:    []string{"no-liability"},
		Content:        "license text",
		ContentHeading: "Content",
		Hints:          []string{"Use this template"},
	})

	want := "## Project Template: MIT\n\n" +
		"- **Key**: mit\n" +
		"- **Nickname**: MIT\n" +
		"- **Popular**: " + EmojiSuccess + "\n" +
		"- **Description**: A permissive license\n" +
		"- **Permissions**: commercial-use, a&#124;b\n" +
		"- **Conditions**: include-copyright\n" +
		"- **Limitations**: no-liability\n" +
		"\n### Content\n\n```\nlicense text\n```\n" +
		hintsSection("Use this template")
	if md != want {
		t.Errorf("template card:\n got %q\nwant %q", md, want)
	}

	minimal := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "License: Minimal"})
	if wantMinimal := "## License: Minimal\n\n"; minimal != wantMinimal {
		t.Errorf("minimal template card = %q, want %q", minimal, wantMinimal)
	}
}

// TestFormatTemplateDetailMarkdown_KeyLineFollowsTheKey verifies the key row
// is written exactly when the template has a key, and never as an empty label.
//
// The same renderer serves license templates, which have a key, and the issue
// and merge-request description templates, which have only a name. A "- **Key**"
// line with nothing after it would read to a model as a template whose key is
// the empty string rather than one that has none.
func TestFormatTemplateDetailMarkdown_KeyLineFollowsTheKey(t *testing.T) {
	withKey := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "Project Template: MIT", Key: "mit"})
	if want := "## Project Template: MIT\n\n- **Key**: mit\n"; withKey != want {
		t.Errorf("template card with a key:\n got %q\nwant %q", withKey, want)
	}

	withoutKey := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "Issue Template: Bug report"})
	if want := "## Issue Template: Bug report\n\n"; withoutKey != want {
		t.Errorf("template card without a key:\n got %q\nwant %q", withoutKey, want)
	}
}

// TestFormatTemplateDetailMarkdown_LicenseShape_RendersRowsNotParagraphs
// verifies the license family's shape, description and three lists without
// a key, renders as card rows: the bullet-less label lines it used to write
// rendered as one run-on paragraph.
func TestFormatTemplateDetailMarkdown_LicenseShape_RendersRowsNotParagraphs(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:       "License: MIT",
		Description: "A permissive license",
		Permissions: []string{"commercial-use"},
		Conditions:  []string{"include-copyright"},
		Limitations: []string{"no-liability"},
	})

	want := "## License: MIT\n\n" +
		"- **Description**: A permissive license\n" +
		"- **Permissions**: commercial-use\n" +
		"- **Conditions**: include-copyright\n" +
		"- **Limitations**: no-liability\n"
	if md != want {
		t.Errorf("license card:\n got %q\nwant %q", md, want)
	}
}

// TestFormatTemplateDetailMarkdown_MultiLineDescription_QuotesUnderTheLabel
// verifies a description of several paragraphs is quoted under its label,
// indented into the item, with the empty line kept as a bare quote marker.
func TestFormatTemplateDetailMarkdown_MultiLineDescription_QuotesUnderTheLabel(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:       "License: MIT",
		Description: "para one\n\npara two",
	})

	want := "## License: MIT\n\n" +
		"- **Description**:\n" +
		"  > para one\n" +
		"  >\n" +
		"  > para two\n"
	if md != want {
		t.Errorf("template card with a multi-line description:\n got %q\nwant %q", md, want)
	}
}

// TestFormatTemplateDetailMarkdown_ContentWithoutHeading verifies the content
// is fenced after a blank line, under no heading, when the caller named none.
func TestFormatTemplateDetailMarkdown_ContentWithoutHeading(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:   "License: MIT",
		Key:     "mit",
		Content: "permission text",
	})

	if want := "## License: MIT\n\n- **Key**: mit\n\n```\npermission text\n```\n"; md != want {
		t.Errorf("template card with unheaded content:\n got %q\nwant %q", md, want)
	}
}

package toolutil

import (
	"strings"
	"testing"
)

// TestFormatTemplateAttributeListMarkdown verifies shared template list rendering for
// populated and empty collections, including hints and escaped table values.
func TestFormatTemplateAttributeListMarkdown(t *testing.T) {
	md := FormatTemplateAttributeListMarkdown([]TemplateAttributeListMarkdownItem{{Key: "mit", Name: "MIT | License", Attribute: "Yes"}}, TemplateAttributeListMarkdownOptions{
		Title:           "Templates",
		EmptyMessage:    "No templates found.",
		AttributeHeader: "Popular",
		Hints:           []string{"Use a get action for details"},
	})

	for _, want := range []string{"## Templates", "mit", "MIT &#124; License", "Yes", "Use a get action"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Fatalf("markdown missing %q:\n%s", want, md)
			}
		})
	}

	empty := FormatTemplateAttributeListMarkdown(nil, TemplateAttributeListMarkdownOptions{Title: "Templates", EmptyMessage: "No templates found.", AttributeHeader: "Popular"})
	if !strings.Contains(empty, "No templates found.") {
		t.Fatalf("empty markdown missing message:\n%s", empty)
	}
}

// TestFormatTemplateDetailMarkdown verifies optional template detail fields are
// included only when present while preserving code block content.
func TestFormatTemplateDetailMarkdown(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:          "Project Template: MIT",
		Key:            "mit",
		Nickname:       "MIT",
		Popular:        true,
		Description:    "A permissive license",
		Permissions:    []string{"commercial-use"},
		Conditions:     []string{"include-copyright"},
		Limitations:    []string{"no-liability"},
		Content:        "license text",
		ContentHeading: "Content",
		Hints:          []string{"Use this template"},
	})

	for _, want := range []string{"Project Template: MIT", "mit", "Nickname", "Popular", "A permissive license", "commercial-use", "include-copyright", "no-liability", "```\nlicense text\n```", "Use this template"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Fatalf("markdown missing %q:\n%s", want, md)
			}
		})
	}

	minimal := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "License: Minimal"})
	if strings.Contains(minimal, "Description") || strings.Contains(minimal, "```") {
		t.Fatalf("minimal markdown contains absent optional fields:\n%s", minimal)
	}
}

// TestFormatTemplateDetailMarkdown_KeyLineFollowsTheKey verifies the key line
// is written exactly when the template has a key, and never as an empty label.
//
// The same renderer serves license templates, which have a key, and the issue
// and merge-request description templates, which have only a name. A "- **Key**"
// line with nothing after it would read to a model as a template whose key is
// the empty string rather than one that has none.
func TestFormatTemplateDetailMarkdown_KeyLineFollowsTheKey(t *testing.T) {
	withKey := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "Project Template: MIT", Key: "mit"})
	if !strings.Contains(withKey, "- **Key**: mit\n") {
		t.Errorf("a template with a key did not render it:\n%s", withKey)
	}

	withoutKey := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "Issue Template: Bug report"})
	if strings.Contains(withoutKey, "**Key**") {
		t.Errorf("a template without a key rendered an empty key line:\n%s", withoutKey)
	}
}

// TestFormatTemplateDetailMarkdown_PlainFields verifies license-style template
// detail rendering can preserve unbulleted field labels while sharing the
// common renderer.
func TestFormatTemplateDetailMarkdown_PlainFields(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:       "License: MIT",
		Description: "A permissive license",
		Permissions: []string{"commercial-use"},
		Conditions:  []string{"include-copyright"},
		Limitations: []string{"no-liability"},
		PlainFields: true,
	})

	for _, want := range []string{"**Description**: A permissive license", "**Permissions**: commercial-use", "**Conditions**: include-copyright", "**Limitations**: no-liability"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Fatalf("markdown missing %q:\n%s", want, md)
			}
		})
	}
	if strings.Contains(md, "- **Permissions**") {
		t.Fatalf("plain markdown should not render bulleted detail fields:\n%s", md)
	}
}

// TestFormatTemplateDetailMarkdown_ContentWithoutHeading verifies the shared
// template detail renderer still fences the content block in a code fence
// when the caller did not supply a ContentHeading.
func TestFormatTemplateDetailMarkdown_ContentWithoutHeading(t *testing.T) {
	md := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{
		Title:   "License: MIT",
		Key:     "mit",
		Content: "permission text",
	})

	if !strings.Contains(md, "```\npermission text\n```") {
		t.Fatalf("expected unfenced-heading content block in:\n%s", md)
	}
	if strings.Contains(md, "###") {
		t.Fatalf("expected no content heading section, got:\n%s", md)
	}
}

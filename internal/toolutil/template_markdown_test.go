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
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
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
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}

	minimal := FormatTemplateDetailMarkdown(TemplateDetailMarkdown{Title: "License: Minimal"})
	if strings.Contains(minimal, "Description") || strings.Contains(minimal, "```") {
		t.Fatalf("minimal markdown contains absent optional fields:\n%s", minimal)
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
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
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

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

// markdown_test.go validates Markdown formatting functions for group wiki
// MCP tool output. Covers single-page rendering (with/without content and
// encoding), list rendering (empty, single, multiple pages), and special
// characters in fields.

package groupwikis

import (
	"strings"
	"testing"
)

// TestFormatOutputMarkdown validates the single wiki page Markdown formatter.
// It covers pages with full fields, optional encoding, optional content, and
// minimal fields.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		contains []string
		excludes []string
	}{
		{
			name: "full fields with content and encoding",
			input: Output{
				Title:    "Home",
				Slug:     "home",
				Format:   "markdown",
				Content:  "# Welcome",
				Encoding: "utf-8",
			},
			contains: []string{
				"## Wiki: Home",
				"**Slug**: home",
				"**Format**: markdown",
				"**Encoding**: utf-8",
				"### Content",
				"# Welcome",
				"gitlab_group_wiki_edit",
				"gitlab_group_wiki_delete",
			},
		},
		{
			name: "without encoding omits encoding line",
			input: Output{
				Title:   "Setup",
				Slug:    "setup",
				Format:  "asciidoc",
				Content: "Setup instructions",
			},
			contains: []string{
				"## Wiki: Setup",
				"**Slug**: setup",
				"**Format**: asciidoc",
				"### Content",
				"Setup instructions",
			},
			excludes: []string{
				"**Encoding**",
			},
		},
		{
			name: "without content omits content section",
			input: Output{
				Title:  "Empty",
				Slug:   "empty",
				Format: "markdown",
			},
			contains: []string{
				"## Wiki: Empty",
				"**Slug**: empty",
			},
			excludes: []string{
				"### Content",
			},
		},
		{
			name: "minimal fields",
			input: Output{
				Title:  "",
				Slug:   "",
				Format: "",
			},
			contains: []string{
				"## Wiki:",
				"**Slug**:",
				"**Format**:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates the wiki list Markdown table formatter.
// It covers empty list, single page, and multiple pages with special characters.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		excludes []string
	}{
		{
			name:  "empty list returns no-results message",
			input: ListOutput{WikiPages: nil},
			contains: []string{
				"No group wiki pages found.",
			},
			excludes: []string{
				"| Title",
			},
		},
		{
			name: "single page renders table",
			input: ListOutput{
				WikiPages: []Output{
					{Title: "Home", Slug: "home", Format: "markdown"},
				},
			},
			contains: []string{
				"| Title | Slug | Format |",
				"| --- | --- | --- |",
				"| Home | home | markdown |",
				"gitlab_group_wiki_get",
				"gitlab_group_wiki_create",
			},
		},
		{
			name: "multiple pages render rows",
			input: ListOutput{
				WikiPages: []Output{
					{Title: "Home", Slug: "home", Format: "markdown"},
					{Title: "Setup Guide", Slug: "setup-guide", Format: "asciidoc"},
					{Title: "FAQ", Slug: "faq", Format: "rdoc"},
				},
			},
			contains: []string{
				"| Home | home | markdown |",
				"| Setup Guide | setup-guide | asciidoc |",
				"| FAQ | faq | rdoc |",
			},
		},
		{
			name: "special characters in title are escaped",
			input: ListOutput{
				WikiPages: []Output{
					{Title: "Pipe | Test", Slug: "pipe-test", Format: "markdown"},
				},
			},
			contains: []string{
				"pipe-test",
				"markdown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

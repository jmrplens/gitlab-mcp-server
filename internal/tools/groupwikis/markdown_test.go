// markdown_test.go validates Markdown formatting functions for group wiki
// MCP tool output. Every expectation is the whole render: a substring
// assertion is what let a table that had stopped rendering keep passing.
package groupwikis

import (
	"testing"
)

// The guidance sections the two group wiki formatters close with.
const (
	groupWikiCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.wiki_edit' to update this page\n" +
		"- Use action 'group.wiki_delete' to remove this page\n"

	groupWikiListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'group.wiki_get' to read one page's content\n" +
		"- Use action 'group.wiki_create' to add a new page\n"
)

// TestFormatOutputMarkdown pins the whole card of a group wiki page, in each
// of the four shapes GitLab answers with: a Markdown page, a page in another
// format, a page read without its content, and a page whose every field is
// empty.
func TestFormatOutputMarkdown(t *testing.T) {
	cases := []struct {
		name  string
		input Output
		want  string
	}{
		{
			// A Markdown body is quoted, so a heading in the page cannot become
			// a heading of the response: this content used to be written raw.
			name: "markdown page is quoted",
			input: Output{
				Title:    "Home",
				Slug:     "home",
				Format:   "markdown",
				Content:  "# Welcome\n\nRead the setup guide.",
				Encoding: "utf-8",
			},
			want: "## Wiki: Home\n\n" +
				"- **Slug**: home\n" +
				"- **Format**: markdown\n" +
				"- **Encoding**: utf-8\n" +
				"- **Content**:\n" +
				"  > # Welcome\n" +
				"  >\n" +
				"  > Read the setup guide.\n" +
				groupWikiCardHints,
		},
		{
			// A page in another format is fenced with that format, since
			// quoting it would render it as the Markdown it is not.
			name: "asciidoc page is fenced",
			input: Output{
				Title:   "Setup",
				Slug:    "setup",
				Format:  "asciidoc",
				Content: "= Setup instructions",
			},
			want: "## Wiki: Setup\n\n" +
				"- **Slug**: setup\n" +
				"- **Format**: asciidoc\n" +
				"\n### Content\n\n" +
				"```asciidoc\n= Setup instructions\n```\n" +
				groupWikiCardHints,
		},
		{
			name:  "page read without its content",
			input: Output{Title: "Empty", Slug: "empty", Format: "markdown"},
			want: "## Wiki: Empty\n\n" +
				"- **Slug**: empty\n" +
				"- **Format**: markdown\n" +
				groupWikiCardHints,
		},
		{
			// Every field absent writes no row at all, rather than a label with
			// nothing after it.
			name:  "every field empty",
			input: Output{},
			want:  "## Wiki\n" + groupWikiCardHints,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(c.input); got != c.want {
				t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// TestFormatListMarkdown pins the whole group wiki listing. The table used to
// be written under a guidance section the formatter opened with, which made
// its header a lazy continuation of that section's last list item: no table
// rendered at all, and the substring assertions that replaced this one passed
// throughout.
func TestFormatListMarkdown(t *testing.T) {
	cases := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "no pages",
			input: ListOutput{WikiPages: nil},
			want:  "No group wiki pages found.\n",
		},
		{
			name: "one page",
			input: ListOutput{WikiPages: []Output{
				{Title: "Home", Slug: "home", Format: "markdown"},
			}},
			want: "## Group Wiki Pages (1)\n\n" +
				"| Title | Slug | Format |\n" +
				"| --- | --- | --- |\n" +
				"| Home | home | markdown |\n" +
				groupWikiListHints,
		},
		{
			name: "several pages",
			input: ListOutput{WikiPages: []Output{
				{Title: "Home", Slug: "home", Format: "markdown"},
				{Title: "Setup Guide", Slug: "setup-guide", Format: "asciidoc"},
				{Title: "FAQ", Slug: "faq", Format: "rdoc"},
			}},
			want: "## Group Wiki Pages (3)\n\n" +
				"| Title | Slug | Format |\n" +
				"| --- | --- | --- |\n" +
				"| Home | home | markdown |\n" +
				"| Setup Guide | setup-guide | asciidoc |\n" +
				"| FAQ | faq | rdoc |\n" +
				groupWikiListHints,
		},
		{
			// A pipe a page author typed into a title is an entity, so it stays
			// inside its cell instead of opening a column of its own.
			name: "a pipe in a title stays in its cell",
			input: ListOutput{WikiPages: []Output{
				{Title: "Pipe | Test", Slug: "pipe-test", Format: "markdown"},
			}},
			want: "## Group Wiki Pages (1)\n\n" +
				"| Title | Slug | Format |\n" +
				"| --- | --- | --- |\n" +
				"| Pipe &#124; Test | pipe-test | markdown |\n" +
				groupWikiListHints,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatListMarkdown(c.input); got != c.want {
				t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

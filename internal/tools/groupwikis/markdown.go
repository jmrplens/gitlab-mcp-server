// markdown.go provides Markdown formatting for group wiki MCP tool output.

package groupwikis

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group wiki page as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Wiki: %s\n\n", toolutil.EscapeMdHeading(out.Title))
	fmt.Fprintf(&b, "- **Slug**: %s\n", out.Slug)
	fmt.Fprintf(&b, "- **Format**: %s\n", out.Format)
	if out.Encoding != "" {
		fmt.Fprintf(&b, "- **Encoding**: %s\n", out.Encoding)
	}
	if out.Content != "" {
		fmt.Fprintf(&b, "\n### Content\n\n%s\n", out.Content)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_wiki_edit to update this page",
		"Use gitlab_group_wiki_delete to remove this page",
	)
	return b.String()
}

// FormatListMarkdown renders a list of group wiki pages as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.WikiPages) == 0 {
		return "No group wiki pages found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	b.WriteString("| Title | Slug | Format |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, w := range out.WikiPages {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(w.Title),
			toolutil.EscapeMdTableCell(w.Slug),
			w.Format,
		)
	}
	toolutil.WriteHints(&b,
		"Use gitlab_group_wiki_get with a slug to read page content",
		"Use gitlab_group_wiki_create to add a new page",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

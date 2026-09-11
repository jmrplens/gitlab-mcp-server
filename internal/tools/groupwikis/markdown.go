package groupwikis

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name that the action specs do not already
// spell. Group wiki actions are routes on the gitlab_group catalog group, so
// their IDs carry the group domain.
//
// The two are declared one per line with a directive each because gosec reads
// a constant whose name ends in a verb over a dotted value as a credential;
// the sibling IDs in action_specs.go are excused by path in .golangci.yml for
// the same reason.
const (
	//nolint:gosec // G101 false positive: a canonical catalog action ID, never a credential.
	actionGroupWikiCreate = "group.wiki_create"
	//nolint:gosec // G101 false positive: a canonical catalog action ID, never a credential.
	actionGroupWikiDelete = "group.wiki_delete"
)

// FormatOutputMarkdown renders one group wiki page as a card: the page's own
// fields, then its body under a label of its own.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, wikiHeading(out.Title))
	c.Field("Slug", out.Slug)
	c.Field("Format", out.Format)
	c.Field("Encoding", out.Encoding)
	c.Count("Page Metadata ID", out.WikiPageMetaID)
	writeWikiContent(c, out.Format, out.Content)
	c.End(
		toolutil.HintAction(actionGroupWikiEdit, "update this page"),
		toolutil.HintAction(actionGroupWikiDelete, "remove this page"),
	)
	return b.String()
}

// wikiHeading names the card: the page's title, and the bare word when GitLab
// sent none, so the heading never ends in a colon with nothing after it.
func wikiHeading(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Wiki"
	}
	return "Wiki: " + title
}

// writeWikiContent writes the page body under its own label. A Markdown page
// is quoted, so nothing a page author wrote can add a heading, a row or a
// guidance section to the card; a page in any other format is fenced with that
// format as the info string, since quoting rdoc or asciidoc would render it as
// the Markdown it is not. The body used to be written raw, which let a page
// forge the whole response.
func writeWikiContent(c *toolutil.Card, format, content string) {
	if content == "" {
		return
	}
	if format == "" || strings.EqualFold(format, "markdown") {
		c.Text("Content", content)
		return
	}
	c.Fence("Content", format, content)
}

// FormatListMarkdown renders a page of group wiki pages as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.WikiPages) == 0 {
		return toolutil.EmptyMessage("group wiki pages")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Wiki Pages", len(out.WikiPages), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Title", "Slug", "Format"))
	for _, w := range out.WikiPages {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(w.Title),
			toolutil.EscapeMdTableCell(w.Slug),
			toolutil.EscapeMdTableCell(w.Format),
		))
	}
	// The table carries no link, so the hints carry no instruction to preserve
	// one: the leading HintPreserveLinks this list used to open with put a
	// guidance section above its own table header, which stopped the table
	// rendering at all.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionGroupWikiGet, "read one page's content"),
		toolutil.HintAction(actionGroupWikiCreate, "add a new page"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

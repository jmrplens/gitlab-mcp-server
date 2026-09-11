package wikis

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name that the action specs do not already
// spell, the one form every surface resolves.
const (
	actionWikiCreate = "wiki.create"
	actionWikiDelete = "wiki.delete"
)

type wikiNotFoundOutput struct {
	Identifier string
}

func formatWikiNotFound(out wikiNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Wiki Page", out.Identifier,
		"Use gitlab_wiki_list with project_id to list wiki pages",
		"Wiki slugs are case-sensitive and may differ from the page title",
	)
}

// FormatOutputMarkdownString renders one wiki page as a card: the page's own
// fields, then its body under a label of its own.
func FormatOutputMarkdownString(w Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, wikiHeading(w.Title))
	c.Field("Slug", w.Slug)
	c.Field("Format", w.Format)
	c.Field("Encoding", w.Encoding)
	c.Count("Page Metadata ID", w.WikiPageMetaID)
	writeWikiContent(c, w.Format, w.Content)
	c.End(
		toolutil.HintAction(actionWikiUpdate, "edit this wiki page"),
		toolutil.HintAction(actionWikiDelete, "remove this wiki page"),
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
// the Markdown it is not.
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

// FormatOutputMarkdown returns an MCP tool result for a single wiki page.
func FormatOutputMarkdown(w Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(w))
}

// FormatListMarkdownString renders a page of wiki pages as a Markdown table.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.WikiPages) == 0 {
		return toolutil.EmptyMessage("wiki pages")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Wiki Pages", len(out.WikiPages), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Title", "Slug", "Format"))
	for _, w := range out.WikiPages {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(w.Title),
			toolutil.EscapeMdTableCell(w.Slug),
			toolutil.EscapeMdTableCell(w.Format),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionWikiGet, "read one wiki page"),
		toolutil.HintAction(actionWikiCreate, "add a new wiki page"),
	)
	return b.String()
}

// FormatListMarkdown returns an MCP tool result for a wiki page list.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatAttachmentMarkdownString renders a wiki attachment upload as the card
// of the object it created.
func FormatAttachmentMarkdownString(o AttachmentOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Wiki Attachment Uploaded")
	c.Field("File Name", o.FileName)
	c.Field("File Path", o.FilePath)
	c.Field("Branch", o.Branch)
	c.URL(o.URL)
	// GitLab builds this snippet around the file name whoever uploaded it chose,
	// and it is meant to be copied into a page verbatim, so it is a code span.
	c.Code("Markdown", o.Markdown)
	c.End(
		toolutil.HintAction(actionWikiGet, "view the wiki page where this attachment is used"),
		toolutil.HintAction(actionWikiList, "see all wiki pages"),
	)
	return b.String()
}

// FormatAttachmentMarkdown returns an MCP tool result for a wiki attachment upload.
func FormatAttachmentMarkdown(o AttachmentOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatAttachmentMarkdownString(o))
}

func init() {
	toolutil.RegisterMarkdownResult(formatWikiNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatAttachmentMarkdownString)
}

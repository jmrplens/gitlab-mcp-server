package wikis

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatOutputMarkdownString formats a single wiki page as Markdown.
func FormatOutputMarkdownString(w Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Wiki: %s\n\n", toolutil.EscapeMdHeading(w.Title))
	fmt.Fprintf(&b, "- **Slug**: %s\n", w.Slug)
	fmt.Fprintf(&b, "- **Format**: %s\n", w.Format)
	if w.Encoding != "" {
		fmt.Fprintf(&b, "- **Encoding**: %s\n", w.Encoding)
	}
	if w.Content != "" {
		fmt.Fprintf(&b, "\n### Content\n\n%s\n", toolutil.WrapGFMBody(w.Content))
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to edit this wiki page",
		"Use action 'delete' to remove this wiki page",
	)
	return b.String()
}

// FormatOutputMarkdown returns an MCP tool result for a single wiki page.
func FormatOutputMarkdown(w Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(w))
}

// FormatListMarkdownString formats a list of wiki pages as a Markdown table.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.WikiPages) == 0 {
		return "No wiki pages found.\n"
	}
	var b strings.Builder
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
		"Use action 'get' with a slug to read a wiki page",
		"Use action 'create' to add a new wiki page",
	)
	return b.String()
}

// FormatListMarkdown returns an MCP tool result for a wiki page list.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatAttachmentMarkdownString renders a wiki attachment upload result as Markdown.
func FormatAttachmentMarkdownString(o AttachmentOutput) string {
	var b strings.Builder
	b.WriteString("## Wiki Attachment Uploaded\n\n")
	fmt.Fprintf(&b, "- **File Name**: %s\n", o.FileName)
	fmt.Fprintf(&b, "- **File Path**: %s\n", o.FilePath)
	if o.Branch != "" {
		fmt.Fprintf(&b, "- **Branch**: %s\n", o.Branch)
	}
	fmt.Fprintf(&b, toolutil.FmtMdURL, o.URL)
	fmt.Fprintf(&b, "- **Markdown**: `%s`\n", o.Markdown)
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'get' to view the wiki page where this attachment is used",
		"Use action 'list' to see all wiki pages",
	)
	return b.String()
}

// FormatAttachmentMarkdown returns an MCP tool result for a wiki attachment upload.
func FormatAttachmentMarkdown(o AttachmentOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatAttachmentMarkdownString(o))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatAttachmentMarkdownString)
}

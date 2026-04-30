// markdown.go provides Markdown formatting functions for issue discussion MCP tool output.
package issuediscussions

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of discussions as Markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders discussions list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Discussions) == 0 {
		return "No issue discussions found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Issue Discussions (%d)\n\n", len(out.Discussions))
	toolutil.WriteListSummary(&b, len(out.Discussions), out.Pagination)
	for _, d := range out.Discussions {
		fmt.Fprintf(&b, "### Discussion %s\n", d.ID)
		for _, n := range d.Notes {
			fmt.Fprintf(&b, "- **@%s** (%s): %s\n", n.Author, toolutil.FormatTime(n.CreatedAt), toolutil.NormalizeText(n.Body))
		}
		b.WriteString("\n")
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b,
		"Use action 'discussion_get' with discussion_id to see full discussion",
		"Use action 'discussion_add_note' to reply to a discussion",
	)
	return b.String()
}

// FormatMarkdown formats a single discussion as Markdown.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a discussion as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Discussion %s\n\n", out.ID)
	for _, n := range out.Notes {
		fmt.Fprintf(&b, "- **@%s** (%s): %s\n", n.Author, toolutil.FormatTime(n.CreatedAt), n.Body)
	}
	toolutil.WriteHints(&b,
		"Use action 'discussion_add_note' to reply to this discussion",
		"Use action 'discussion_update_note' with note_id to edit a note",
	)
	return b.String()
}

// FormatNoteMarkdown formats a single note as Markdown.
func FormatNoteMarkdown(out NoteOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatNoteMarkdownString(out))
}

// FormatNoteMarkdownString renders a note as Markdown.
func FormatNoteMarkdownString(out NoteOutput) string {
	var b strings.Builder
	b.WriteString("## Note\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdAuthorAt, out.Author)
	fmt.Fprintf(&b, "- **Body**: %s\n", out.Body)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'discussion_update_note' with note_id to edit this note",
		"Use action 'discussion_delete_note' with note_id to remove this note",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatNoteMarkdownString)
}

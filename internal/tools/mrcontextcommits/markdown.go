package mrcontextcommits

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders the context commits attached to a merge request as
// a Markdown table: a collection of objects that share columns.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders the context commit list as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Commits) == 0 {
		return toolutil.EmptyMessage("context commits")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "MR Context Commits", len(out.Commits), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("SHA", "Title", "Author", "Created"))
	linked := false
	for _, c := range out.Commits {
		short := c.ShortID
		if short == "" {
			short = c.ID
		}
		sha := toolutil.MdCodeSpanCell(short)
		if c.WebURL != "" {
			sha = toolutil.MdTitleLink(short, c.WebURL)
			linked = true
		}
		sb.WriteString(toolutil.MarkdownTableRow(
			sha,
			toolutil.EscapeMdTableCell(c.Title),
			toolutil.EscapeMdTableCell(c.AuthorName),
			toolutil.FormatTime(c.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, linked,
		toolutil.HintAction(actionCommitGet, "read one of these commits in full"),
		toolutil.HintAction(actionContextCommitsCreate, "pin another commit to this review"),
		toolutil.HintAction(actionContextCommitsDelete, "unpin one of these commits"),
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
}

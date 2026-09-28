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
	return commitsMarkdown(out.Commits, out.Pagination)
}

// FormatCreateMarkdown renders the commits create_context_commits pinned as
// the same table the list is rendered as.
func FormatCreateMarkdown(out CreateOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatCreateMarkdownString(out))
}

// FormatCreateMarkdownString renders the pinned commits as Markdown. The
// answer is every commit pinned rather than a page, so the table carries no
// page and the heading counts what is shown.
func FormatCreateMarkdownString(out CreateOutput) string {
	return commitsMarkdown(out.Commits, toolutil.PaginationOutput{})
}

// commitsMarkdown renders context commits as a table, placed in the whole
// list by pagination when they are a page of it.
func commitsMarkdown(commits []CommitItem, pagination toolutil.PaginationOutput) string {
	if len(commits) == 0 {
		return toolutil.EmptyMessage("context commits")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "MR Context Commits", len(commits), pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("SHA", "Title", "Author", "Created"))
	linked := false
	for _, c := range commits {
		short := c.ShortID
		if short == "" {
			short = c.ID
		}
		sha := toolutil.MdCodeSpanCell(short)
		if c.WebURL != "" {
			sha = toolutil.MdTitleLink(short, c.WebURL)
			linked = true
		}
		author := toolutil.EscapeMdTableCell(c.AuthorName)
		if c.Author != nil && c.Author.WebURL != "" {
			author = toolutil.MdTitleLink(authorLabel(c), c.Author.WebURL)
			linked = true
		}
		sb.WriteString(toolutil.MarkdownTableRow(
			sha,
			toolutil.EscapeMdTableCell(c.Title),
			author,
			toolutil.FormatTime(c.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&sb, pagination, linked,
		toolutil.HintAction(actionCommitGet, "read one of these commits in full"),
		toolutil.HintAction(actionContextCommitsCreate, "pin another commit to this review"),
		toolutil.HintAction(actionContextCommitsDelete, "unpin one of these commits"),
	)
	return sb.String()
}

// authorLabel is the text an author's profile link carries: the name the
// commit was authored under, or, for a commit whose author name is blank, the
// name of the account GitLab matched its email to, so the link is never empty.
func authorLabel(c CommitItem) string {
	if c.AuthorName != "" {
		return c.AuthorName
	}
	return c.Author.Name
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatCreateMarkdown)
}

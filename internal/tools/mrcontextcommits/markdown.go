// markdown.go provides Markdown formatting functions for merge request context commit MCP tool output.

package mrcontextcommits

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats the list of context commits as markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Commits) == 0 {
		return toolutil.ToolResultWithMarkdown("No context commits found for this merge request.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## MR Context Commits (%d)\n\n", len(out.Commits))
	sb.WriteString("| SHA | Title | Author |\n")
	sb.WriteString("|-----|-------|--------|\n")
	for _, c := range out.Commits {
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", c.ShortID, toolutil.EscapeMdTableCell(c.Title), c.AuthorName)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_commit_get` to view full details of a specific commit")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
}

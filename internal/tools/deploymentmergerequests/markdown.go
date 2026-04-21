// markdown.go provides Markdown formatting functions for deployment merge request MCP tool output.

package deploymentmergerequests

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats the list of deployment merge requests as markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.MergeRequests) == 0 {
		return toolutil.ToolResultWithMarkdown("No merge requests found for this deployment.\n")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Deployment Merge Requests (%d)\n\n", len(out.MergeRequests))
	sb.WriteString("| IID | Title | State | Author | Source → Target |\n")
	sb.WriteString("|-----|-------|-------|--------|----------------|\n")
	for _, mr := range out.MergeRequests {
		fmt.Fprintf(&sb, "| !%d | %s | %s | %s | %s → %s |\n",
			mr.IID,
			toolutil.MdTitleLink(mr.Title, mr.WebURL),
			mr.State,
			mr.Author,
			mr.SourceBranch,
			mr.TargetBranch)
	}
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks, "Use `gitlab_mr_get` to view full MR details")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
}

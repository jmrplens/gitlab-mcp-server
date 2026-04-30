// markdown.go provides Markdown formatting functions for runner controller token MCP tool output.
package runnercontrollertokens

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a runner controller token as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Controller Token #%d\n\n", out.ID)
	fmt.Fprintf(&b, "- **Controller ID**: %d\n", out.RunnerControllerID)
	fmt.Fprintf(&b, toolutil.FmtMdDescription, out.Description)
	if out.Token != "" {
		fmt.Fprintf(&b, "- **Token**: %s\n", out.Token)
	}
	if out.LastUsedAt != "" {
		fmt.Fprintf(&b, "- **Last Used At**: %s\n", toolutil.FormatTime(out.LastUsedAt))
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created At**: %s\n", toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(&b, "Store the token value securely — it cannot be retrieved later")
	return b.String()
}

// FormatListMarkdown renders a list of runner controller tokens as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return "No runner controller tokens found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Controller Tokens (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Tokens), out.Pagination)
	b.WriteString("| ID | Controller | Description | Last Used | Created At |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, t := range out.Tokens {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s |\n",
			t.ID, t.RunnerControllerID, toolutil.EscapeMdTableCell(t.Description), t.LastUsedAt, t.CreatedAt)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_runner_controller_token_get` to view details of a specific token")
	return b.String()
}

// FormatGetMarkdown formats Get output as an MCP tool result.
func FormatGetMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

// markdown.go provides Markdown formatting functions for runner controller MCP tool output.
package runnercontrollers

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatOutputMarkdown renders a runner controller as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Controller #%d\n\n", out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdDescription, out.Description)
	fmt.Fprintf(&b, toolutil.FmtMdState, out.State)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created At**: %s\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, "- **Updated At**: %s\n", toolutil.FormatTime(out.UpdatedAt))
	}
	toolutil.WriteHints(&b, "Use `gitlab_runner_controller_token_list` to manage authentication")
	return b.String()
}

// FormatDetailsMarkdown renders detailed runner controller info as Markdown.
func FormatDetailsMarkdown(out DetailsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Controller #%d — Details\n\n", out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdDescription, out.Description)
	fmt.Fprintf(&b, toolutil.FmtMdState, out.State)
	fmt.Fprintf(&b, "- **Connected**: %t\n", out.Connected)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created At**: %s\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.UpdatedAt != "" {
		fmt.Fprintf(&b, "- **Updated At**: %s\n", toolutil.FormatTime(out.UpdatedAt))
	}
	toolutil.WriteHints(&b, "Use `gitlab_runner_controller_scope_list` to view scopes")
	return b.String()
}

// FormatListMarkdown renders a list of runner controllers as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Controllers) == 0 {
		return "No runner controllers found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Controllers (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Controllers), out.Pagination)
	b.WriteString("| ID | Description | State | Created At |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, rc := range out.Controllers {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
			rc.ID, toolutil.EscapeMdTableCell(rc.Description), rc.State, rc.CreatedAt)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use `gitlab_runner_controller_get` to view details of a specific controller")
	return b.String()
}

// FormatGetMarkdown formats Get output as an MCP tool result.
func FormatGetMarkdown(out DetailsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatDetailsMarkdown(out))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDetailsMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

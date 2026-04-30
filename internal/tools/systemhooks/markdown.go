// markdown.go provides Markdown formatting functions for system hook MCP tool output.
package systemhooks

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of system hooks.
func FormatListMarkdown(output ListOutput) *mcp.CallToolResult {
	if len(output.Hooks) == 0 {
		return toolutil.ToolResultWithMarkdown("No system hooks found.\n")
	}
	var sb strings.Builder
	sb.WriteString("## System Hooks\n\n")
	sb.WriteString("| ID | Name | URL | Push | Tag Push | MR | Repo Update | SSL |\n")
	sb.WriteString("|----|------|-----|------|----------|----|-------------|-----|\n")
	for _, h := range output.Hooks {
		fmt.Fprintf(&sb, "| %d | %s | %s | %v | %v | %v | %v | %v |\n",
			h.ID,
			toolutil.EscapeMdTableCell(h.Name),
			toolutil.EscapeMdTableCell(h.URL),
			h.PushEvents,
			h.TagPushEvents,
			h.MergeRequestsEvents,
			h.RepositoryUpdateEvents,
			h.EnableSSLVerification)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_system_hook` to view details of a specific hook")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatHookMarkdown formats a single system hook.
func FormatHookMarkdown(item HookItem) *mcp.CallToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## System Hook #%d\n\n", item.ID)
	sb.WriteString("| Property | Value |\n")
	sb.WriteString("|----------|-------|\n")
	if item.Name != "" {
		fmt.Fprintf(&sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(item.Name))
	}
	if item.Description != "" {
		fmt.Fprintf(&sb, "| Description | %s |\n", toolutil.EscapeMdTableCell(item.Description))
	}
	fmt.Fprintf(&sb, "| URL | %s |\n", toolutil.EscapeMdTableCell(item.URL))
	fmt.Fprintf(&sb, "| Push Events | %v |\n", item.PushEvents)
	fmt.Fprintf(&sb, "| Tag Push Events | %v |\n", item.TagPushEvents)
	fmt.Fprintf(&sb, "| MR Events | %v |\n", item.MergeRequestsEvents)
	fmt.Fprintf(&sb, "| Repo Update Events | %v |\n", item.RepositoryUpdateEvents)
	fmt.Fprintf(&sb, "| SSL Verification | %v |\n", item.EnableSSLVerification)
	if item.CreatedAt != "" {
		fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(item.CreatedAt))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_test_system_hook` to verify this hook is working")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatTestMarkdown formats a hook test event result.
func FormatTestMarkdown(output TestOutput) *mcp.CallToolResult {
	e := output.Event
	var sb strings.Builder
	sb.WriteString("## Hook Test Event\n\n")
	sb.WriteString("| Property | Value |\n")
	sb.WriteString("|----------|-------|\n")
	fmt.Fprintf(&sb, "| Event Name | %s |\n", e.EventName)
	fmt.Fprintf(&sb, "| Name | %s |\n", e.Name)
	fmt.Fprintf(&sb, "| Path | %s |\n", e.Path)
	fmt.Fprintf(&sb, "| Project ID | %d |\n", e.ProjectID)
	fmt.Fprintf(&sb, "| Owner | %s (%s) |\n", e.OwnerName, e.OwnerEmail)
	toolutil.WriteHints(&sb, "Verify the hook is receiving events correctly")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatHookMarkdown)
	toolutil.RegisterMarkdownResult(FormatTestMarkdown)
}

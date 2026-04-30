// markdown.go provides Markdown formatting functions for work item MCP tool output.
package workitems

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatGetMarkdown formats a single work item as markdown.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	wi := out.WorkItem
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Work Item #%d: %s\n\n", wi.IID, wi.Title)
	fmt.Fprintf(&sb, "- **Type**: %s\n", wi.Type)
	fmt.Fprintf(&sb, toolutil.FmtMdState, wi.State)
	if wi.Status != "" {
		fmt.Fprintf(&sb, "- **Status**: %s\n", wi.Status)
	}
	if wi.Author != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdAuthor, wi.Author)
	}
	if len(wi.Assignees) > 0 {
		fmt.Fprintf(&sb, "- **Assignees**: %s\n", strings.Join(wi.Assignees, ", "))
	}
	if len(wi.Labels) > 0 {
		fmt.Fprintf(&sb, "- **Labels**: %s\n", strings.Join(wi.Labels, ", "))
	}
	if wi.WebURL != "" {
		fmt.Fprintf(&sb, "- **URL**: %s\n", wi.WebURL)
	}
	if wi.Description != "" {
		fmt.Fprintf(&sb, "\n### Description\n\n%s\n", wi.Description)
	}
	if len(wi.LinkedItems) > 0 {
		sb.WriteString("\n### Linked Items\n\n")
		sb.WriteString("| IID | Link Type | Path |\n")
		sb.WriteString("|-----|-----------|------|\n")
		for _, li := range wi.LinkedItems {
			fmt.Fprintf(&sb, "| %d | %s | %s |\n", li.IID, li.LinkType, li.Path)
		}
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_work_item` to modify this work item")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatListMarkdown formats a list of work items as markdown.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.WorkItems) == 0 {
		return toolutil.ToolResultWithMarkdown("No work items found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Work Items (%d)\n\n", len(out.WorkItems))
	sb.WriteString("| IID | Type | State | Status | Title | Author |\n")
	sb.WriteString("|-----|------|-------|--------|-------|--------|\n")
	for _, wi := range out.WorkItems {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s | %s |\n",
			wi.IID, wi.Type, wi.State, wi.Status, toolutil.EscapeMdTableCell(wi.Title), wi.Author)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_work_item` to view full details of a specific item")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
}

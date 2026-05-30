package workitems

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// FormatWorkItemTypeListMarkdown formats a list of work item types as a Markdown table.
func FormatWorkItemTypeListMarkdown(out WorkItemTypeListOutput) *mcp.CallToolResult {
	if len(out.Types) == 0 {
		return toolutil.ToolResultWithMarkdown("No work item types found.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Work Item Types (%d)\n\n", len(out.Types))
	sb.WriteString("| Name | ID | Enabled |\n")
	sb.WriteString("|------|----|---------|\n")
	for _, t := range out.Types {
		fmt.Fprintf(&sb, "| %s | `%s` | %v |\n",
			toolutil.EscapeMdTableCell(t.Name), toolutil.EscapeMdTableCell(t.ID), t.Enabled)
	}
	if out.Pagination.HasNextPage {
		fmt.Fprintf(&sb, "\n> Next page cursor: `%s`\n", out.Pagination.EndCursor)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_create_work_item` with work_item_type_id from the ID column to create work items of this type")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatWorkItemTypeListMarkdown)
}

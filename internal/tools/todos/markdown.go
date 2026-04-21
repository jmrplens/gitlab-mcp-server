// markdown.go provides Markdown formatting functions for to-do MCP tool output.

package todos

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatOutputMarkdownString formats a single to-do item as Markdown.
func FormatOutputMarkdownString(t Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## To-Do #%d\n\n", t.ID)
	fmt.Fprintf(&b, "**Action:** %s\n", t.ActionName)
	if t.TargetURL != "" {
		fmt.Fprintf(&b, "**Target:** [%s](%s) (type: %s)\n", t.TargetTitle, t.TargetURL, t.TargetType)
	} else {
		fmt.Fprintf(&b, "**Target:** %s (type: %s)\n", t.TargetTitle, t.TargetType)
	}
	fmt.Fprintf(&b, "**State:** %s\n", t.State)
	fmt.Fprintf(&b, "**Project:** %s\n", t.ProjectName)
	if t.AuthorName != "" {
		fmt.Fprintf(&b, "**Author:** %s (created: %s)\n", t.AuthorName, toolutil.FormatTime(t.CreatedAt))
	}
	if t.Body != "" {
		fmt.Fprintf(&b, "\n---\n\n%s\n", t.Body)
	}
	toolutil.WriteHints(&b,
		"Use action 'todo_mark_done' with this todo ID to mark it as done",
		"Use action 'todo_list' to see all your to-do items",
	)
	return b.String()
}

// FormatOutputMarkdown returns an MCP tool result for a single to-do item.
func FormatOutputMarkdown(t Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(t))
}

// FormatListMarkdownString formats a list of to-do items as a Markdown table.
func FormatListMarkdownString(v ListOutput) string {
	if len(v.Todos) == 0 {
		return "No to-do items found.\n"
	}
	var b strings.Builder
	b.WriteString("| ID | Action | Target | Type | State | Project |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, t := range v.Todos {
		target := toolutil.EscapeMdTableCell(t.TargetTitle)
		if t.TargetURL != "" {
			target = fmt.Sprintf("[%s](%s)", target, t.TargetURL)
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
			t.ID,
			toolutil.EscapeMdTableCell(t.ActionName),
			target,
			toolutil.EscapeMdTableCell(t.TargetType),
			toolutil.EscapeMdTableCell(t.State),
			toolutil.EscapeMdTableCell(t.ProjectName),
		)
	}
	b.WriteString(toolutil.FormatPagination(v.Pagination))
	toolutil.WriteHints(&b,
		toolutil.HintPreserveLinks,
		"Use action 'todo_mark_done' with a todo ID to mark it as done",
		"Use action 'todo_mark_all_done' to clear all to-do items",
	)
	return b.String()
}

// FormatListMarkdown returns an MCP tool result for a to-do list.
func FormatListMarkdown(v ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(v))
}

// FormatMarkDoneMarkdownString formats a mark-done result as Markdown.
func FormatMarkDoneMarkdownString(v MarkDoneOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", toolutil.EmojiSuccess, v.Message)
	toolutil.WriteHints(&b, "Use action 'todo_list' to see remaining to-do items")
	return b.String()
}

// FormatMarkDoneMarkdown returns an MCP tool result for marking a to-do as done.
func FormatMarkDoneMarkdown(v MarkDoneOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkDoneMarkdownString(v))
}

// FormatMarkAllDoneMarkdownString formats a mark-all-done result as Markdown.
func FormatMarkAllDoneMarkdownString(v MarkAllDoneOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", toolutil.EmojiSuccess, v.Message)
	toolutil.WriteHints(&b, "All to-do items cleared — use action 'list' to confirm")
	return b.String()
}

// FormatMarkAllDoneMarkdown returns an MCP tool result for marking all to-dos as done.
func FormatMarkAllDoneMarkdown(v MarkAllDoneOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkAllDoneMarkdownString(v))
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkDoneMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkAllDoneMarkdownString)
}

// markdown.go provides Markdown formatting functions for namespace MCP tool output.
package namespaces

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of namespaces as a Markdown CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatListMarkdownString renders a list of namespaces as a Markdown string.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Namespaces) == 0 {
		return "No namespaces found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Namespaces (%d)\n\n", len(out.Namespaces))
	toolutil.WriteListSummary(&b, len(out.Namespaces), out.Pagination)
	for _, ns := range out.Namespaces {
		fmt.Fprintf(&b, "- **%s** (ID: %d) — kind: %s, path: `%s`\n", ns.Name, ns.ID, ns.Kind, ns.FullPath)
	}
	b.WriteString(toolutil.FormatPagination(out.Pagination))
	toolutil.WriteHints(&b, "Use `gitlab_namespace_get` to view details of a specific namespace")
	return b.String()
}

// FormatMarkdown formats a single namespace as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a single namespace as a Markdown string.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Namespace: %s\n\n", out.Name)
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Name | %s |\n", out.Name)
	fmt.Fprintf(&b, "| Path | %s |\n", out.Path)
	fmt.Fprintf(&b, "| Full Path | %s |\n", out.FullPath)
	fmt.Fprintf(&b, "| Kind | %s |\n", out.Kind)
	if out.ParentID > 0 {
		fmt.Fprintf(&b, "| Parent ID | %d |\n", out.ParentID)
	}
	if out.WebURL != "" {
		fmt.Fprintf(&b, "| Web URL | %s |\n", out.WebURL)
	}
	if out.Plan != "" {
		fmt.Fprintf(&b, "| Plan | %s |\n", out.Plan)
	}
	toolutil.WriteHints(&b, "Use the namespace ID with project or group tools for further operations")
	return b.String()
}

// FormatExistsMarkdown formats a namespace existence check as a Markdown CallToolResult.
func FormatExistsMarkdown(out ExistsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatExistsMarkdownString(out))
}

// FormatExistsMarkdownString renders a namespace existence result as a Markdown string.
func FormatExistsMarkdownString(out ExistsOutput) string {
	var b strings.Builder
	if out.Exists {
		b.WriteString("Namespace **exists** (path is taken).\n")
	} else {
		b.WriteString("Namespace **does not exist** (path is available).\n")
	}
	if len(out.Suggests) > 0 {
		b.WriteString("\n**Suggestions:** ")
		b.WriteString(strings.Join(out.Suggests, ", "))
		b.WriteString("\n")
	}
	toolutil.WriteHints(&b, "Try one of the suggested paths if the namespace was not found")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatExistsMarkdownString)
}

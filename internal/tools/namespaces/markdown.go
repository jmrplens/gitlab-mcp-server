package namespaces

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		//gitlab:allow-unescaped ns.Kind: GitLab derives the kind from the namespace class and answers group or user, so this is a word the server chose.
		fmt.Fprintf(&b, "- **%s** (ID: %d), kind: %s, path: `%s`\n",
			toolutil.EscapeMdTableCell(ns.Name), ns.ID, ns.Kind, toolutil.EscapeMdTableCell(ns.FullPath))
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
	// For a user namespace GitLab keeps the name in step with the account
	// holder's display name, which is free text; the paths are slugs a person
	// chose, and every sibling that renders one escapes it.
	fmt.Fprintf(&b, "## Namespace: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Path | %s |\n", toolutil.EscapeMdTableCell(out.Path))
	fmt.Fprintf(&b, "| Full Path | %s |\n", toolutil.EscapeMdTableCell(out.FullPath))
	//gitlab:allow-unescaped out.Kind: GitLab derives the kind from the namespace class and answers group or user, so this is a word the server chose.
	fmt.Fprintf(&b, "| Kind | %s |\n", out.Kind)
	if out.ParentID > 0 {
		fmt.Fprintf(&b, "| Parent ID | %d |\n", out.ParentID)
	}
	if out.WebURL != "" {
		// GitLab builds this from the instance URL and the full path above, so
		// it inherits whatever that path can hold.
		fmt.Fprintf(&b, "| Web URL | %s |\n", toolutil.EscapeMdTableCell(out.WebURL))
	}
	if out.Plan != "" {
		//gitlab:allow-unescaped out.Plan: one of GitLab's own seeded subscription names, such as free or ultimate, which no API lets a person write.
		fmt.Fprintf(&b, "| Plan | %s |\n", out.Plan)
	}
	if out.TrialEndsOn != "" {
		//gitlab:allow-unescaped out.TrialEndsOn: a date toOutput rendered from a gl.ISOTime with time.Format, so it holds digits and dashes.
		fmt.Fprintf(&b, "| Trial Ends On | %s |\n", out.TrialEndsOn)
	}
	if out.MaxSeatsUsed != nil {
		fmt.Fprintf(&b, "| Max Seats Used | %d |\n", *out.MaxSeatsUsed)
	}
	if out.SeatsInUse != nil {
		fmt.Fprintf(&b, "| Seats In Use | %d |\n", *out.SeatsInUse)
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

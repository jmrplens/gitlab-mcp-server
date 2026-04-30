// markdown.go provides Markdown formatting functions for group CI/CD variable MCP tool output.
package groupvariables

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single group CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	if v.Key == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Variable: %s\n\n", v.Key)
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| Type | %s |\n", toolutil.EscapeMdTableCell(v.VariableType))
	fmt.Fprintf(&b, "| Protected | %s |\n", toolutil.BoolEmoji(v.Protected))
	fmt.Fprintf(&b, "| Masked | %s |\n", toolutil.BoolEmoji(v.Masked))
	if v.Hidden {
		fmt.Fprintf(&b, "| Hidden | %s |\n", toolutil.BoolEmoji(true))
	}
	fmt.Fprintf(&b, "| Raw | %s |\n", toolutil.BoolEmoji(v.Raw))
	fmt.Fprintf(&b, "| Environment Scope | %s |\n", toolutil.EscapeMdTableCell(v.EnvironmentScope))
	if v.Description != "" {
		fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(v.Description))
	}
	if !v.Masked && !v.Hidden {
		fmt.Fprintf(&b, "| Value | %s |\n", toolutil.EscapeMdTableCell(v.Value))
	} else {
		b.WriteString("| Value | [masked] |\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to change this variable",
		"Use action 'delete' to remove this variable",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of group CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Variables) == 0 {
		return "No group CI/CD variables found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Group CI/CD Variables (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Variables), out.Pagination)
	b.WriteString("| Key | Type | Protected | Masked | Scope |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, v := range out.Variables {
		fmt.Fprintf(&b, "| %s | %s | %t | %t | %s |\n",
			toolutil.EscapeMdTableCell(v.Key), v.VariableType, v.Protected, v.Masked, toolutil.EscapeMdTableCell(v.EnvironmentScope))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new group variable",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

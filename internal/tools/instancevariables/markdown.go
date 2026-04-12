// markdown.go provides Markdown formatting functions for instance-level CI/CD variable MCP tool output.

package instancevariables

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single instance CI/CD variable as Markdown.
func FormatOutputMarkdown(v Output) string {
	if v.Key == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Instance Variable: %s\n\n", v.Key)
	fmt.Fprintf(&b, "- **Type**: %s\n", v.VariableType)
	fmt.Fprintf(&b, "- **Protected**: %t\n", v.Protected)
	fmt.Fprintf(&b, "- **Masked**: %t\n", v.Masked)
	fmt.Fprintf(&b, "- **Raw**: %t\n", v.Raw)
	if v.Description != "" {
		fmt.Fprintf(&b, toolutil.FmtMdDescription, v.Description)
	}
	if !v.Masked {
		fmt.Fprintf(&b, "- **Value**: %s\n", v.Value)
	} else {
		b.WriteString("- **Value**: [masked]\n")
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to change this variable",
		"Use action 'delete' to remove this variable",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of instance CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Variables) == 0 {
		return "No instance CI/CD variables found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Instance CI/CD Variables (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Variables), out.Pagination)
	b.WriteString("| Key | Type | Protected | Masked |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, v := range out.Variables {
		fmt.Fprintf(&b, "| %s | %s | %t | %t |\n",
			toolutil.EscapeMdTableCell(v.Key), v.VariableType, v.Protected, v.Masked)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new instance variable",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

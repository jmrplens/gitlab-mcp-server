package instancevariables

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	toolutil.WriteHints(
		&b,
		"Use action 'update' to change this variable",
		"Use action 'delete' to remove this variable",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of instance CI/CD variables as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	return toolutil.FormatCICDVariableCollectionMarkdown(
		out.Variables, out.Pagination, toMarkdownVariable, "Instance CI/CD Variables", "No instance CI/CD variables found.\n", false,
		"Use action 'get' with key for full details",
		"Use action 'create' to add a new instance variable",
	)
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	flags := toolutil.CICDVariableFlags{Protected: v.Protected, Masked: v.Masked, Raw: v.Raw}
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, flags, "", v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

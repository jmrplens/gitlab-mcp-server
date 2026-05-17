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
	return toolutil.FormatCICDVariableListMarkdown(toolutil.CICDVariableMarkdowns(out.Variables, toMarkdownVariable), out.Pagination, toolutil.CICDVariableListMarkdownOptions{
		Title:        "Instance CI/CD Variables",
		EmptyMessage: "No instance CI/CD variables found.\n",
		Hints: []string{
			"Use action 'get' with key for full details",
			"Use action 'create' to add a new instance variable",
		},
	})
}

func toMarkdownVariable(v Output) toolutil.CICDVariableMarkdown {
	return toolutil.NewCICDVariableMarkdown(v.Key, v.Value, v.VariableType, v.Protected, v.Masked, false, v.Raw, "", v.Description)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

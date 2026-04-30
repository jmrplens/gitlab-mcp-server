// markdown.go provides Markdown formatting functions for GitLab feature MCP tool output.
package features

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatListMarkdown formats a list of features as markdown.
func FormatListMarkdown(output ListOutput) *mcp.CallToolResult {
	if len(output.Features) == 0 {
		return toolutil.ToolResultWithMarkdown("No feature flags found.\n")
	}

	var sb strings.Builder
	sb.WriteString("## Feature Flags\n\n")
	sb.WriteString("| Name | State | Gates |\n")
	sb.WriteString("|------|-------|-------|\n")
	for _, f := range output.Features {
		gates := formatGates(f.Gates)
		fmt.Fprintf(&sb, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(f.Name),
			toolutil.EscapeMdTableCell(f.State),
			toolutil.EscapeMdTableCell(gates))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_set_feature_flag` to toggle a specific feature")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatListDefinitionsMarkdown formats a list of feature definitions as markdown.
func FormatListDefinitionsMarkdown(output ListDefinitionsOutput) *mcp.CallToolResult {
	if len(output.Definitions) == 0 {
		return toolutil.ToolResultWithMarkdown("No feature definitions found.\n")
	}

	var sb strings.Builder
	sb.WriteString("## Feature Definitions\n\n")
	sb.WriteString("| Name | Type | Group | Milestone | Default Enabled |\n")
	sb.WriteString("|------|------|-------|-----------|----------------|\n")
	for _, d := range output.Definitions {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %v |\n",
			toolutil.EscapeMdTableCell(d.Name),
			toolutil.EscapeMdTableCell(d.Type),
			toolutil.EscapeMdTableCell(d.Group),
			toolutil.EscapeMdTableCell(d.Milestone),
			d.DefaultEnabled)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_set_feature_flag` to enable or disable a feature")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatFeatureMarkdown formats a single feature as markdown.
func FormatFeatureMarkdown(output SetOutput) *mcp.CallToolResult {
	f := output.Feature
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Feature Flag: %s\n\n", f.Name)
	sb.WriteString("| Property | Value |\n")
	sb.WriteString("|----------|-------|\n")
	fmt.Fprintf(&sb, "| State | %s |\n", f.State)
	fmt.Fprintf(&sb, "| Gates | %s |\n", toolutil.EscapeMdTableCell(formatGates(f.Gates)))
	if f.Definition != nil {
		fmt.Fprintf(&sb, "| Type | %s |\n", f.Definition.Type)
		fmt.Fprintf(&sb, "| Group | %s |\n", f.Definition.Group)
		fmt.Fprintf(&sb, "| Default Enabled | %v |\n", f.Definition.DefaultEnabled)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_set_feature_flag` to toggle this feature")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatListDefinitionsMarkdown)
	toolutil.RegisterMarkdownResult(FormatFeatureMarkdown)
}

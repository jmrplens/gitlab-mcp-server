package customattributes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats custom attributes list as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Custom Attributes\n\n")
	if len(out.Attributes) == 0 {
		sb.WriteString("No custom attributes found.\n")
		return sb.String()
	}
	sb.WriteString("| Key | Value |\n|---|---|\n")
	for _, a := range out.Attributes {
		fmt.Fprintf(&sb, "| %s | %s |\n",
			toolutil.EscapeMdTableCell(a.Key), toolutil.EscapeMdTableCell(a.Value))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_set_custom_attribute` to add or update an attribute")
	return sb.String()
}

// FormatGetMarkdown formats a single custom attribute as markdown.
func FormatGetMarkdown(out GetOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Custom Attribute\n\n**Key**: %s\n**Value**: %s\n", out.Key, out.Value)
	toolutil.WriteHints(&b,
		"Use `gitlab_set_custom_attribute` to update this attribute",
		"Use `gitlab_delete_custom_attribute` to remove it",
	)
	return b.String()
}

// FormatSetMarkdown formats a set custom attribute result as markdown.
func FormatSetMarkdown(out SetOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Custom Attribute Set\n\n**Key**: %s\n**Value**: %s\n", out.Key, out.Value)
	toolutil.WriteHints(&b, "Use `gitlab_get_custom_attribute` to verify the value")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatSetMarkdown)
}

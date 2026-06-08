package securitycategories

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown renders a security category as Markdown.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	sb.WriteString("## Security Category\n\n")
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | `%d` |\n", out.ID)
	fmt.Fprintf(&sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	if out.Description != "" {
		fmt.Fprintf(&sb, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	}
	fmt.Fprintf(&sb, "| Multiple selection | %t |\n", out.MultipleSelection)
	if out.EditableState != "" {
		fmt.Fprintf(&sb, "| Editable state | `%s` |\n", out.EditableState)
	}
	if out.TemplateType != "" {
		fmt.Fprintf(&sb, "| Template type | `%s` |\n", out.TemplateType)
	}
	if len(out.SecurityAttributes) > 0 {
		sb.WriteString("\n### Attributes\n\n")
		sb.WriteString("| ID | Name | Color | Editable state |\n")
		sb.WriteString("|----|------|-------|----------------|\n")
		for _, attribute := range out.SecurityAttributes {
			fmt.Fprintf(
				&sb, "| `%d` | %s | `%s` | `%s` |\n",
				attribute.ID,
				toolutil.EscapeMdTableCell(attribute.Name),
				attribute.Color,
				attribute.EditableState,
			)
		}
	}
	toolutil.WriteHints(
		&sb,
		"Use `gitlab_create_security_attribute` to add attributes under this category",
		"Use `gitlab_update_security_category` to rename or describe this category",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
}

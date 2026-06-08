package securityattributes

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	fieldValueTableHeader    = "| Field | Value |\n"
	fieldValueTableSeparator = "|-------|-------|\n"
)

// FormatOutputMarkdown renders a security attribute as Markdown.
func FormatOutputMarkdown(out Output) string {
	var sb strings.Builder
	sb.WriteString("## Security Attribute\n\n")
	writeAttributeTable(&sb, out)
	toolutil.WriteHints(
		&sb,
		"Use `gitlab_update_security_attribute` to edit this attribute",
		"Use `gitlab_update_project_security_attributes` to apply attributes to a project",
	)
	return sb.String()
}

// FormatCreateMarkdown renders created security attributes as Markdown.
func FormatCreateMarkdown(out CreateOutput) string {
	var sb strings.Builder
	sb.WriteString(toolutil.EmojiSuccess + " Security attributes created.\n\n")
	if len(out.Attributes) == 0 {
		sb.WriteString("No security attributes returned.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Name | Color | Category |\n")
	sb.WriteString("|----|------|-------|----------|\n")
	for _, attribute := range out.Attributes {
		category := "-"
		if attribute.SecurityCategory != nil {
			category = toolutil.EscapeMdTableCell(attribute.SecurityCategory.Name)
		}
		fmt.Fprintf(
			&sb, "| `%d` | %s | `%s` | %s |\n",
			attribute.ID,
			toolutil.EscapeMdTableCell(attribute.Name),
			toolutil.EscapeMdTableCell(attribute.Color),
			category,
		)
	}
	toolutil.WriteHints(
		&sb,
		toolutil.HintPreserveLinks,
		"Use `gitlab_update_project_security_attributes` to apply attributes to a project",
		"Use `gitlab_bulk_update_security_attributes` to apply attributes to many groups or projects",
	)
	return sb.String()
}

// FormatProjectUpdateMarkdown renders project security attribute update results.
func FormatProjectUpdateMarkdown(out ProjectUpdateOutput) string {
	var sb strings.Builder
	sb.WriteString(toolutil.EmojiSuccess + " Project security attributes updated.\n\n")
	sb.WriteString(fieldValueTableHeader)
	sb.WriteString(fieldValueTableSeparator)
	fmt.Fprintf(&sb, "| Added | `%d` |\n", out.AddedCount)
	fmt.Fprintf(&sb, "| Removed | `%d` |\n", out.RemovedCount)
	return sb.String()
}

// FormatBulkUpdateMarkdown renders bulk security attribute update results.
func FormatBulkUpdateMarkdown(out BulkUpdateOutput) string {
	var sb strings.Builder
	sb.WriteString(toolutil.EmojiSuccess + " Security attributes updated in bulk.\n\n")
	sb.WriteString(fieldValueTableHeader)
	sb.WriteString(fieldValueTableSeparator)
	fmt.Fprintf(&sb, "| Mode | `%s` |\n", out.Mode)
	fmt.Fprintf(&sb, "| Attributes | `%v` |\n", out.AttributeIDs)
	if len(out.GroupIDs) > 0 {
		fmt.Fprintf(&sb, "| Groups | `%v` |\n", out.GroupIDs)
	}
	if len(out.ProjectIDs) > 0 {
		fmt.Fprintf(&sb, "| Projects | `%v` |\n", out.ProjectIDs)
	}
	return sb.String()
}

func writeAttributeTable(sb *strings.Builder, out Output) {
	sb.WriteString(fieldValueTableHeader)
	sb.WriteString(fieldValueTableSeparator)
	fmt.Fprintf(sb, "| ID | `%d` |\n", out.ID)
	fmt.Fprintf(sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(sb, "| Color | `%s` |\n", toolutil.EscapeMdTableCell(out.Color))
	if out.Description != "" {
		fmt.Fprintf(sb, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	}
	if out.EditableState != "" {
		fmt.Fprintf(sb, "| Editable state | `%s` |\n", out.EditableState)
	}
	if out.SecurityCategory != nil {
		fmt.Fprintf(sb, "| Category | %s (`%d`) |\n", toolutil.EscapeMdTableCell(out.SecurityCategory.Name), out.SecurityCategory.ID)
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
	toolutil.RegisterMarkdown(FormatProjectUpdateMarkdown)
	toolutil.RegisterMarkdown(FormatBulkUpdateMarkdown)
}

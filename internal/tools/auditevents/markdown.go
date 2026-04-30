// markdown.go provides Markdown formatting for audit event outputs.
package auditevents

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkdown renders a single audit event as Markdown.
func FormatMarkdown(e Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Audit Event #%d\n\n", e.ID)
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", e.ID)
	fmt.Fprintf(&sb, "| Author ID | %d |\n", e.AuthorID)
	fmt.Fprintf(&sb, "| Entity ID | %d |\n", e.EntityID)
	fmt.Fprintf(&sb, "| Entity Type | %s |\n", toolutil.EscapeMdTableCell(e.EntityType))
	fmt.Fprintf(&sb, "| Event Name | %s |\n", toolutil.EscapeMdTableCell(e.EventName))
	fmt.Fprintf(&sb, "| Event Type | %s |\n", toolutil.EscapeMdTableCell(e.EventType))
	fmt.Fprintf(&sb, "| Created At | %s |\n", e.CreatedAt)
	if e.Details.AuthorName != "" {
		fmt.Fprintf(&sb, "| Author Name | %s |\n", toolutil.EscapeMdTableCell(e.Details.AuthorName))
	}
	if e.Details.TargetType != "" {
		fmt.Fprintf(&sb, "| Target Type | %s |\n", toolutil.EscapeMdTableCell(e.Details.TargetType))
	}
	if e.Details.TargetDetails != "" {
		fmt.Fprintf(&sb, "| Target Details | %s |\n", toolutil.EscapeMdTableCell(e.Details.TargetDetails))
	}
	if e.Details.IPAddress != "" {
		fmt.Fprintf(&sb, "| IP Address | %s |\n", e.Details.IPAddress)
	}
	if e.Details.EntityPath != "" {
		fmt.Fprintf(&sb, "| Entity Path | %s |\n", toolutil.EscapeMdTableCell(e.Details.EntityPath))
	}
	toolutil.WriteHints(&sb,
		"Use `gitlab_list_project_audit_events` or `gitlab_list_group_audit_events` to browse more events",
	)
	return sb.String()
}

// FormatListMarkdown renders a paginated list of audit events as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Audit Events\n\n")
	if len(out.AuditEvents) == 0 {
		sb.WriteString("No audit events found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Event Name | Entity Type | Entity ID | Author ID | Created At |\n")
	sb.WriteString("|-----|------------|-------------|-----------|-----------|------------|\n")
	for _, e := range out.AuditEvents {
		fmt.Fprintf(&sb, "| %d | %s | %s | %d | %d | %s |\n",
			e.ID,
			toolutil.EscapeMdTableCell(e.EventName),
			toolutil.EscapeMdTableCell(e.EntityType),
			e.EntityID,
			e.AuthorID,
			e.CreatedAt,
		)
	}
	toolutil.WriteListSummary(&sb, len(out.AuditEvents), out.Pagination)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

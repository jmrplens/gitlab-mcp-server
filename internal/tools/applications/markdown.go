package applications

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats application list as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Applications\n\n")
	toolutil.WriteListSummary(&sb, len(out.Applications), out.Pagination)
	if len(out.Applications) == 0 {
		sb.WriteString("No applications found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Name | App ID | Callback URL | Confidential | Scopes |\n|---|---|---|---|---|---|\n")
	for _, a := range out.Applications {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %v | %s |\n",
			a.ID,
			toolutil.EscapeMdTableCell(a.ApplicationName),
			toolutil.EscapeMdTableCell(a.ApplicationID),
			toolutil.EscapeMdTableCell(a.CallbackURL),
			a.Confidential,
			toolutil.EscapeMdTableCell(strings.Join(a.Scopes, ", ")))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_create_application` to register a new application")
	return sb.String()
}

// FormatCreateMarkdown formats a created application as markdown.
func FormatCreateMarkdown(out CreateOutput) string {
	var sb strings.Builder
	sb.WriteString("## Application Created\n\n")
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.ApplicationName))
	fmt.Fprintf(&sb, "| App ID | %s |\n", toolutil.EscapeMdTableCell(out.ApplicationID))
	fmt.Fprintf(&sb, "| Callback URL | %s |\n", toolutil.EscapeMdTableCell(out.CallbackURL))
	fmt.Fprintf(&sb, "| Confidential | %v |\n", out.Confidential)
	fmt.Fprintf(&sb, "| Secret | %s |\n", toolutil.EscapeMdTableCell(out.Secret))
	fmt.Fprintf(&sb, "| Scopes | %s |\n", toolutil.EscapeMdTableCell(strings.Join(out.Scopes, ", ")))
	toolutil.WriteHints(&sb, "Store the application secret securely — it cannot be retrieved later")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
	toolutil.RegisterMarkdown(FormatRenewSecretMarkdown)
}

// FormatRenewSecretMarkdown formats a renewed application secret as markdown.
func FormatRenewSecretMarkdown(out RenewSecretOutput) string {
	var sb strings.Builder
	sb.WriteString("## Application Secret Renewed\n\n")
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.ApplicationName))
	fmt.Fprintf(&sb, "| App ID | %s |\n", toolutil.EscapeMdTableCell(out.ApplicationID))
	fmt.Fprintf(&sb, "| Callback URL | %s |\n", toolutil.EscapeMdTableCell(out.CallbackURL))
	fmt.Fprintf(&sb, "| Confidential | %v |\n", out.Confidential)
	fmt.Fprintf(&sb, "| New Secret | %s |\n", toolutil.EscapeMdTableCell(out.Secret))
	fmt.Fprintf(&sb, "| Scopes | %s |\n", toolutil.EscapeMdTableCell(strings.Join(out.Scopes, ", ")))
	toolutil.WriteHints(&sb, "Store the new secret securely — the previous secret is now invalid and any client using it must be updated")
	return sb.String()
}

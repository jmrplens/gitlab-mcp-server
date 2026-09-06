package errortracking

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatSettingsMarkdown formats error tracking settings as markdown.
func FormatSettingsMarkdown(out SettingsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Error Tracking Settings\n\n")
	fmt.Fprintf(&sb, "- **Active**: %v\n", out.Active)
	fmt.Fprintf(&sb, "- **Integrated**: %v\n", out.Integrated)
	if out.ProjectName != "" {
		fmt.Fprintf(&sb, "- **Project Name**: %s\n", toolutil.EscapeMdTableCell(out.ProjectName))
	}
	if out.SentryExternalURL != "" {
		// Both come from the Sentry integration a maintainer configured.
		fmt.Fprintf(&sb, "- **Sentry URL**: %s\n", toolutil.EscapeMdTableCell(out.SentryExternalURL))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_list_error_tracking_client_keys` to view client keys")
	return sb.String()
}

// FormatListKeysMarkdown formats client keys as markdown.
func FormatListKeysMarkdown(out ListClientKeysOutput) string {
	var sb strings.Builder
	sb.WriteString("## Error Tracking Client Keys\n\n")
	if len(out.Keys) == 0 {
		sb.WriteString("No client keys found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Active | Public Key |\n|----|--------|------------|\n")
	for _, k := range out.Keys {
		fmt.Fprintf(&sb, "| %d | %v | %s |\n", k.ID, k.Active, toolutil.EscapeMdTableCell(k.PublicKey))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_create_error_tracking_client_key` to generate a new key")
	return sb.String()
}

// FormatKeyMarkdown formats a single client key as markdown.
func FormatKeyMarkdown(k ClientKeyItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Error Tracking Client Key\n\n- **ID**: %d\n- **Active**: %v\n- **Public Key**: %s\n- **Sentry DSN**: %s\n",
		//gitlab:allow-unescaped k.PublicKey: the client key GitLab generated, hexadecimal digits.
		//gitlab:allow-unescaped k.SentryDsn: the DSN GitLab composed from the instance URL, that generated key and a numeric project id.
		k.ID, k.Active, k.PublicKey, k.SentryDsn)
	toolutil.WriteHints(&b, "Use `gitlab_delete_error_tracking_client_key` to revoke this key")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatSettingsMarkdown)
	toolutil.RegisterMarkdown(FormatListKeysMarkdown)
	toolutil.RegisterMarkdown(FormatKeyMarkdown)
}

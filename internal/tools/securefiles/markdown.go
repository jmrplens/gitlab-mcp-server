package securefiles

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// expiryOrDash formats an optional expiry timestamp as RFC 3339, or "—" when nil.
// The em dash is used in table cells to signal "no expiry set" rather than an empty cell.
func expiryOrDash(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return toolutil.FormatTimePtr(t)
}

// FormatListMarkdown formats secure files as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Secure Files\n\n")
	toolutil.WriteListSummary(&sb, len(out.Files), out.Pagination)
	if len(out.Files) == 0 {
		sb.WriteString("No secure files found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Name | Checksum Algorithm | Expires At |\n|----|------|-----------|-----------|\n")
	for _, f := range out.Files {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", f.ID, toolutil.EscapeMdTableCell(f.Name), f.ChecksumAlgorithm, expiryOrDash(f.ExpiresAt))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_show_secure_file` to view details of a specific file")
	return sb.String()
}

// FormatShowMarkdown formats a secure file as markdown.
func FormatShowMarkdown(f SecureFileItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Secure File\n\n- **ID**: %d\n- **Name**: %s\n- **Checksum**: %s\n- **Algorithm**: %s\n- **Created At**: %s\n- **Expires At**: %s\n",
		f.ID, f.Name, f.Checksum, f.ChecksumAlgorithm, toolutil.FormatTimePtr(f.CreatedAt), toolutil.FormatTimePtr(f.ExpiresAt))
	if f.Metadata != nil {
		m := f.Metadata
		fmt.Fprintf(&b, "\n### Certificate Metadata\n\n- **ID**: %s\n- **Expires At**: %s\n- **Issuer CN**: %s\n- **Subject CN**: %s\n",
			m.ID, toolutil.FormatTimePtr(m.ExpiresAt), m.Issuer.CN, m.Subject.CN)
	}
	toolutil.WriteHints(&b, "Use `gitlab_download_secure_file` to download this file")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatShowMarkdown)
}

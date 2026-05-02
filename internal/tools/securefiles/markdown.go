package securefiles

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown formats secure files as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Secure Files\n\n")
	toolutil.WriteListSummary(&sb, len(out.Files), out.Pagination)
	if len(out.Files) == 0 {
		sb.WriteString("No secure files found.\n")
		return sb.String()
	}
	sb.WriteString("| ID | Name | Checksum Algorithm |\n|----|------|-----------|\n")
	for _, f := range out.Files {
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", f.ID, toolutil.EscapeMdTableCell(f.Name), f.ChecksumAlgorithm)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_show_secure_file` to view details of a specific file")
	return sb.String()
}

// FormatShowMarkdown formats a secure file as markdown.
func FormatShowMarkdown(f SecureFileItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Secure File\n\n- **ID**: %d\n- **Name**: %s\n- **Checksum**: %s\n- **Algorithm**: %s\n",
		f.ID, f.Name, f.Checksum, f.ChecksumAlgorithm)
	toolutil.WriteHints(&b, "Use `gitlab_download_secure_file` to download this file")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatShowMarkdown)
}

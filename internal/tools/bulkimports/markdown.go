// markdown.go provides Markdown formatting functions for bulk import migration MCP tool output.

package bulkimports

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatStartMigrationMarkdown formats a start migration result as markdown.
func FormatStartMigrationMarkdown(out MigrationOutput) string {
	var sb strings.Builder
	sb.WriteString("## Bulk Import Migration Started\n\n")
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&sb, "| Source Type | %s |\n", out.SourceType)
	fmt.Fprintf(&sb, "| Source URL | %s |\n", toolutil.EscapeMdTableCell(out.SourceURL))
	fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	fmt.Fprintf(&sb, "| Updated At | %s |\n", toolutil.FormatTime(out.UpdatedAt))
	fmt.Fprintf(&sb, "| Has Failures | %v |\n", out.HasFailures)
	toolutil.WriteHints(&sb, "Monitor migration progress by checking status periodically")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatStartMigrationMarkdown)
}

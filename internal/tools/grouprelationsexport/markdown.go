package grouprelationsexport

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatListExportStatusMarkdownString)
}

// FormatListExportStatusMarkdownString renders group relations export statuses.
func FormatListExportStatusMarkdownString(o ListExportStatusOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Relations Export Status (%d)\n\n", len(o.Statuses))
	toolutil.WriteListSummary(&b, len(o.Statuses), o.Pagination)
	if len(o.Statuses) == 0 {
		b.WriteString("No export statuses found.\n")
	} else {
		toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
		b.WriteString("| Relation | Status | Batched | Batches | Error |\n")
		b.WriteString("|---|---|---|---|---|\n")
		for _, s := range o.Statuses {
			fmt.Fprintf(&b, "| %s | %d | %s | %d | %s |\n",
				toolutil.EscapeMdTableCell(s.Relation),
				s.Status,
				toolutil.BoolEmoji(s.Batched),
				s.BatchesCount,
				toolutil.EscapeMdTableCell(s.Error))
		}
	}
	return b.String()
}

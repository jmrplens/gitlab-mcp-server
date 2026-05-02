package groupmarkdownuploads

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

// FormatListMarkdownString renders a list of group markdown uploads.
func FormatListMarkdownString(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Markdown Uploads (%d)\n\n", len(o.Uploads))
	toolutil.WriteListSummary(&b, len(o.Uploads), o.Pagination)
	if len(o.Uploads) == 0 {
		b.WriteString("No uploads found.\n")
	} else {
		toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
		b.WriteString("| ID | Filename | Size | Created |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, u := range o.Uploads {
			fmt.Fprintf(&b, "| %d | %s | %d | %s |\n",
				u.ID,
				toolutil.EscapeMdTableCell(u.Filename),
				u.Size,
				u.CreatedAt)
		}
	}
	return b.String()
}

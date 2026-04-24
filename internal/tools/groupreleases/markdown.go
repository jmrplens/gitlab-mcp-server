// markdown.go provides Markdown formatting functions for group release MCP tool output.

package groupreleases

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of group releases as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Releases) == 0 {
		return "No group releases found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Releases), out.Pagination)
	b.WriteString("| Tag | Name | Released | Author |\n| --- | --- | --- | --- |\n")
	for _, r := range out.Releases {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(r.TagName),
			toolutil.EscapeMdTableCell(r.Name),
			r.ReleasedAt,
			r.Author,
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

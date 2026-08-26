package dbmigrations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatMarkMarkdown formats the mark migration result as markdown.
func FormatMarkMarkdown(out MarkOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Mark Migration\n\n**Status**: %s | **Version**: %d\n", out.Status, out.Version)
	toolutil.WriteHints(&b, "Verify overall migration state in the GitLab admin area (no list action is exposed here)")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkMarkdown)
}

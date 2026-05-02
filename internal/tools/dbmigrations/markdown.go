package dbmigrations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatMarkMarkdown formats the mark migration result as markdown.
func FormatMarkMarkdown(out MarkOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Mark Migration\n\n**Status**: %s | **Version**: %d\n", out.Status, out.Version)
	toolutil.WriteHints(&b, "Use `gitlab_list_db_migrations` to verify migration state")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkMarkdown)
}

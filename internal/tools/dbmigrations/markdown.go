package dbmigrations

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkMarkdown renders the result of marking a migration as the card of
// one object.
//
// It used to write "**Status**: %s | **Version**: %d" as a bare paragraph,
// which put the status into a line the card escaped nothing in: a status
// carrying a tag reached the page as markup, and one carrying a line break
// added a heading or a list item of its own.
func FormatMarkMarkdown(out MarkOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Mark Migration")
	c.Field("Status", out.Status)
	c.Int("Version", out.Version)
	c.End("Verify overall migration state in the GitLab admin area (no list action is exposed here)")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkMarkdown)
}

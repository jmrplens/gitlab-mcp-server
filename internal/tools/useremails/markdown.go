// markdown.go provides Markdown formatting functions for user email
// MCP tool output.

package useremails

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteMarkdownString)
}

// FormatDeleteMarkdownString renders a deletion confirmation.
func FormatDeleteMarkdownString(o DeleteOutput) string {
	return fmt.Sprintf("## Email Deleted\n\n- **Email ID**: %d\n- **Deleted**: %s\n",
		o.EmailID, toolutil.BoolEmoji(o.Deleted))
}

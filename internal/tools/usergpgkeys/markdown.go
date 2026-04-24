// markdown.go provides Markdown formatting functions for user GPG key
// MCP tool output.
package usergpgkeys

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteMarkdownString)
}

// FormatDeleteMarkdownString renders a GPG key deletion confirmation.
func FormatDeleteMarkdownString(o DeleteOutput) string {
	return fmt.Sprintf("## GPG Key Deleted\n\n- **Key ID**: %d\n- **Deleted**: %s\n",
		o.KeyID, toolutil.BoolEmoji(o.Deleted))
}

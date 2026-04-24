// markdown.go provides Markdown formatting functions for impersonation token
// MCP tool output.

package impersonationtokens

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatPATMarkdownString)
	toolutil.RegisterMarkdown(FormatRevokeMarkdownString)
}

// FormatRevokeMarkdownString renders a revocation confirmation.
func FormatRevokeMarkdownString(o RevokeOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Token Revoked\n\n")
	fmt.Fprintf(&b, "- **User ID**: %d\n", o.UserID)
	fmt.Fprintf(&b, "- **Token ID**: %d\n", o.TokenID)
	fmt.Fprintf(&b, "- **Revoked**: %s\n", toolutil.BoolEmoji(o.Revoked))
	return b.String()
}

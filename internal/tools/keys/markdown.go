package keys

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatMarkdown renders a key as Markdown wrapped in an
// [mcp.CallToolResult]. It is registered as the package's
// formatter for [Output].
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a key as a Markdown summary with the ID,
// title, truncated public key, owner, and creation timestamp.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	b.WriteString("## SSH Key\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	if out.Title != "" {
		fmt.Fprintf(&b, toolutil.FmtMdTitle, toolutil.EscapeMdTableCell(out.Title))
	}
	// The key is truncated here rather than constrained, and its trailing
	// comment is whatever the key's owner typed.
	fmt.Fprintf(&b, "- **Key**: `%s`\n", toolutil.EscapeMdTableCell(truncateKey(out.Key)))
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	fmt.Fprintf(&b, "- **User**: %s (ID: %d, @%s)\n",
		toolutil.EscapeMdTableCell(out.User.Name), out.User.ID, toolutil.EscapeMdTableCell(out.User.Username))
	toolutil.WriteHints(&b, "Use `gitlab_get_key_by_fingerprint` to look up a key by its fingerprint")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

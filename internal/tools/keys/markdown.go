// markdown.go provides Markdown formatting functions for SSH key MCP tool output.
package keys

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMarkdown formats a key as Markdown.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a key as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	b.WriteString("## SSH Key\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	if out.Title != "" {
		fmt.Fprintf(&b, toolutil.FmtMdTitle, out.Title)
	}
	fmt.Fprintf(&b, "- **Key**: `%s`\n", truncateKey(out.Key))
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(out.CreatedAt))
	}
	fmt.Fprintf(&b, "- **User**: %s (ID: %d, @%s)\n", out.User.Name, out.User.ID, out.User.Username)
	toolutil.WriteHints(&b, "Use `gitlab_list_keys` to search for other SSH keys")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

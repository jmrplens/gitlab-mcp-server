package keys

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders a key as Markdown wrapped in an
// [mcp.CallToolResult]. It is registered as the package's
// formatter for [Output].
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders a key as a card: the identity, the truncated
// public key, when it expires and when it was last used, and the owning user
// as a nested object.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("SSH Key #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Title", out.Title)
	// The key is truncated here rather than constrained, and its trailing
	// comment is whatever the key's owner typed.
	c.Code("Key", truncateKey(out.Key))
	c.Field("Usage Type", out.UsageType)
	c.Time("Created", out.CreatedAt)
	c.Time("Expires", out.ExpiresAt)
	c.Time("Last Used", out.LastUsedAt)
	if out.User != (UserOutput{}) {
		user := c.Sub("User")
		user.Int("ID", out.User.ID)
		user.Field("Name", out.User.Name)
		user.Field("Username", toolutil.MdUserHandle(out.User.Username))
	}
	c.End(toolutil.HintAction("keys.key_get_by_fingerprint", "look a key up by its fingerprint instead of its ID"))
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}

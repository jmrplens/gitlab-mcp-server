package usergpgkeys

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintGetGPGKey names the canonical catalog ID every surface accepts, rather
// than an individual tool name the default surface does not register.
var hintGetGPGKey = toolutil.HintAction("user.get_gpg_key", "view full key details")

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteMarkdownString)
}

// FormatDeleteMarkdownString renders a GPG key deletion confirmation.
func FormatDeleteMarkdownString(o DeleteOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "GPG Key Deleted")
	card.Int("Key ID", o.KeyID)
	card.Bool("Deleted", o.Deleted)
	card.End()
	return b.String()
}

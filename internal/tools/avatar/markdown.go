package avatar

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown renders the avatar lookup as a card. The address is the whole
// answer, so it is written as the link it is rather than as escaped text: a
// reader told to "use the avatar URL directly" has to be able to open it, and
// an empty answer writes no row at all rather than a label with nothing after
// it.
func FormatMarkdown(out GetOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Avatar")
	card.URL(out.AvatarURL)
	card.End("Use the avatar URL directly in your application")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdown)
}

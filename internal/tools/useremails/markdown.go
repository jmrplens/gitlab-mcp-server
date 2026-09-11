package useremails

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintGetEmail names the canonical catalog ID every surface accepts, rather
// than an individual tool name the default surface does not register.
var hintGetEmail = toolutil.HintAction("user.get_email", "read one address")

// confirmationValue renders the confirmation state of an address. An address
// GitLab has not confirmed is the answer a reader is asking for, and the card
// used to write no row at all for it while the list wrote a dash: neither says
// that the address is waiting for its confirmation mail, and the row is now
// always written and always says which of the two it is.
func confirmationValue(confirmedAt string) string {
	if confirmedAt == "" {
		return toolutil.BoolEmoji(false) + " awaiting confirmation"
	}
	return toolutil.BoolEmoji(true) + " " + toolutil.FormatTime(confirmedAt)
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatDeleteMarkdownString)
}

// FormatDeleteMarkdownString renders a deletion confirmation.
func FormatDeleteMarkdownString(o DeleteOutput) string {
	var b strings.Builder
	card := toolutil.NewCard(&b, "Email Deleted")
	card.Int("Email ID", o.EmailID)
	card.Bool("Deleted", o.Deleted)
	card.End()
	return b.String()
}

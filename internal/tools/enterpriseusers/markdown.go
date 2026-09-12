package enterpriseusers

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The next steps an enterprise-user result offers, each naming the canonical
// catalog ID every surface accepts rather than an individual tool name the
// default surface does not register.
var (
	hintDisable2FA = toolutil.HintAction("enterprise_user.disable_2fa", "reset two-factor authentication")
	hintListUsers  = toolutil.HintAction("enterprise_user.list", "browse all enterprise users")
	hintGetUser    = toolutil.HintAction("enterprise_user.get", "read one enterprise user in full")
)

// FormatOutputMarkdown renders one enterprise user as a card. Locked is a
// warning rather than a tick, since a locked account is not a success, and the
// account's address is a link so a reader can open the profile.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	card := toolutil.NewCard(&b, "Enterprise User: "+o.Name)
	card.Int("ID", o.ID)
	card.Field("Username", o.Username)
	card.Field("Email", o.Email)
	card.Field("State", o.State)
	card.Bool("Admin", o.IsAdmin)
	card.Bool("2FA Enabled", o.TwoFactorEnabled)
	card.Bool("External", o.External)
	card.Bool("Bot", o.Bot)
	card.Warn("Locked", o.Locked)
	card.URL(o.WebURL)
	card.Time("Created", o.CreatedAt)
	card.End(hintDisable2FA, hintListUsers)
	return b.String()
}

// FormatListMarkdown renders enterprise users as a Markdown table. The handle
// links to the profile, which is what the preserve-links hint the footer
// carries is about: the table used to name that hint over a render with no
// link in it.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Users) == 0 {
		return toolutil.EmptyMessage("enterprise users")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Enterprise Users", len(out.Users), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email", "State", "2FA"))
	for _, u := range out.Users {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.MdUserLink(u.Username, u.WebURL),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.Email),
			toolutil.EscapeMdTableCell(u.State),
			toolutil.BoolEmoji(u.TwoFactorEnabled),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true, hintGetUser)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

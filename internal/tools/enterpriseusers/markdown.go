package enterpriseusers

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single enterprise user as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Enterprise User: %s\n\n", toolutil.EscapeMdHeading(o.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	fmt.Fprintf(&b, toolutil.FmtMdUsername, o.Username)
	fmt.Fprintf(&b, toolutil.FmtMdEmail, o.Email)
	fmt.Fprintf(&b, toolutil.FmtMdState, o.State)
	fmt.Fprintf(&b, "- **Admin**: %v\n", o.IsAdmin)
	fmt.Fprintf(&b, "- **2FA Enabled**: %v\n", o.TwoFactorEnabled)
	fmt.Fprintf(&b, "- **External**: %v\n", o.External)
	fmt.Fprintf(&b, "- **Locked**: %v\n", o.Locked)
	fmt.Fprintf(&b, "- **Bot**: %v\n", o.Bot)
	if o.WebURL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, o.WebURL)
	}
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, o.CreatedAt)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_disable_2fa_enterprise_user` to reset two-factor authentication",
		"Use `gitlab_list_enterprise_users` to browse all enterprise users",
	)
	return b.String()
}

// FormatListMarkdown renders enterprise users as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Users) == 0 {
		return "No enterprise users found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Enterprise Users (%d)\n\n", len(out.Users))
	toolutil.WriteListSummary(&b, len(out.Users), out.Pagination)
	b.WriteString("| ID | Username | Name | Email | State | 2FA |\n")
	b.WriteString("| --: | -------- | ---- | ----- | ----- | --- |\n")
	for _, u := range out.Users {
		twoFA := "No"
		if u.TwoFactorEnabled {
			twoFA = "Yes"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
			u.ID,
			toolutil.EscapeMdTableCell(u.Username),
			toolutil.EscapeMdTableCell(u.Name),
			toolutil.EscapeMdTableCell(u.Email),
			toolutil.EscapeMdTableCell(u.State),
			twoFA,
		)
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

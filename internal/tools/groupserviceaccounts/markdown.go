package groupserviceaccounts

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatPATMarkdownString)
	toolutil.RegisterMarkdown(FormatListPATMarkdownString)
}

// FormatMarkdownString renders a service account as Markdown.
func FormatMarkdownString(o Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Service Account: %s\n\n", toolutil.EscapeMdHeading(o.Username))
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	fmt.Fprintf(&b, "- **Name**: %s\n", toolutil.EscapeMdTableCell(o.Name))
	fmt.Fprintf(&b, "- **Username**: %s\n", toolutil.EscapeMdTableCell(o.Username))
	fmt.Fprintf(&b, toolutil.FmtMdEmail, toolutil.EscapeMdTableCell(o.Email))
	return b.String()
}

// FormatListMarkdownString renders a paginated list of service accounts.
func FormatListMarkdownString(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Group Service Accounts (%d)\n\n", len(o.Accounts))
	toolutil.WriteListSummary(&b, len(o.Accounts), o.Pagination)
	if len(o.Accounts) == 0 {
		b.WriteString("No service accounts found.\n")
	} else {
		toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
		b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email"))
		for _, a := range o.Accounts {
			fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
				a.ID,
				toolutil.EscapeMdTableCell(a.Username),
				toolutil.EscapeMdTableCell(a.Name),
				toolutil.EscapeMdTableCell(a.Email))
		}
	}
	return b.String()
}

// FormatPATMarkdownString renders a service account PAT as Markdown.
func FormatPATMarkdownString(o PATOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Personal Access Token: %s\n\n", toolutil.EscapeMdHeading(o.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, o.ID)
	fmt.Fprintf(&b, "- **Active**: %s\n", toolutil.BoolEmoji(o.Active))
	fmt.Fprintf(&b, "- **Revoked**: %s\n", toolutil.BoolEmoji(o.Revoked))
	//gitlab:allow-unescaped strings.Join(o.Scopes, ", "): token scopes, which GitLab refuses to store outside its own fixed set.
	fmt.Fprintf(&b, "- **Scopes**: %s\n", strings.Join(o.Scopes, ", "))
	fmt.Fprintf(&b, "- **User ID**: %d\n", o.UserID)
	if o.CreatedAt != "" {
		//gitlab:allow-unescaped o.CreatedAt: a timestamp toPATOutput formatted itself, with time.Time.Format.
		fmt.Fprintf(&b, "- **Created**: %s\n", o.CreatedAt)
	}
	if o.LastUsedAt != "" {
		//gitlab:allow-unescaped o.LastUsedAt: a timestamp toPATOutput formatted itself, with time.Time.Format.
		fmt.Fprintf(&b, "- **Last Used**: %s\n", o.LastUsedAt)
	}
	if o.ExpiresAt != "" {
		//gitlab:allow-unescaped o.ExpiresAt: a date toPATOutput formatted itself, on the ISO date layout.
		fmt.Fprintf(&b, "- **Expires**: %s\n", o.ExpiresAt)
	}
	if o.Token != "" {
		//gitlab:allow-unescaped o.Token: the secret GitLab generated, which the reader has to copy back verbatim.
		fmt.Fprintf(&b, "- **Token**: `%s`\n", o.Token)
	}
	return b.String()
}

// FormatListPATMarkdownString renders a paginated list of PATs.
func FormatListPATMarkdownString(o ListPATOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Service Account Tokens (%d)\n\n", len(o.Tokens))
	toolutil.WriteListSummary(&b, len(o.Tokens), o.Pagination)
	if len(o.Tokens) == 0 {
		b.WriteString("No tokens found.\n")
	} else {
		toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
		b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires"))
		for _, t := range o.Tokens {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
				t.ID,
				toolutil.EscapeMdTableCell(t.Name),
				toolutil.BoolEmoji(t.Active),
				toolutil.BoolEmoji(t.Revoked),
				strings.Join(t.Scopes, ", "),
				//gitlab:allow-unescaped t.ExpiresAt: a date toPATOutput formatted itself, on the ISO date layout.
				t.ExpiresAt)
		}
	}
	return b.String()
}

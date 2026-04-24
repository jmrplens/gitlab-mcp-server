// markdown.go provides Markdown formatting functions for group service account
// MCP tool output.
package groupserviceaccounts

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	fmt.Fprintf(&b, "- **Name**: %s\n", o.Name)
	fmt.Fprintf(&b, "- **Username**: %s\n", o.Username)
	fmt.Fprintf(&b, toolutil.FmtMdEmail, o.Email)
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
		b.WriteString("| ID | Username | Name | Email |\n")
		b.WriteString("|---|---|---|---|\n")
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
	fmt.Fprintf(&b, "- **Scopes**: %s\n", strings.Join(o.Scopes, ", "))
	fmt.Fprintf(&b, "- **User ID**: %d\n", o.UserID)
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, "- **Created**: %s\n", o.CreatedAt)
	}
	if o.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires**: %s\n", o.ExpiresAt)
	}
	if o.Token != "" {
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
		b.WriteString("| ID | Name | Active | Revoked | Scopes | Expires |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, t := range o.Tokens {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
				t.ID,
				toolutil.EscapeMdTableCell(t.Name),
				toolutil.BoolEmoji(t.Active),
				toolutil.BoolEmoji(t.Revoked),
				strings.Join(t.Scopes, ", "),
				t.ExpiresAt)
		}
	}
	return b.String()
}

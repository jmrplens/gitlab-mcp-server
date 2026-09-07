package projectserviceaccounts

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

// FormatMarkdownString renders a project service account as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Service Account: %s\n\n", toolutil.EscapeMdHeading(out.Username))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdUsername, toolutil.EscapeMdTableCell(out.Username))
	// GitLab validates an address with a regexp that excludes whitespace and a
	// second '@' and admits both '|' and '<'.
	fmt.Fprintf(&b, toolutil.FmtMdEmail, toolutil.EscapeMdTableCell(out.Email))
	if out.UnconfirmedEmail != "" {
		fmt.Fprintf(&b, "- **Unconfirmed email**: %s\n", toolutil.EscapeMdTableCell(out.UnconfirmedEmail))
	}
	toolutil.WriteHints(
		&b,
		"Use gitlab_project_service_account_update to modify this account",
		"Use gitlab_project_service_account_pat_create to create a token",
	)
	return b.String()
}

// FormatListMarkdownString renders a paginated list of project service accounts.
func FormatListMarkdownString(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Service Accounts (%d)\n\n", len(out.Accounts))
	toolutil.WriteListSummary(&b, len(out.Accounts), out.Pagination)
	if len(out.Accounts) == 0 {
		b.WriteString("No project service accounts found.\n")
		return b.String()
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email"))
	for _, account := range out.Accounts {
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n",
			account.ID,
			toolutil.EscapeMdTableCell(account.Username),
			toolutil.EscapeMdTableCell(account.Name),
			toolutil.EscapeMdTableCell(account.Email))
	}
	return b.String()
}

// FormatPATMarkdownString renders a project service account PAT as Markdown.
func FormatPATMarkdownString(out PATOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Service Account Token: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "- **Active**: %s\n", toolutil.BoolEmoji(out.Active))
	fmt.Fprintf(&b, "- **Revoked**: %s\n", toolutil.BoolEmoji(out.Revoked))
	//gitlab:allow-unescaped strings.Join(out.Scopes, ", "): GitLab validates a token request against the scopes it knows and rejects anything else, so a scope is one of that fixed set.
	fmt.Fprintf(&b, "- **Scopes**: %s\n", strings.Join(out.Scopes, ", "))
	fmt.Fprintf(&b, "- **User ID**: %d\n", out.UserID)
	if out.CreatedAt != "" {
		//gitlab:allow-unescaped out.CreatedAt: toPATOutput formats the time client-go parsed, so this is an RFC 3339 timestamp this server wrote.
		fmt.Fprintf(&b, toolutil.FmtMdCreated, out.CreatedAt)
	}
	if out.LastUsedAt != "" {
		//gitlab:allow-unescaped out.LastUsedAt: toPATOutput formats the time client-go parsed, so this is an RFC 3339 timestamp this server wrote.
		fmt.Fprintf(&b, "- **Last used**: %s\n", out.LastUsedAt)
	}
	if out.ExpiresAt != "" {
		//gitlab:allow-unescaped out.ExpiresAt: toPATOutput formats the date client-go parsed, so this is a YYYY-MM-DD date this server wrote.
		fmt.Fprintf(&b, "- **Expires**: %s\n", out.ExpiresAt)
	}
	if out.Token != "" {
		//gitlab:allow-unescaped out.Token: the instance generates this credential from a URL-safe alphabet, and it has to reach the caller byte for byte.
		fmt.Fprintf(&b, "- **Token**: `%s`\n", out.Token)
	}
	toolutil.WriteHints(
		&b,
		"Use gitlab_project_service_account_pat_rotate to rotate this token",
		"Use gitlab_project_service_account_pat_revoke to revoke this token",
	)
	return b.String()
}

// FormatListPATMarkdownString renders a paginated list of project service account PATs.
func FormatListPATMarkdownString(out ListPATOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Project Service Account Tokens (%d)\n\n", len(out.Tokens))
	toolutil.WriteListSummary(&b, len(out.Tokens), out.Pagination)
	if len(out.Tokens) == 0 {
		b.WriteString("No project service account tokens found.\n")
		return b.String()
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires"))
	//gitlab:allow-unescaped strings.Join(token.Scopes, ", "): GitLab validates a token request against the scopes it knows and rejects anything else, so a scope is one of that fixed set.
	//gitlab:allow-unescaped token.ExpiresAt: toPATOutput formats the date client-go parsed, so this is a YYYY-MM-DD date this server wrote.
	for _, token := range out.Tokens {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
			token.ID,
			toolutil.EscapeMdTableCell(token.Name),
			toolutil.BoolEmoji(token.Active),
			toolutil.BoolEmoji(token.Revoked),
			strings.Join(token.Scopes, ", "),
			token.ExpiresAt)
	}
	return b.String()
}

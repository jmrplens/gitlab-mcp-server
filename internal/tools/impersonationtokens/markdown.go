package impersonationtokens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatPATMarkdownString)
	toolutil.RegisterMarkdown(FormatRevokeMarkdownString)
}

// expiryCell renders a token's expiry for a table cell. A token GitLab sent no
// expiry for never expires, which is a fact about the token rather than a
// value the response was missing.
func expiryCell(expiresAt string) string {
	if expiresAt == "" {
		return "never"
	}
	return toolutil.FormatTime(expiresAt)
}

// FormatListMarkdownString renders a list of impersonation tokens as a table.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("impersonation tokens")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Impersonation Tokens", len(out.Tokens), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires At"))
	for _, t := range out.Tokens {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.BoolEmoji(t.Active),
			toolutil.BoolEmoji(t.Revoked),
			toolutil.EscapeMdTableCell(strings.Join(t.Scopes, ", ")),
			expiryCell(t.ExpiresAt),
		))
	}
	toolutil.WriteHints(
		&sb,
		toolutil.HintAction(actionImpersonationTokenGet, "read one of these tokens in full"),
		toolutil.HintAction(actionImpersonationTokenRevoke, "revoke one of these tokens"),
	)
	return sb.String()
}

// FormatMarkdownString renders one impersonation token as a card.
func FormatMarkdownString(out Output) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, fmt.Sprintf("Impersonation Token #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Bool("Active", out.Active)
	c.Warn("Revoked", out.Revoked)
	c.Field("Scopes", strings.Join(out.Scopes, ", "))
	c.Time("Expires At", out.ExpiresAt)
	c.Secret("Token", out.Token)
	c.End(
		toolutil.HintAction(actionImpersonationTokenRevoke, "revoke this token"),
	)
	return sb.String()
}

// FormatPATMarkdownString renders one personal access token as a card.
func FormatPATMarkdownString(out PATOutput) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, fmt.Sprintf("Personal Access Token #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Bool("Active", out.Active)
	c.Warn("Revoked", out.Revoked)
	c.Field("Scopes", strings.Join(out.Scopes, ", "))
	c.Text("Description", out.Description)
	c.Int("User ID", out.UserID)
	c.Time("Expires At", out.ExpiresAt)
	c.Secret("Token", out.Token)
	c.End()
	return sb.String()
}

// FormatRevokeMarkdownString renders a revocation confirmation.
func FormatRevokeMarkdownString(o RevokeOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Token Revoked")
	c.Int("User ID", o.UserID)
	c.Int("Token ID", o.TokenID)
	c.Bool("Revoked", o.Revoked)
	c.End(
		toolutil.HintAction(actionImpersonationTokenList, "list the tokens this user has left"),
	)
	return b.String()
}

package groupserviceaccounts

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	c := toolutil.NewCard(&b, "Service Account: "+o.Username)
	c.Int("ID", o.ID)
	c.Field("Name", o.Name)
	c.Field("Username", o.Username)
	c.Field("Email", o.Email)
	c.Field("Public Email", o.PublicEmail)
	c.Field("Unconfirmed Email", o.UnconfirmedEmail)
	c.End(
		toolutil.HintAction("group.service_account_update", "change this account's name or username"),
		toolutil.HintAction("group.service_account_pat_create", "create a token for it"),
	)
	return b.String()
}

// FormatListMarkdownString renders a paginated list of service accounts.
func FormatListMarkdownString(o ListOutput) string {
	if len(o.Accounts) == 0 {
		return toolutil.EmptyMessage("service accounts")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Service Accounts", len(o.Accounts), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email"))
	for _, a := range o.Accounts {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(a.ID, 10),
			toolutil.EscapeMdTableCell(a.Username),
			toolutil.EscapeMdTableCell(a.Name),
			toolutil.EscapeMdTableCell(a.Email),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction("group.service_account_pat_list", "list one account's tokens"),
	)
	return b.String()
}

// FormatPATMarkdownString renders a service account PAT as Markdown.
func FormatPATMarkdownString(o PATOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Personal Access Token: "+o.Name)
	c.Int("ID", o.ID)
	c.Bool("Active", o.Active)
	c.Warn("Revoked", o.Revoked)
	// Token scopes are one of GitLab's own fixed set: the instance refuses a
	// request naming anything else.
	c.Field("Scopes", strings.Join(o.Scopes, ", "))
	c.Bool("Granular", o.Granular)
	c.Int("User ID", o.UserID)
	c.Time("Created", o.CreatedAt)
	c.Time("Last Used", o.LastUsedAt)
	c.Time("Expires", o.ExpiresAt)
	c.Secret("Token", o.Token)
	writeGranularScopes(c, o.GranularScopes)
	c.End(
		toolutil.HintAction("group.service_account_pat_rotate", "rotate this token"),
		toolutil.HintAction("group.service_account_pat_revoke", "revoke it"),
	)
	return b.String()
}

// writeGranularScopes writes a granular token's scopes as a nested collection
// of the card. A granular token carries its permissions here and nowhere else,
// so a card that printed only the flat scopes said nothing about what it can
// actually reach.
func writeGranularScopes(c *toolutil.Card, scopes []toolutil.TokenGranularScopeOutput) {
	if len(scopes) == 0 {
		return
	}
	t := c.Table("Granular Scopes", "Access", "Permissions", "Project", "Group")
	for _, scope := range scopes {
		t.Row(
			toolutil.EscapeMdTableCell(scope.Access),
			toolutil.EscapeMdTableCell(strings.Join(scope.Permissions, ", ")),
			scopeNamespace(scope.ProjectID),
			scopeNamespace(scope.GroupID),
		)
	}
}

// scopeNamespace renders the project or group a granular scope is bound to.
// Exactly one of the two is set on any scope, so the other is a dash rather
// than a zero that reads as an identifier.
func scopeNamespace(id int64) string {
	if id == 0 {
		return "-"
	}
	return strconv.FormatInt(id, 10)
}

// FormatListPATMarkdownString renders a paginated list of PATs.
func FormatListPATMarkdownString(o ListPATOutput) string {
	if len(o.Tokens) == 0 {
		return toolutil.EmptyMessage("tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Service Account Tokens", len(o.Tokens), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires"))
	for _, t := range o.Tokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.BoolEmoji(t.Active),
			toolutil.BoolEmoji(t.Revoked),
			toolutil.EscapeMdTableCell(strings.Join(t.Scopes, ", ")),
			toolutil.FormatTime(t.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction("group.service_account_pat_revoke", "revoke one of these tokens"),
	)
	return b.String()
}

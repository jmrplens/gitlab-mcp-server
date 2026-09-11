package projectserviceaccounts

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

// FormatMarkdownString renders a project service account as Markdown.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Service Account: "+out.Username)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field("Username", out.Username)
	c.Field("Email", out.Email)
	c.Field("Public Email", out.PublicEmail)
	c.Field("Unconfirmed Email", out.UnconfirmedEmail)
	c.End(
		toolutil.HintAction("project.service_account_update", "modify this account"),
		toolutil.HintAction("project.service_account_pat_create", "create a token for it"),
	)
	return b.String()
}

// FormatListMarkdownString renders a paginated list of project service accounts.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Accounts) == 0 {
		return toolutil.EmptyMessage("project service accounts")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project Service Accounts", len(out.Accounts), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Username", "Name", "Email"))
	for _, account := range out.Accounts {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(account.ID, 10),
			toolutil.EscapeMdTableCell(account.Username),
			toolutil.EscapeMdTableCell(account.Name),
			toolutil.EscapeMdTableCell(account.Email),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction("project.service_account_pat_list", "list one account's tokens"),
	)
	return b.String()
}

// FormatPATMarkdownString renders a project service account PAT as Markdown.
func FormatPATMarkdownString(out PATOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Project Service Account Token: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Bool("Active", out.Active)
	c.Warn("Revoked", out.Revoked)
	// GitLab validates a token request against the scopes it knows and rejects
	// anything else, so a scope is one of that fixed set.
	c.Field("Scopes", strings.Join(out.Scopes, ", "))
	c.Bool("Granular", out.Granular)
	c.Int("User ID", out.UserID)
	c.Time("Created", out.CreatedAt)
	c.Time("Last used", out.LastUsedAt)
	c.Time("Expires", out.ExpiresAt)
	c.Secret("Token", out.Token)
	writeGranularScopes(c, out.GranularScopes)
	c.End(
		toolutil.HintAction("project.service_account_pat_rotate", "rotate this token"),
		toolutil.HintAction("project.service_account_pat_revoke", "revoke it"),
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

// FormatListPATMarkdownString renders a paginated list of project service account PATs.
func FormatListPATMarkdownString(out ListPATOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("project service account tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Project Service Account Tokens", len(out.Tokens), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires"))
	for _, token := range out.Tokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(token.ID, 10),
			toolutil.EscapeMdTableCell(token.Name),
			toolutil.BoolEmoji(token.Active),
			toolutil.BoolEmoji(token.Revoked),
			toolutil.EscapeMdTableCell(strings.Join(token.Scopes, ", ")),
			toolutil.FormatTime(token.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction("project.service_account_pat_revoke", "revoke one of these tokens"),
	)
	return b.String()
}

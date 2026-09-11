package accesstokens

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// granularScopeNamespace names what one granular scope reaches. GitLab sends
// project_id on a project scope and group_id on a group one, never both, so a
// scope that names neither renders as a dash rather than as an empty cell.
func granularScopeNamespace(scope toolutil.TokenGranularScopeOutput) string {
	switch {
	case scope.ProjectID != 0:
		return "project #" + strconv.FormatInt(scope.ProjectID, 10)
	case scope.GroupID != 0:
		return "group #" + strconv.FormatInt(scope.GroupID, 10)
	default:
		return "-"
	}
}

// writeGranularScopes writes the granular scopes a token carries as a nested
// collection under the card. A granular token whose scopes GitLab sent is the
// only thing that says what the token may actually do: the flat scope list
// says "api" for it and nothing about which project that reaches.
func writeGranularScopes(c *toolutil.Card, scopes []toolutil.TokenGranularScopeOutput) {
	if len(scopes) == 0 {
		return
	}
	table := c.Table("Granular Scopes", "Access", "Permissions", "Namespace")
	for _, scope := range scopes {
		table.Row(
			toolutil.EscapeMdTableCell(scope.Access),
			toolutil.EscapeMdTableCell(strings.Join(scope.Permissions, ", ")),
			toolutil.EscapeMdTableCell(granularScopeNamespace(scope)),
		)
	}
}

// FormatOutputMarkdown renders an access token as a card.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Access Token #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Text("Description", out.Description)
	c.Bool("Active", out.Active)
	c.Warn("Revoked", out.Revoked)
	c.Field("Scopes", strings.Join(out.Scopes, ", "))
	c.Bool("Granular", out.Granular)
	if out.AccessLevel != 0 {
		c.Field("Access Level", fmt.Sprintf("%s (%d)",
			toolutil.AccessLevelDescription(gl.AccessLevelValue(out.AccessLevel)), out.AccessLevel))
	}
	c.Time("Created", out.CreatedAt)
	c.Time("Last Used", out.LastUsedAt)
	c.Time("Expires", out.ExpiresAt)
	c.Secret("Token", out.Token)
	writeGranularScopes(c, out.GranularScopes)
	c.End(
		"Use `gitlab_project_access_token_revoke`, `gitlab_group_access_token_revoke`, or `gitlab_personal_access_token_revoke` to revoke this token from the matching scope",
		"Use `gitlab_project_access_token_rotate`, `gitlab_group_access_token_rotate`, or `gitlab_personal_access_token_rotate` to rotate this token from the matching scope",
	)
	return b.String()
}

// FormatListMarkdown renders a list of access tokens as a table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Tokens) == 0 {
		return toolutil.EmptyMessage("access tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Access Tokens", len(out.Tokens), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Active", "Revoked", "Scopes", "Expires"))
	for _, t := range out.Tokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.BoolEmoji(t.Active),
			toolutil.BoolEmoji(t.Revoked),
			toolutil.EscapeMdTableCell(strings.Join(t.Scopes, ", ")),
			expiryCell(t.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'get' with token_id for full details",
		"Use action 'create' to generate a new access token",
	)
	return b.String()
}

// expiryCell renders a token's expiry, or "never" for the token GitLab sent no
// expiry for, which is a fact about the token rather than a missing value.
func expiryCell(expiresAt string) string {
	if expiresAt == "" {
		return "never"
	}
	return toolutil.FormatTime(expiresAt)
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

package deploytokens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders one deploy token as a card.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Deploy Token: %s (ID: %d)", o.Name, o.ID))
	c.Int("ID", o.ID)
	c.Field("Name", o.Name)
	c.Field("Username", o.Username)
	c.Secret("Token", o.Token)
	c.Field("Scopes", strings.Join(o.Scopes, ", "))
	c.Warn("Revoked", o.Revoked)
	c.Warn("Expired", o.Expired)
	c.Time("Expires", o.ExpiresAt)
	c.End(
		"Use the selected tool surface's deploy-token get action with the matching scope (project or group) and deploy_token_id to fetch this deploy token before changing it",
		"Use the selected tool surface's deploy-token delete action with the matching scope (project or group), this deploy_token_id, and explicit confirm=true to revoke this deploy token",
	)
	return b.String()
}

// FormatListMarkdown renders a list of deploy tokens as a table.
func FormatListMarkdown(o ListOutput) string {
	if len(o.DeployTokens) == 0 {
		return toolutil.EmptyMessage("deploy tokens")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Deploy Tokens", len(o.DeployTokens), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Username", "Scopes", "Revoked", "Expired"))
	for _, t := range o.DeployTokens {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(t.Username),
			toolutil.EscapeMdTableCell(strings.Join(t.Scopes, ", ")),
			toolutil.BoolEmoji(t.Revoked),
			toolutil.BoolEmoji(t.Expired),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		"Use the selected tool surface's deploy-token get action with the matching scope (project or group) and deploy_token_id for full details",
		"Use the selected tool surface's deploy-token create action with the matching scope (project or group) to generate a new deploy token",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}

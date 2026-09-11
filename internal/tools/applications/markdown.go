package applications

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a list of OAuth applications as a table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Applications) == 0 {
		return toolutil.EmptyMessage("applications")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Applications", len(out.Applications), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "App ID", "Callback URL", "Confidential", "Scopes"))
	for _, a := range out.Applications {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(a.ID, 10),
			toolutil.EscapeMdTableCell(a.ApplicationName),
			toolutil.MdCodeSpanCell(a.ApplicationID),
			toolutil.EscapeMdTableCell(a.CallbackURL),
			toolutil.BoolEmoji(a.Confidential),
			toolutil.EscapeMdTableCell(strings.Join(a.Scopes, ", ")),
		))
	}
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		"Use `gitlab_create_application` to register a new application")
	return sb.String()
}

// formatApplicationDetail renders one application as a card. It is shared by
// the create and renew-secret formatters, which differ only in the heading,
// the label the secret row carries, and the closing hints; the advice to store
// the secret is added by the card itself, from the row that showed one.
func formatApplicationDetail(heading string, item ApplicationItem, secretLabel string, hints ...string) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, heading)
	c.Int("ID", item.ID)
	c.Field("Name", item.ApplicationName)
	c.Code("App ID", item.ApplicationID)
	c.Field("Callback URL", item.CallbackURL)
	c.Bool("Confidential", item.Confidential)
	c.Secret(secretLabel, item.Secret)
	c.Field("Scopes", strings.Join(item.Scopes, ", "))
	c.End(hints...)
	return sb.String()
}

// FormatCreateMarkdown renders a newly registered application.
func FormatCreateMarkdown(out CreateOutput) string {
	return formatApplicationDetail("Application Created", out.ApplicationItem, "Secret")
}

// FormatRenewSecretMarkdown renders an application whose secret was renewed.
func FormatRenewSecretMarkdown(out RenewSecretOutput) string {
	return formatApplicationDetail("Application Secret Renewed", out.ApplicationItem, "New Secret",
		"The previous secret is now invalid and any client using it must be updated")
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatCreateMarkdown)
	toolutil.RegisterMarkdown(FormatRenewSecretMarkdown)
}

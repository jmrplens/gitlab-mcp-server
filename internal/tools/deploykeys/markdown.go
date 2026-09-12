package deploykeys

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// expiryCell renders a deploy key's expiry for a table cell. A key GitLab sent
// no expiry for never expires, which is a fact about the key rather than a
// value the response was missing.
func expiryCell(expiresAt string) string {
	if expiresAt == "" {
		return "never"
	}
	return toolutil.FormatTime(expiresAt)
}

// FormatOutputMarkdown renders one project deploy key as a card.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	// A deploy key title is free text, and this server's own deploy_key.add and
	// deploy_key.update send it.
	c := toolutil.NewCard(&b, fmt.Sprintf("Deploy Key: %s (ID: %d)", o.Title, o.ID))
	c.Int("ID", o.ID)
	c.Field("Title", o.Title)
	c.Code("Fingerprint", o.Fingerprint)
	c.Code("SHA256", o.FingerprintSHA256)
	c.Bool("Can Push", o.CanPush)
	c.Field("Usage Type", o.UsageType)
	c.Time("Created", o.CreatedAt)
	c.Time("Expires", o.ExpiresAt)
	c.Time("Last Used", o.LastUsedAt)
	writeProjectAccessTables(c, o.ProjectsWithWriteAccess, o.ProjectsWithReadonlyAccess)
	c.End(
		"If the workflow asks to fetch/get this key before update or delete, use the selected tool surface's deploy-key get action with the same project_id and this deploy_key_id next",
		"Use the selected tool surface's deploy-key enable action with project_id and this deploy_key_id to grant this key to another project",
		"Use the selected tool surface's deploy-key delete action with the same project_id, this deploy_key_id, and explicit confirm=true to remove this deploy key",
	)
	return b.String()
}

// FormatListMarkdown renders a list of project deploy keys as a table.
func FormatListMarkdown(o ListOutput) string {
	if len(o.DeployKeys) == 0 {
		return toolutil.EmptyMessage("deploy keys")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Deploy Keys", len(o.DeployKeys), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Title", "Can Push", "Fingerprint", "Created", "Expires"))
	for _, k := range o.DeployKeys {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(k.ID, 10),
			toolutil.EscapeMdTableCell(k.Title),
			toolutil.BoolEmoji(k.CanPush),
			toolutil.MdCodeSpanCell(k.Fingerprint),
			toolutil.FormatTime(k.CreatedAt),
			expiryCell(k.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		"Use the selected tool surface's deploy-key get action with the same project_id and deploy_key_id for full details",
		"Use the selected tool surface's deploy-key add action with project_id to create a new deploy key",
	)
	return b.String()
}

// FormatInstanceOutputMarkdown renders one instance deploy key as a card.
func FormatInstanceOutputMarkdown(o InstanceOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Instance Deploy Key: %s (ID: %d)", o.Title, o.ID))
	c.Int("ID", o.ID)
	c.Field("Title", o.Title)
	c.Code("Fingerprint", o.Fingerprint)
	c.Code("SHA256", o.FingerprintSHA256)
	c.Time("Created", o.CreatedAt)
	c.Time("Expires", o.ExpiresAt)
	c.Time("Last Used", o.LastUsedAt)
	c.Field("Usage Type", o.UsageType)
	writeProjectAccessTables(c, o.ProjectsWithWriteAccess, o.ProjectsWithReadonlyAccess)
	c.End(
		"Use the selected tool surface's deploy-key enable action with project_id and this deploy_key_id to grant this instance key to a project",
		"Use the selected tool surface's deploy-key list action with project_id to verify project-level references before deletion workflows",
	)
	return b.String()
}

// writeProjectAccessTables writes one nested collection per non-empty access
// list, which a deploy key carries when the request asked for the projects it
// reaches.
func writeProjectAccessTables(c *toolutil.Card, write, readonly []ProjectSummary) {
	for _, section := range []struct {
		heading  string
		projects []ProjectSummary
	}{
		{heading: "Projects with Write Access", projects: write},
		{heading: "Projects with Readonly Access", projects: readonly},
	} {
		if len(section.projects) == 0 {
			continue
		}
		table := c.Table(section.heading, "ID", "Name", "Path")
		for _, p := range section.projects {
			table.Row(
				strconv.FormatInt(p.ID, 10),
				toolutil.EscapeMdTableCell(p.Name),
				toolutil.EscapeMdTableCell(p.PathWithNamespace),
			)
		}
	}
}

// FormatInstanceListMarkdown renders a list of instance deploy keys as a table.
func FormatInstanceListMarkdown(o InstanceListOutput) string {
	if len(o.DeployKeys) == 0 {
		return toolutil.EmptyMessage("instance deploy keys")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Instance Deploy Keys", len(o.DeployKeys), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Title", "Fingerprint", "Created", "Expires"))
	for _, k := range o.DeployKeys {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(k.ID, 10),
			toolutil.EscapeMdTableCell(k.Title),
			toolutil.MdCodeSpanCell(k.Fingerprint),
			toolutil.FormatTime(k.CreatedAt),
			expiryCell(k.ExpiresAt),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		"Use the selected tool surface's deploy-key enable action with project_id and deploy_key_id to grant one of these keys to a project",
		"Use the selected tool surface's deploy-key list action with project_id to inspect project-level deploy key metadata",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceOutputMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceListMarkdown)
}

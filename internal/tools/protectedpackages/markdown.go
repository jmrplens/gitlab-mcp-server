package protectedpackages

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single package protection rule as the card
// of one object: the pattern it matches and the access levels it demands.
func FormatOutputMarkdown(r Output) string {
	if r.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Package Protection Rule #%d", r.ID))
	// The pattern is free text this server's own create action passes through,
	// and a reader has to copy it exactly, so it is a code span rather than an
	// escaped cell: an entity inside a span renders as its own characters.
	c.Code("Pattern", r.PackageNamePattern)
	c.Field("Package Type", r.PackageType)
	c.Field("Min Push Level", r.MinimumAccessLevelForPush)
	c.Field("Min Delete Level", r.MinimumAccessLevelForDelete)
	c.End(
		"Use `gitlab_update_package_protection_rule` to modify this rule",
		"Use `gitlab_delete_package_protection_rule` to remove it",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of package protection rules as
// a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("package protection rules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Package Protection Rules", len(out.Rules), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Pattern", "Type", "Min Push", "Min Delete"))
	for _, r := range out.Rules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.MdCodeSpanCell(r.PackageNamePattern),
			toolutil.EscapeMdTableCell(r.PackageType),
			toolutil.EscapeMdTableCell(r.MinimumAccessLevelForPush),
			toolutil.EscapeMdTableCell(r.MinimumAccessLevelForDelete),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use `gitlab_create_package_protection_rule` to add a new rule")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown) // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)   // ListOutput
}

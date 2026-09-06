package containerregistry

import (
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatRepositoryMarkdown formats a single registry repository as markdown.
func FormatRepositoryMarkdown(out RepositoryOutput) string {
	heading := out.Path
	if heading == "" {
		heading = out.Name
	}
	var b strings.Builder
	// The name, the path and the location a path is built into are the image
	// name whoever pushed it chose. What holds them to a safe character set is
	// the OCI reference grammar, which a separate service enforces and this one
	// neither sees nor models.
	fmt.Fprintf(&b, "## Registry Repository: %s\n\n", toolutil.EscapeMdHeading(heading))
	fmt.Fprint(&b, toolutil.TblFieldValue)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Path | %s |\n", toolutil.EscapeMdTableCell(out.Path))
	fmt.Fprintf(&b, "| Location | %s |\n", toolutil.EscapeMdTableCell(out.Location))
	fmt.Fprintf(&b, "| Tags Count | %d |\n", out.TagsCount)
	if out.Status != "" {
		//gitlab:allow-unescaped out.Status: a container repository status, a gl.ContainerRegistryStatus GitLab picks from a fixed set (delete_scheduled, delete_failed, delete_ongoing).
		fmt.Fprintf(&b, "| Status | %s |\n", out.Status)
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(
		&b,
		"Use action 'registry_tag_list' to list tags in this repository",
		"Use action 'registry_delete' to delete this repository",
	)
	return b.String()
}

// FormatRepositoryListMarkdown formats a list of registry repositories.
func FormatRepositoryListMarkdown(out RepositoryListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Registry Repositories (%d)\n\n", len(out.Repositories))
	toolutil.WriteListSummary(&b, len(out.Repositories), out.Pagination)
	if len(out.Repositories) == 0 {
		b.WriteString("No registry repositories found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Path", "Tags Count"))
	for _, r := range out.Repositories {
		fmt.Fprintf(&b, "| %s | %s | %d |\n",
			toolutil.EscapeMdTableCell(r.Name), toolutil.EscapeMdTableCell(r.Path), r.TagsCount)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'registry_get' with repository_id for full details",
	)
	return b.String()
}

// FormatTagMarkdown formats a single registry tag as markdown.
func FormatTagMarkdown(out TagOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Registry Tag: %s\n\n", toolutil.EscapeMdHeading(out.Name))
	fmt.Fprint(&b, toolutil.TblFieldValue)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Path | %s |\n", toolutil.EscapeMdTableCell(out.Path))
	fmt.Fprintf(&b, "| Location | %s |\n", toolutil.EscapeMdTableCell(out.Location))
	if out.Digest != "" {
		//gitlab:allow-unescaped out.Digest: an image manifest digest, an algorithm name and hexadecimal digits.
		fmt.Fprintf(&b, "| Digest | %s |\n", out.Digest)
	}
	if out.Revision != "" {
		//gitlab:allow-unescaped out.Revision: an image revision, hexadecimal digits only.
		fmt.Fprintf(&b, "| Revision | %s |\n", out.Revision)
	}
	fmt.Fprintf(&b, "| Total Size | %d |\n", out.TotalSize)
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	toolutil.WriteHints(
		&b,
		"Use action 'registry_tag_delete' to remove this tag",
	)
	return b.String()
}

// FormatTagListMarkdown formats a list of registry tags.
func FormatTagListMarkdown(out TagListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Registry Tags (%d)\n\n", len(out.Tags))
	toolutil.WriteListSummary(&b, len(out.Tags), out.Pagination)
	if len(out.Tags) == 0 {
		b.WriteString("No registry tags found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Path", "Total Size"))
	for _, t := range out.Tags {
		fmt.Fprintf(&b, "| %s | %s | %d |\n",
			toolutil.EscapeMdTableCell(t.Name), toolutil.EscapeMdTableCell(t.Path), t.TotalSize)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'registry_tag_get' with tag name for full details",
		"Use action 'registry_tag_delete_bulk' to clean up old tags",
	)
	return b.String()
}

// FormatProtectionRuleMarkdown formats a single protection rule as markdown.
func FormatProtectionRuleMarkdown(out ProtectionRuleOutput) string {
	var b strings.Builder
	// The pattern is free text this server's own registry_rule_create and
	// registry_rule_update pass through with no validation of their own.
	fmt.Fprintf(&b, "## Protection Rule: %s\n\n", toolutil.EscapeMdHeading(out.RepositoryPathPattern))
	fmt.Fprint(&b, toolutil.TblFieldValue)
	fmt.Fprintf(&b, "| Repository Path Pattern | %s |\n", toolutil.EscapeMdTableCell(out.RepositoryPathPattern))
	//gitlab:allow-unescaped out.MinimumAccessLevelForPush: a protection rule access level, a gl.ProtectionRuleAccessLevel GitLab picks from a fixed set (maintainer, owner, admin).
	fmt.Fprintf(&b, "| Min Access Level (Push) | %s |\n", out.MinimumAccessLevelForPush)
	//gitlab:allow-unescaped out.MinimumAccessLevelForDelete: a protection rule access level, a gl.ProtectionRuleAccessLevel GitLab picks from a fixed set (maintainer, owner, admin).
	fmt.Fprintf(&b, "| Min Access Level (Delete) | %s |\n", out.MinimumAccessLevelForDelete)
	toolutil.WriteHints(
		&b,
		"Use action 'registry_rule_update' to modify access levels",
		"Use action 'registry_rule_delete' to remove this rule",
	)
	return b.String()
}

// FormatProtectionRuleListMarkdown formats a list of protection rules.
func FormatProtectionRuleListMarkdown(out ProtectionRuleListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Protection Rules (%d)\n\n", len(out.Rules))
	toolutil.WriteListSummary(&b, len(out.Rules), out.Pagination)
	if len(out.Rules) == 0 {
		b.WriteString("No protection rules found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString(toolutil.MarkdownTableHeader("Pattern", "Min Push", "Min Delete"))
	for _, r := range out.Rules {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			//gitlab:allow-unescaped r.MinimumAccessLevelForPush: a protection rule access level, a gl.ProtectionRuleAccessLevel GitLab picks from a fixed set (maintainer, owner, admin).
			//gitlab:allow-unescaped r.MinimumAccessLevelForDelete: a protection rule access level, a gl.ProtectionRuleAccessLevel GitLab picks from a fixed set (maintainer, owner, admin).
			toolutil.EscapeMdTableCell(r.RepositoryPathPattern), r.MinimumAccessLevelForPush, r.MinimumAccessLevelForDelete)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'registry_rule_create' to add a new rule",
	)
	return b.String()
}

// FormatTagProtectionRuleMarkdown formats a single tag protection rule as markdown.
func FormatTagProtectionRuleMarkdown(out TagProtectionRuleOutput) string {
	var b strings.Builder
	// An RE2 pattern a person types, where '|' is ordinary alternation, so
	// "v.+|latest" would end the heading's own line without it.
	fmt.Fprintf(&b, "## Tag Protection Rule: %s\n\n", toolutil.EscapeMdHeading(out.TagNamePattern))
	fmt.Fprint(&b, toolutil.TblFieldValue)
	fmt.Fprintf(&b, "| Tag Name Pattern | %s |\n", toolutil.EscapeMdTableCell(out.TagNamePattern))
	//gitlab:allow-unescaped protectionAccessLabel(out.MinimumAccessLevelForPush): the same access level through a helper whose only other result is the constant "immutable".
	fmt.Fprintf(&b, "| Min Access Level (Push) | %s |\n", protectionAccessLabel(out.MinimumAccessLevelForPush))
	//gitlab:allow-unescaped protectionAccessLabel(out.MinimumAccessLevelForDelete): the same access level through a helper whose only other result is the constant "immutable".
	fmt.Fprintf(&b, "| Min Access Level (Delete) | %s |\n", protectionAccessLabel(out.MinimumAccessLevelForDelete))
	toolutil.WriteHints(
		&b,
		"Use action 'registry_tag_rule_update' to modify access levels",
		"Use action 'registry_tag_rule_delete' to remove this rule",
	)
	return b.String()
}

// FormatTagProtectionRuleListMarkdown formats a list of tag protection rules.
func FormatTagProtectionRuleListMarkdown(out TagProtectionRuleListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Tag Protection Rules (%d)\n\n", len(out.Rules))
	toolutil.WriteListSummary(&b, len(out.Rules), out.Pagination)
	if len(out.Rules) == 0 {
		b.WriteString("No tag protection rules found.\n")
		toolutil.WritePagination(&b, out.Pagination)
		return b.String()
	}
	b.WriteString(toolutil.MarkdownTableHeader("Tag Pattern", "Min Push", "Min Delete"))
	for _, r := range out.Rules {
		fmt.Fprintf(&b, "| %s | %s | %s |\n",
			//gitlab:allow-unescaped protectionAccessLabel(r.MinimumAccessLevelForPush): the same access level through a helper whose only other result is the constant "immutable".
			//gitlab:allow-unescaped protectionAccessLabel(r.MinimumAccessLevelForDelete): the same access level through a helper whose only other result is the constant "immutable".
			toolutil.EscapeMdTableCell(r.TagNamePattern), protectionAccessLabel(r.MinimumAccessLevelForPush), protectionAccessLabel(r.MinimumAccessLevelForDelete))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		"Use action 'registry_tag_rule_create' to add a new rule",
		"These rules protect image *tags*; use action 'registry_rule_list' for repository-path protection rules",
	)
	return b.String()
}

// protectionAccessLabel renders an empty minimum access level as "immutable",
// which is how the GitLab API expresses a rule that forbids push and delete
// for everyone.
func protectionAccessLabel(level gl.ProtectionRuleAccessLevel) string {
	if level == "" {
		return "immutable"
	}
	return string(level)
}

func init() {
	toolutil.RegisterMarkdown(FormatRepositoryMarkdown)
	toolutil.RegisterMarkdown(FormatRepositoryListMarkdown)
	toolutil.RegisterMarkdown(FormatTagMarkdown)
	toolutil.RegisterMarkdown(FormatTagListMarkdown)
	toolutil.RegisterMarkdown(FormatProtectionRuleMarkdown)
	toolutil.RegisterMarkdown(FormatProtectionRuleListMarkdown)
	toolutil.RegisterMarkdown(FormatTagProtectionRuleMarkdown)
	toolutil.RegisterMarkdown(FormatTagProtectionRuleListMarkdown)
}

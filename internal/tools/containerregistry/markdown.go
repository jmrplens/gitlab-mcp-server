package containerregistry

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatRepositoryMarkdown renders one registry repository as the card of one
// object.
func FormatRepositoryMarkdown(out RepositoryOutput) string {
	name := out.Path
	if name == "" {
		name = out.Name
	}
	var b strings.Builder
	// The name, the path and the location a path is built into are the image
	// name whoever pushed it chose. What holds them to a safe character set is
	// the OCI reference grammar, which a separate service enforces and this one
	// neither sees nor models.
	c := toolutil.NewCard(&b, repositoryHeading(name))
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field("Path", out.Path)
	c.Field("Location", out.Location)
	// The tag count is sent only when the caller asked for it, so a zero is
	// GitLab saying nothing rather than a repository with no tags; Count is
	// the row that writes nothing for it.
	c.Count("Tags Count", out.TagsCount)
	c.Field("Status", string(out.Status))
	c.Time("Created At", out.CreatedAt)
	c.End(
		"Use action 'registry_tag_list' to list tags in this repository",
		"Use action 'registry_delete' to delete this repository",
	)
	return b.String()
}

// repositoryHeading names the card after the image when GitLab sent a name,
// and after the resource alone when it did not, so the heading never ends in
// a colon with nothing behind it.
func repositoryHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Registry Repository"
	}
	return "Registry Repository: " + name
}

// FormatRepositoryListMarkdown formats a list of registry repositories.
func FormatRepositoryListMarkdown(out RepositoryListOutput) string {
	if len(out.Repositories) == 0 {
		return toolutil.EmptyMessage("registry repositories")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Registry Repositories", len(out.Repositories), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path", "Tags Count"))
	for _, r := range out.Repositories {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.EscapeMdTableCell(r.Path),
			countCell(r.TagsCount),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'registry_get' with repository_id for full details")
	return b.String()
}

// countCell renders a count GitLab sends only when it was asked for: a dash
// says nothing was reported, where a bare 0 would claim the repository has no
// tags. Distinguishing an absent count from a real zero needs the field to be
// optional in the output type, which is a surface change this does not make.
func countCell(count int64) string {
	if count == 0 {
		return "-"
	}
	return strconv.FormatInt(count, 10)
}

// FormatTagMarkdown renders one registry tag as the card of one object.
func FormatTagMarkdown(out TagOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, tagHeading(out.Name))
	c.Field("Name", out.Name)
	c.Field("Path", out.Path)
	c.Field("Location", out.Location)
	// The digest and the revision are values a reader copies character by
	// character, so each is a code span sized to its content.
	c.Code("Digest", out.Digest)
	c.Code("Revision", out.Revision)
	c.Code("Short Revision", out.ShortRevision)
	c.Field("Total Size", fmt.Sprintf("%d bytes", out.TotalSize))
	c.Time("Created At", out.CreatedAt)
	c.End("Use action 'registry_tag_delete' to remove this tag")
	return b.String()
}

// tagHeading names the card after the tag when GitLab sent a name, and after
// the resource alone when it did not.
func tagHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Registry Tag"
	}
	return "Registry Tag: " + name
}

// FormatTagListMarkdown formats a list of registry tags.
func FormatTagListMarkdown(out TagListOutput) string {
	if len(out.Tags) == 0 {
		return toolutil.EmptyMessage("registry tags")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Registry Tags", len(out.Tags), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Path", "Total Size (bytes)"))
	for _, t := range out.Tags {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(t.Path),
			strconv.FormatInt(t.TotalSize, 10),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'registry_tag_get' with tag name for full details",
		"Use action 'registry_tag_delete_bulk' to clean up old tags")
	return b.String()
}

// FormatProtectionRuleMarkdown renders one repository-path protection rule as
// the card of one object.
func FormatProtectionRuleMarkdown(out ProtectionRuleOutput) string {
	var b strings.Builder
	// The pattern is free text this server's own registry_rule_create and
	// registry_rule_update pass through with no validation of their own.
	c := toolutil.NewCard(&b, protectionRuleHeading("Protection Rule", out.RepositoryPathPattern))
	c.Int("ID", out.ID)
	c.Code("Repository Path Pattern", out.RepositoryPathPattern)
	c.Field("Min Access Level (Push)", string(out.MinimumAccessLevelForPush))
	c.Field("Min Access Level (Delete)", string(out.MinimumAccessLevelForDelete))
	c.End(
		"Use action 'registry_rule_update' to modify access levels",
		"Use action 'registry_rule_delete' to remove this rule",
	)
	return b.String()
}

// protectionRuleHeading names a rule card after the pattern it matches, and
// after the resource alone when GitLab sent none.
func protectionRuleHeading(resource, pattern string) string {
	if strings.TrimSpace(pattern) == "" {
		return resource
	}
	return resource + ": " + pattern
}

// FormatProtectionRuleListMarkdown formats a list of protection rules.
//
// GitLab sends no pagination headers for this endpoint, so the heading counts
// what is shown rather than a total nobody reported.
func FormatProtectionRuleListMarkdown(out ProtectionRuleListOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("protection rules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Protection Rules", len(out.Rules), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Pattern", "Min Push", "Min Delete"))
	for _, r := range out.Rules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.MdCodeSpanCell(r.RepositoryPathPattern),
			toolutil.EscapeMdTableCell(string(r.MinimumAccessLevelForPush)),
			toolutil.EscapeMdTableCell(string(r.MinimumAccessLevelForDelete)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'registry_rule_create' to add a new rule")
	return b.String()
}

// FormatTagProtectionRuleMarkdown renders one tag protection rule as the card
// of one object.
func FormatTagProtectionRuleMarkdown(out TagProtectionRuleOutput) string {
	var b strings.Builder
	// An RE2 pattern a person types, where '|' is ordinary alternation, so
	// "v.+|latest" would end the heading's own line without it.
	c := toolutil.NewCard(&b, protectionRuleHeading("Tag Protection Rule", out.TagNamePattern))
	c.Int("ID", out.ID)
	c.Code("Tag Name Pattern", out.TagNamePattern)
	c.Field("Min Access Level (Push)", protectionAccessLabel(out.MinimumAccessLevelForPush))
	c.Field("Min Access Level (Delete)", protectionAccessLabel(out.MinimumAccessLevelForDelete))
	c.End(
		"Use action 'registry_tag_rule_update' to modify access levels",
		"Use action 'registry_tag_rule_delete' to remove this rule",
	)
	return b.String()
}

// FormatTagProtectionRuleListMarkdown formats a list of tag protection rules.
func FormatTagProtectionRuleListMarkdown(out TagProtectionRuleListOutput) string {
	if len(out.Rules) == 0 {
		return toolutil.EmptyMessage("tag protection rules")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Tag Protection Rules", len(out.Rules), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Tag Pattern", "Min Push", "Min Delete"))
	for _, r := range out.Rules {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.MdCodeSpanCell(r.TagNamePattern),
			toolutil.EscapeMdTableCell(protectionAccessLabel(r.MinimumAccessLevelForPush)),
			toolutil.EscapeMdTableCell(protectionAccessLabel(r.MinimumAccessLevelForDelete)),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'registry_tag_rule_create' to add a new rule",
		"These rules protect image *tags*; use action 'registry_rule_list' for repository-path protection rules")
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

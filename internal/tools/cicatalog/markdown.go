package cicatalog

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name.
const (
	actionCatalogGetHint  = "ci_catalog.get"
	actionCatalogListHint = "ci_catalog.list"
	actionLintHint        = "template.lint"
)

// descriptionCellRunes is how much of a resource description one table cell
// carries before it is cut.
const descriptionCellRunes = 60

// FormatListMarkdown renders a page of catalog resources as a Markdown table.
//
// The name is not linked. GitLab's catalog query answers with webPath, a path
// relative to the instance root ("/explore/catalog/group/project"), and this
// package never learns which instance answered, so a link built from it
// resolved against whatever base the reading client happened to have. The path
// is shown as the value it is, beside the full path the get action takes.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Resources) == 0 {
		return toolutil.EmptyMessage("catalog resources")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "CI/CD Catalog Resources", len(out.Resources), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader(
		"Name", "Path", "Description", "Stars", "Usage (30d)", "Verification", "Latest Version", "Released",
	))
	for _, r := range out.Resources {
		b.WriteString(toolutil.MarkdownTableRow(
			resourceNameCell(r.Name, r.Archived),
			toolutil.MdCodeSpanCell(r.FullPath),
			toolutil.EscapeMdTableCell(truncateRunes(r.Description, descriptionCellRunes)),
			strconv.Itoa(r.StarCount),
			strconv.Itoa(r.Last30DayUsageCount),
			toolutil.EscapeMdTableCell(r.VerificationLevel),
			toolutil.EscapeMdTableCell(r.LatestVersionName),
			toolutil.FormatTime(r.LatestReleasedAt),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Resources))
	toolutil.WriteHints(&b,
		toolutil.HintAction(actionCatalogGetHint, "see one resource with its components and inputs"),
		toolutil.HintAction(actionLintHint, "check a configuration that includes one"),
	)
	return b.String()
}

// resourceNameCell carries the archived marker, without which a reader cannot
// tell a resource nobody maintains any more from a live one.
func resourceNameCell(name string, archived bool) string {
	cell := toolutil.EscapeMdTableCell(name)
	if archived {
		cell += " " + toolutil.EmojiArchived
	}
	return cell
}

// FormatGetMarkdown renders one catalog resource as the card of a single
// object: its own fields, then its components and released versions as the
// nested collections they are.
func FormatGetMarkdown(out GetOutput) string {
	r := out.Resource
	var b strings.Builder
	c := toolutil.NewCard(&b, catalogHeading(r))
	writeCatalogResourceSummary(c, r)
	writeCatalogResourceComponents(c, r.Components)
	writeCatalogResourceVersions(c, r.Versions)
	c.End(
		toolutil.HintAction(actionLintHint, "check a configuration that includes this component"),
		toolutil.HintAction(actionCatalogListHint, "browse the catalog for others"),
	)
	return b.String()
}

// catalogHeading names the resource and marks an archived one.
func catalogHeading(r ResourceDetail) string {
	heading := "Catalog Resource: " + r.Name
	if r.Archived {
		heading += " " + toolutil.EmojiArchived
	}
	return heading
}

func writeCatalogResourceSummary(c *toolutil.Card, r ResourceDetail) {
	c.Code("ID", r.ID)
	c.Code("Full Path", r.FullPath)
	// A relative path, shown as the path it is rather than linked: see
	// FormatListMarkdown.
	c.Code("Web Path", r.WebPath)
	c.Count("Stars", int64(r.StarCount))
	c.Count("Usage (30d)", int64(r.Last30DayUsageCount))
	c.Field("Verification", r.VerificationLevel)
	c.Field("Visibility", r.VisibilityLevel)
	c.Field("Topics", strings.Join(r.Topics, ", "))
	c.Flag(toolutil.EmojiArchived, "Archived", r.Archived)
	c.Time("Latest Release", r.LatestReleasedAt)
	c.Field("Latest Version", r.LatestVersionName)
	// The description is whatever the resource's maintainer typed, so it is
	// quoted rather than written into the response as Markdown of its own.
	c.Text("Description", r.Description)
}

func writeCatalogResourceComponents(c *toolutil.Card, components []ComponentItem) {
	if len(components) == 0 {
		return
	}
	section := c.Section("Components (Latest Version)")
	for _, component := range components {
		writeCatalogResourceComponent(section, component)
	}
}

func writeCatalogResourceComponent(section *toolutil.Card, component ComponentItem) {
	card := section.Section(component.Name)
	card.Text("Description", component.Description)
	card.Code("Include", component.IncludePath)
	if len(component.Inputs) == 0 {
		return
	}
	table := card.Table("", "Input", "Type", "Required", "Default", "Description")
	for _, input := range component.Inputs {
		table.Row(
			toolutil.MdCodeSpanCell(input.Name),
			toolutil.EscapeMdTableCell(input.Type),
			toolutil.BoolEmoji(input.Required),
			toolutil.EscapeMdTableCell(input.Default),
			toolutil.EscapeMdTableCell(input.Description),
		)
	}
}

func writeCatalogResourceVersions(c *toolutil.Card, versions []VersionItem) {
	if len(versions) == 0 {
		return
	}
	table := c.Table("Released Versions", "Version", "Released", "Components")
	for _, version := range versions {
		table.Row(
			toolutil.EscapeMdTableCell(version.Name),
			toolutil.FormatTime(version.ReleasedAt),
			toolutil.EscapeMdTableCell(strings.Join(catalogVersionComponentNames(version), ", ")),
		)
	}
}

func catalogVersionComponentNames(version VersionItem) []string {
	names := make([]string, 0, len(version.Components))
	for _, component := range version.Components {
		names = append(names, component.Name)
	}
	return names
}

// truncateRunes shortens s to maxRunes characters, appending "..." when it
// cut anything. It counts runes rather than bytes: cutting a UTF-8 sequence in
// half leaves a replacement character in the middle of a description, and a
// description is the one field of a catalog resource most likely to be
// written in a language that needs more than one byte per character.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}

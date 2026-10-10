package cicatalog

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// descriptionCellRunes is how much of a resource description one table cell
// carries before it is cut.
const descriptionCellRunes = 60

// usage30dLabel names the thirty-day usage count wherever a resource, a version
// or a component shows it, so the list column and the card rows read alike.
const usage30dLabel = "Usage (30d)"

// FormatListMarkdown renders a page of catalog resources as a Markdown table.
//
// The name is not linked. GitLab's catalog query answers with webPath, a path
// relative to the instance root ("/explore/catalog/group/project"), and this
// package never learns which instance answered, so a link built from it
// resolved against whatever base the reading client happened to have. The path
// is shown as the value it is, beside the full path the get action takes.
//
// A page holding items GitLab answered as null counts them in its heading
// beside the ones shown and keeps its cursor, a page that shows none of its
// items included: such a page is neither the whole catalog nor its end.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Resources) == 0 && out.HiddenItems == 0 {
		return toolutil.EmptyMessage("catalog resources")
	}
	var b strings.Builder
	writeListHeading(&b, len(out.Resources), out.HiddenItems)
	var hints []string
	if len(out.Resources) > 0 {
		writeResourceTable(&b, out.Resources)
		hints = append(hints,
			toolutil.HintAction(actionCatalogGet, "see one resource with its components and inputs"),
			toolutil.HintAction(actionTemplateLint, "check a configuration that includes one"),
		)
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Resources))
	if out.HiddenItems > 0 {
		hints = append(hints, hiddenItemsHint(out.HiddenItems))
	}
	toolutil.WriteHints(&b, hints...)
	return b.String()
}

// listTitle is the heading of a page of catalog resources.
const listTitle = "CI/CD Catalog Resources"

// writeListHeading writes the page's heading: the count shown, and beside it
// the count hidden when GitLab answered any item as null.
func writeListHeading(b *strings.Builder, shown, hidden int) {
	if hidden == 0 {
		toolutil.WriteListHeading(b, listTitle, shown, toolutil.PaginationOutput{})
		return
	}
	fmt.Fprintf(b, "## %s (%d shown, %d hidden)\n\n", listTitle, shown, hidden)
}

// writeResourceTable writes one row per resource shown.
func writeResourceTable(b *strings.Builder, resources []ResourceItem) {
	b.WriteString(toolutil.MarkdownTableHeader(
		"Name", "Path", "Description", "Stars", usage30dLabel, "Verification", "Latest Version", "Released",
	))
	for _, r := range resources {
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
}

// hiddenItemsHint says how many items of a page GitLab answered as null and
// why, so a short or empty page is not read as all the catalog holds (issue
// 1103). It names no GitLab version: which types a fine-grained token is
// refused changes with every release, and the null is how each one answers.
func hiddenItemsHint(hidden int) string {
	return fmt.Sprintf("GitLab answered %d of the items on this page as null, with no error, which is how it answers an item the credential may not read, "+
		"a catalog resource whose GraphQL type a fine-grained personal access token is not granted among them. "+
		"They are left out, so this page shows fewer items than GitLab returned.", hidden)
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
	// The README is the maintainer's Markdown, fenced so it reads as the
	// document it is rather than as headings and lists of this response.
	c.Fence("README (Latest Version)", "markdown", r.Readme)
	c.End(
		toolutil.HintAction(actionTemplateLint, "check a configuration that includes this component"),
		toolutil.HintAction(actionCatalogList, "browse the catalog for others"),
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
	// A relative path too, for the same reason as the web path.
	c.Code("Starrers Path", r.StarrersPath)
	c.Count(usage30dLabel, int64(r.Last30DayUsageCount))
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
	card.Code("ID", component.ID)
	card.Text("Description", component.Description)
	card.Code("Include", component.IncludePath)
	// Nil is a count GitLab did not send; zero is a component nobody used.
	if component.Last30DayUsageCount != nil {
		card.Int(usage30dLabel, int64(*component.Last30DayUsageCount))
	}
	if len(component.Inputs) == 0 {
		return
	}
	table := card.Table("", "Input", "Type", "Required", "Default", "Options", "Regex", "Description")
	var rules []ruleRow
	for _, input := range component.Inputs {
		table.Row(
			toolutil.MdCodeSpanCell(input.Name),
			toolutil.EscapeMdTableCell(input.Type),
			toolutil.BoolEmoji(input.Required),
			toolutil.EscapeMdTableCell(inputValueText(input.Default)),
			toolutil.EscapeMdTableCell(inputValueText(input.Options)),
			toolutil.MdCodeSpanCell(input.Regex),
			toolutil.EscapeMdTableCell(input.Description),
		)
		for _, rule := range input.Rules {
			rules = append(rules, ruleRow{input: input.Name, rule: rule})
		}
	}
	if len(rules) == 0 {
		return
	}
	ruleTable := card.Table("Input Rules", "Input", "If", "Default", "Options")
	for _, row := range rules {
		ruleTable.Row(
			toolutil.MdCodeSpanCell(row.input),
			toolutil.MdCodeSpanCell(row.rule.If),
			toolutil.EscapeMdTableCell(inputValueText(row.rule.Default)),
			toolutil.EscapeMdTableCell(ruleOptionsText(row.rule.Options)),
		)
	}
}

// ruleOptionsText renders the options a rule offers, and nothing for a rule
// that offers none: a nil list would reach inputValueText as a typed nil
// inside the interface, which is not the nil its first case catches, and read
// as JSON null.
func ruleOptionsText(options []any) string {
	if len(options) == 0 {
		return ""
	}
	return inputValueText(options)
}

// ruleRow is one conditional rule and the input it belongs to.
type ruleRow struct {
	input string
	rule  InputRule
}

// inputValueText renders an input's default or options as text for a cell:
// a string as itself, nothing for a value GitLab did not send, and any other
// value as the JSON it arrived as, so a boolean reads false and a list reads
// as the list it is.
func inputValueText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		// Nothing decoded from a GitLab answer lands here: it is only ever a
		// value encoding/json wrote. A caller building the output by hand can
		// hand it anything, and fmt shows that rather than nothing.
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func writeCatalogResourceVersions(c *toolutil.Card, versions []VersionItem) {
	if len(versions) == 0 {
		return
	}
	table := c.Table("Released Versions", "Version", "Released", "Author", "Commit", "Components")
	for _, version := range versions {
		table.Row(
			toolutil.EscapeMdTableCell(version.Name),
			toolutil.FormatTime(version.ReleasedAt),
			versionAuthorCell(version.Author),
			versionCommitCell(version.Commit),
			toolutil.EscapeMdTableCell(strings.Join(catalogVersionComponentNames(version), ", ")),
		)
	}
}

// versionAuthorCell links the user who published a version by username, or
// writes nothing when GitLab sent no author.
func versionAuthorCell(author *VersionAuthor) string {
	if author == nil {
		return ""
	}
	return toolutil.MdTitleLink("@"+author.Username, author.WebURL)
}

// versionCommitCell links the commit a version was released from by its short
// SHA, or writes nothing when GitLab sent no commit.
func versionCommitCell(commit *VersionCommit) string {
	if commit == nil {
		return ""
	}
	return toolutil.MdTitleLink(commit.ShortID, commit.WebURL)
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

package integrations

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet               = "project.integration_get"
	actionSet               = "project.integration_set"
	actionDelete            = "project.integration_delete"
	actionGetGroup          = "project.integration_get_group"
	actionSetGroup          = "project.integration_set_group"
	actionDeleteGroup       = "project.integration_delete_group"
	actionSetGroupDatadog   = "project.integration_set_group_datadog"
	actionDelGroupDatadog   = "project.integration_delete_group_datadog"
	actionListGroup         = "project.integration_list_group"
	groupDatadogDefaultName = "datadog"
)

// hintWhatAReadReturns says what a read of an integration does and does not
// return. It used to promise "the stored configuration", which the get output
// does not carry: GitLab answers with the integration's identity and whether
// it is active, and never with the properties a caller set.
const hintWhatAReadReturns = "A read returns the integration's identity and whether it is active, not the values configured on it; credentials are write-only and are never returned"

// fallback returns def when s is empty.
func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// writeIntegrationRows writes the rows every integration card shares: the
// identity GitLab keys the integration by, whether it is active, and when it
// was configured.
func writeIntegrationRows(c *toolutil.Card, id int64, slug string, active bool, createdAt, updatedAt string) {
	c.Int("ID", id)
	// The slug is derived from the integration type rather than from anything a
	// person writes, and every value it takes is a URL path segment such as
	// "jira"; it is a code span because it is what the get and set actions take.
	c.Code("Slug", slug)
	c.Bool("Active", active)
	c.Time("Created", createdAt)
	c.Time("Updated", updatedAt)
}

// integrationRowCells renders one integration as the cells of an integration
// table.
func integrationRowCells(i IntegrationItem) []string {
	return []string{
		strconv.FormatInt(i.ID, 10),
		// The title is a locale-dependent display name GitLab returns, and
		// older self-managed instances let a person fill it in.
		toolutil.EscapeMdTableCell(i.Title),
		toolutil.MdCodeSpanCell(i.Slug),
		toolutil.BoolEmoji(i.Active),
	}
}

// writeIntegrationList renders a page of integrations as a Markdown table: a
// collection of objects that share columns.
func writeIntegrationList(items []IntegrationItem, title, emptyResource string, hints ...string) string {
	if len(items) == 0 {
		return toolutil.EmptyMessage(emptyResource)
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, title, len(items), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Title", "Slug", "Active"))
	for _, i := range items {
		sb.WriteString(toolutil.MarkdownTableRow(integrationRowCells(i)...))
	}
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false, hints...)
	return sb.String()
}

// formatListMarkdownString renders the active project integrations.
//
// The empty sentence names the scope: "No integrations found." over a project
// read as an instance with no integrations at all, which is a different
// answer from the one GitLab gave.
func formatListMarkdownString(out ListOutput) string {
	return writeIntegrationList(out.Integrations, "Project Integrations", "active project integrations",
		toolutil.HintAction(actionGet, "read one project integration by its slug"),
		toolutil.HintAction(actionSet, "configure one"),
	)
}

// formatGroupIntegrationListString renders the active group integrations.
func formatGroupIntegrationListString(out ListGroupIntegrationsOutput) string {
	return writeIntegrationList(out.Integrations, "Group Integrations", "active group integrations",
		toolutil.HintAction(actionGetGroup, "read one group integration by its slug"),
		toolutil.HintAction(actionSetGroup, "configure one"),
	)
}

// formatIntegrationItemString renders one integration as the card of one
// object, under the heading the caller names.
func formatIntegrationItemString(heading string, i IntegrationItem, hints ...string) string {
	var sb strings.Builder
	// The title is a locale-dependent display name GitLab returns, and older
	// self-managed instances let a person fill it in for some integrations.
	c := toolutil.NewCard(&sb, heading+": "+fallback(i.Title, i.Slug))
	writeIntegrationRows(c, i.ID, i.Slug, i.Active, i.CreatedAt, i.UpdatedAt)
	c.End(hints...)
	return sb.String()
}

// formatGetMarkdownString renders a single project integration.
func formatGetMarkdownString(out GetOutput) string {
	return formatIntegrationItemString("Integration", out.Integration,
		toolutil.HintAction(actionSet, "change this integration's configuration"),
		toolutil.HintAction(actionDelete, "disable it"),
		hintWhatAReadReturns,
	)
}

// formatSetIntegrationMarkdownString renders the generic project integration
// upsert response.
func formatSetIntegrationMarkdownString(out SetIntegrationOutput) string {
	return formatIntegrationItemString("Integration Updated", out.Integration,
		toolutil.HintAction(actionGet, "read the integration back"),
		hintWhatAReadReturns,
	)
}

// formatSetJiraMarkdownString renders the Jira integration upsert response.
func formatSetJiraMarkdownString(out SetJiraOutput) string {
	return formatIntegrationItemString("Jira Integration Updated", out.Integration,
		toolutil.HintAction(actionGet, "read the integration back"),
		"Jira credentials (username and password, or the API token) are write-only and are never returned",
	)
}

// formatGetGroupIntegrationString renders a single group integration.
func formatGetGroupIntegrationString(out GetGroupIntegrationOutput) string {
	return formatIntegrationItemString("Group Integration", out.Integration,
		toolutil.HintAction(actionSetGroup, "change this integration's configuration"),
		toolutil.HintAction(actionDeleteGroup, "disable it"),
		hintWhatAReadReturns,
	)
}

// formatSetGroupIntegrationString renders the generic group integration upsert
// response.
func formatSetGroupIntegrationString(out SetGroupIntegrationOutput) string {
	return formatIntegrationItemString("Group Integration Updated", out.Integration,
		toolutil.HintAction(actionGetGroup, "read the integration back"),
		hintWhatAReadReturns,
	)
}

// writeGroupDatadogCard renders the common body of a group-level Datadog
// integration as the card of one object: the identity rows, the Datadog
// configuration as a nested object, and the timestamps the read response
// carries and the set response does not.
func writeGroupDatadogCard(sb *strings.Builder, i GroupDatadogItem, headingSuffix string, includeTimestamps bool) *toolutil.Card {
	heading := fmt.Sprintf("Group Datadog Integration%s: %s", headingSuffix, fallback(i.Title, groupDatadogDefaultName))
	c := toolutil.NewCard(sb, heading)
	c.Int("ID", i.ID)
	// The slug is the same type-derived value as every other integration's,
	// here always the literal "datadog", with a compiled-in default when the
	// response omits it.
	c.Code("Slug", fallback(i.Slug, groupDatadogDefaultName))
	c.Bool("Active", i.Active)
	if p := i.Properties; p != nil {
		// Every value here is a Datadog configuration string this server's own
		// set action sends and GitLab stores verbatim.
		properties := c.Sub("Datadog Configuration")
		properties.Field("API URL", p.APIURL)
		properties.Field("Datadog Env", p.DatadogEnv)
		properties.Field("Datadog Service", p.DatadogService)
		properties.Field("Datadog Site", p.DatadogSite)
		properties.Field("Datadog Tags", p.DatadogTags)
		properties.Bool("Datadog CI Visibility", p.DatadogCIVisibility)
		properties.Bool("Archive Trace Events", p.ArchiveTraceEvents)
	}
	if includeTimestamps {
		c.Time("Created", i.CreatedAt)
		c.Time("Updated", i.UpdatedAt)
	}
	return c
}

// formatGetGroupDatadogMarkdownString renders the read output for the
// group-level Datadog integration.
func formatGetGroupDatadogMarkdownString(out GetGroupDatadogOutput) string {
	var sb strings.Builder
	c := writeGroupDatadogCard(&sb, out.Integration, "", true)
	c.End(
		toolutil.HintAction(actionSetGroupDatadog, "update the Datadog configuration"),
		toolutil.HintAction(actionDelGroupDatadog, "remove it from the group"),
	)
	return sb.String()
}

// formatSetGroupDatadogMarkdownString renders the mutate output for the
// group-level Datadog integration. The set response does not include
// created/updated timestamps, so the include flag is false.
func formatSetGroupDatadogMarkdownString(out SetGroupDatadogOutput) string {
	var sb strings.Builder
	c := writeGroupDatadogCard(&sb, out.Integration, " Updated", false)
	c.End(
		toolutil.HintAction(actionListGroup, "see every integration configured on the group"),
		"The api_key value is write-only and is never returned by the read endpoint; rotate the key in Datadog if you need to replace it on the group",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(formatListMarkdownString)
	toolutil.RegisterMarkdown(formatGetMarkdownString)
	toolutil.RegisterMarkdown(formatGetGroupDatadogMarkdownString)
	toolutil.RegisterMarkdown(formatSetGroupDatadogMarkdownString)
	toolutil.RegisterMarkdown(formatSetIntegrationMarkdownString)
	toolutil.RegisterMarkdown(formatGroupIntegrationListString)
	toolutil.RegisterMarkdown(formatGetGroupIntegrationString)
	toolutil.RegisterMarkdown(formatSetGroupIntegrationString)
	toolutil.RegisterMarkdown(formatSetJiraMarkdownString)
}

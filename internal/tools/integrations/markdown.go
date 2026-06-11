package integrations

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Group Datadog markdown format primitives. These format strings are shared
// between the read and set/mutate markdown formatters so the rendering stays
// consistent and the format literals are defined once.
const (
	groupDatadogDefaultTitle  = "datadog"
	groupDatadogHeaderFmt     = "## Group Datadog Integration%s: %s\n\n"
	groupDatadogSlugLineFmt   = "- **Slug**: %s\n"
	groupDatadogActiveLineFmt = "- **Active**: %s\n"
	groupDatadogLabelValueFmt = "- **%s**: %s\n"
	groupDatadogBoolLineFmt   = "- **%s**: %t\n"
)

// writeGroupDatadogActiveLine emits the Active badge for a Datadog
// integration as a single Markdown line.
func writeGroupDatadogActiveLine(sb *strings.Builder, active bool) {
	fmt.Fprintf(sb, groupDatadogActiveLineFmt, yesNo(active))
}

// writeGroupDatadogSlugLine emits the Slug line for a Datadog integration,
// using the supplied default when the Slug is empty.
func writeGroupDatadogSlugLine(sb *strings.Builder, slug string) {
	fmt.Fprintf(sb, groupDatadogSlugLineFmt, fallback(slug, groupDatadogDefaultTitle))
}

// writeGroupDatadogStringField emits a single label-value line when the
// value is non-empty.
func writeGroupDatadogStringField(sb *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(sb, groupDatadogLabelValueFmt, label, value)
}

// writeGroupDatadogBoolField emits a single label-bool line when the
// pointer is non-nil.
func writeGroupDatadogBoolField(sb *strings.Builder, label string, value *bool) {
	if value == nil {
		return
	}
	fmt.Fprintf(sb, groupDatadogBoolLineFmt, label, *value)
}

// writeGroupDatadogTimestamp emits a Created/Updated-style line when the
// timestamp is non-empty, formatting the value through toolutil.FormatTime.
func writeGroupDatadogTimestamp(sb *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(sb, groupDatadogLabelValueFmt, label, toolutil.FormatTime(value))
}

// writeGroupDatadogHeader emits the top-level "## Group Datadog Integration"
// heading. headingSuffix lets the mutate formatter reuse the same body.
func writeGroupDatadogHeader(sb *strings.Builder, i GroupDatadogItem, headingSuffix string) {
	fmt.Fprintf(sb, groupDatadogHeaderFmt, headingSuffix, fallback(i.Title, groupDatadogDefaultTitle))
}

// formatGroupDatadogItem renders the common body of a group-level Datadog
// integration: heading, ID, slug/active badge, per-field lines (skipped when
// empty), and the Created/Updated timestamps. The headingSuffix is appended
// to the heading (e.g. " Updated" for the mutate formatter) and the
// includeTimestamps flag controls whether Created/Updated are rendered
// (the read formatter includes them; the mutate formatter omits them).
func formatGroupDatadogItem(i GroupDatadogItem, headingSuffix string, includeTimestamps bool) string {
	var sb strings.Builder
	writeGroupDatadogHeader(&sb, i, headingSuffix)
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	writeGroupDatadogSlugLine(&sb, i.Slug)
	writeGroupDatadogActiveLine(&sb, i.Active)
	writeGroupDatadogStringField(&sb, "API URL", i.APIURL)
	writeGroupDatadogStringField(&sb, "Datadog Env", i.DatadogEnv)
	writeGroupDatadogStringField(&sb, "Datadog Service", i.DatadogService)
	writeGroupDatadogStringField(&sb, "Datadog Site", i.DatadogSite)
	writeGroupDatadogStringField(&sb, "Datadog Tags", i.DatadogTags)
	writeGroupDatadogBoolField(&sb, "Archive Trace Events", i.ArchiveTraceEvents)
	if includeTimestamps {
		writeGroupDatadogTimestamp(&sb, "Created", i.CreatedAt)
		writeGroupDatadogTimestamp(&sb, "Updated", i.UpdatedAt)
	}
	return sb.String()
}

// yesNo renders a Go bool as the strings "Yes" or "No" used by the
// markdown formatters' Active badge.
func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// formatListMarkdownString renders a ListOutput as a Markdown table string.
func formatListMarkdownString(out ListOutput) string {
	if len(out.Integrations) == 0 {
		return "No integrations found for this project.\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Integrations (%d)\n\n", len(out.Integrations))
	sb.WriteString("| ID | Title | Slug | Active |\n")
	sb.WriteString("|----|-------|------|--------|\n")
	for _, i := range out.Integrations {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", i.ID, toolutil.EscapeMdTableCell(i.Title), i.Slug, yesNo(i.Active))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_integration` to view details of a specific integration")
	return sb.String()
}

// formatGetMarkdownString renders a single integration as a Markdown string.
func formatGetMarkdownString(out GetOutput) string {
	i := out.Integration
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Integration: %s\n\n", i.Title)
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	fmt.Fprintf(&sb, "- **Slug**: %s\n", i.Slug)
	writeGroupDatadogActiveLine(&sb, i.Active)
	writeGroupDatadogTimestamp(&sb, "Created", i.CreatedAt)
	writeGroupDatadogTimestamp(&sb, "Updated", i.UpdatedAt)
	toolutil.WriteHints(&sb, "Use `gitlab_update_integration` to modify this integration's settings")
	return sb.String()
}

// formatGetGroupDatadogMarkdownString renders the read output for the
// group-level Datadog integration as a Markdown string.
func formatGetGroupDatadogMarkdownString(out GetGroupDatadogOutput) string {
	body := formatGroupDatadogItem(out.Integration, "", true)
	// Append the standard hints footer to the rendered body.
	var sb strings.Builder
	sb.WriteString(body)
	toolutil.WriteHints(&sb, "Use `gitlab_set_group_datadog_integration` to update fields; use `gitlab_delete_group_datadog_integration` to remove the configuration")
	return sb.String()
}

// formatSetGroupDatadogMarkdownString renders the mutate output for the
// group-level Datadog integration as a Markdown string. The set response
// does not include created/updated timestamps, so the include flag is false.
func formatSetGroupDatadogMarkdownString(out SetGroupDatadogOutput) string {
	body := formatGroupDatadogItem(out.Integration, " Updated", false)
	var sb strings.Builder
	sb.WriteString(body)
	toolutil.WriteHints(&sb, "Note: the `api_key` value is write-only and is never returned by the read endpoint; rotate the key in Datadog if you need to replace it on the group")
	return sb.String()
}

// FormatListMarkdown formats a list of integrations.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatListMarkdownString(out))
}

// FormatGetMarkdown formats a single integration.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatGetMarkdownString(out))
}

// FormatGetGroupDatadogMarkdown renders the read output for the group-level Datadog integration.
func FormatGetGroupDatadogMarkdown(out GetGroupDatadogOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatGetGroupDatadogMarkdownString(out))
}

// FormatSetGroupDatadogMarkdown renders the mutate output for the group-level Datadog integration.
func FormatSetGroupDatadogMarkdown(out SetGroupDatadogOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatSetGroupDatadogMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdown(formatListMarkdownString)
	toolutil.RegisterMarkdown(formatGetMarkdownString)
	toolutil.RegisterMarkdown(formatGetGroupDatadogMarkdownString)
	toolutil.RegisterMarkdown(formatSetGroupDatadogMarkdownString)
}

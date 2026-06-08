package integrations

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown formats a list of integrations.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	if len(out.Integrations) == 0 {
		return toolutil.ToolResultWithMarkdown("No integrations found for this project.\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Project Integrations (%d)\n\n", len(out.Integrations))
	sb.WriteString("| ID | Title | Slug | Active |\n")
	sb.WriteString("|----|-------|------|--------|\n")
	for _, i := range out.Integrations {
		active := "No"
		if i.Active {
			active = "Yes"
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n", i.ID, toolutil.EscapeMdTableCell(i.Title), i.Slug, active)
	}
	toolutil.WriteHints(&sb, "Use `gitlab_get_integration` to view details of a specific integration")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetMarkdown formats a single integration.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	i := out.Integration
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Integration: %s\n\n", i.Title)
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	fmt.Fprintf(&sb, "- **Slug**: %s\n", i.Slug)
	active := "No"
	if i.Active {
		active = "Yes"
	}
	fmt.Fprintf(&sb, "- **Active**: %s\n", active)
	if i.CreatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdCreated, toolutil.FormatTime(i.CreatedAt))
	}
	if i.UpdatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdUpdated, toolutil.FormatTime(i.UpdatedAt))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_update_integration` to modify this integration's settings")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetGroupDatadogMarkdown renders the read output for the group-level Datadog integration.
func FormatGetGroupDatadogMarkdown(out GetGroupDatadogOutput) *mcp.CallToolResult {
	i := out.Integration
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Group Datadog Integration: %s\n\n", fallback(i.Title, "datadog"))
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	fmt.Fprintf(&sb, "- **Slug**: %s\n", fallback(i.Slug, "datadog"))
	active := "No"
	if i.Active {
		active = "Yes"
	}
	fmt.Fprintf(&sb, "- **Active**: %s\n", active)
	if i.APIURL != "" {
		fmt.Fprintf(&sb, "- **API URL**: %s\n", i.APIURL)
	}
	if i.DatadogEnv != "" {
		fmt.Fprintf(&sb, "- **Datadog Env**: %s\n", i.DatadogEnv)
	}
	if i.DatadogService != "" {
		fmt.Fprintf(&sb, "- **Datadog Service**: %s\n", i.DatadogService)
	}
	if i.DatadogSite != "" {
		fmt.Fprintf(&sb, "- **Datadog Site**: %s\n", i.DatadogSite)
	}
	if i.DatadogTags != "" {
		fmt.Fprintf(&sb, "- **Datadog Tags**: %s\n", i.DatadogTags)
	}
	if i.ArchiveTraceEvents != nil {
		fmt.Fprintf(&sb, "- **Archive Trace Events**: %t\n", *i.ArchiveTraceEvents)
	}
	if i.CreatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdCreated, toolutil.FormatTime(i.CreatedAt))
	}
	if i.UpdatedAt != "" {
		fmt.Fprintf(&sb, toolutil.FmtMdUpdated, toolutil.FormatTime(i.UpdatedAt))
	}
	toolutil.WriteHints(&sb, "Use `gitlab_set_group_datadog_integration` to update fields; use `gitlab_delete_group_datadog_integration` to remove the configuration")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatSetGroupDatadogMarkdown renders the mutate output for the group-level Datadog integration.
func FormatSetGroupDatadogMarkdown(out SetGroupDatadogOutput) *mcp.CallToolResult {
	i := out.Integration
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Group Datadog Integration Updated: %s\n\n", fallback(i.Title, "datadog"))
	fmt.Fprintf(&sb, toolutil.FmtMdID, i.ID)
	active := "No"
	if i.Active {
		active = "Yes"
	}
	fmt.Fprintf(&sb, "- **Active**: %s\n", active)
	if i.APIURL != "" {
		fmt.Fprintf(&sb, "- **API URL**: %s\n", i.APIURL)
	}
	if i.DatadogEnv != "" {
		fmt.Fprintf(&sb, "- **Datadog Env**: %s\n", i.DatadogEnv)
	}
	if i.DatadogService != "" {
		fmt.Fprintf(&sb, "- **Datadog Service**: %s\n", i.DatadogService)
	}
	if i.DatadogSite != "" {
		fmt.Fprintf(&sb, "- **Datadog Site**: %s\n", i.DatadogSite)
	}
	if i.DatadogTags != "" {
		fmt.Fprintf(&sb, "- **Datadog Tags**: %s\n", i.DatadogTags)
	}
	if i.ArchiveTraceEvents != nil {
		fmt.Fprintf(&sb, "- **Archive Trace Events**: %t\n", *i.ArchiveTraceEvents)
	}
	toolutil.WriteHints(&sb, "Note: the `api_key` value is write-only and is never returned by the read endpoint; rotate the key in Datadog if you need to replace it on the group")
	return toolutil.ToolResultWithMarkdown(sb.String())
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetGroupDatadogMarkdown)
	toolutil.RegisterMarkdownResult(FormatSetGroupDatadogMarkdown)
}

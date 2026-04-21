// markdown.go provides Markdown formatting functions for usage data MCP tool output.

package usagedata

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatServicePingMarkdown formats service ping data as markdown.
func FormatServicePingMarkdown(out GetServicePingOutput) string {
	var sb strings.Builder
	sb.WriteString("## Service Ping Data\n\n")
	fmt.Fprintf(&sb, "**Recorded At**: %s\n\n", toolutil.FormatTime(out.RecordedAt))

	if len(out.License) > 0 {
		sb.WriteString("### License\n\n")
		sb.WriteString("| Key | Value |\n|---|---|\n")
		for _, k := range sortedKeys(out.License) {
			fmt.Fprintf(&sb, "| %s | %s |\n",
				toolutil.EscapeMdTableCell(k), toolutil.EscapeMdTableCell(out.License[k]))
		}
		sb.WriteString("\n")
	}

	if len(out.Counts) > 0 {
		sb.WriteString("### Counts (first 20)\n\n")
		sb.WriteString("| Metric | Count |\n|---|---|\n")
		keys := sortedKeysInt64(out.Counts)
		limit := min(len(keys), 20)
		for _, k := range keys[:limit] {
			fmt.Fprintf(&sb, "| %s | %d |\n",
				toolutil.EscapeMdTableCell(k), out.Counts[k])
		}
		if len(keys) > 20 {
			fmt.Fprintf(&sb, "\n*...and %d more metrics*\n", len(keys)-20)
		}
	}

	toolutil.WriteHints(&sb, "Use individual metric tools for detailed analysis")
	return sb.String()
}

// FormatNonSQLMetricsMarkdown formats non-SQL metrics as markdown.
func FormatNonSQLMetricsMarkdown(out NonSQLMetricsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Non-SQL Metrics\n\n")
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| UUID | %s |\n", toolutil.EscapeMdTableCell(out.UUID))
	fmt.Fprintf(&sb, "| Hostname | %s |\n", toolutil.EscapeMdTableCell(out.Hostname))
	fmt.Fprintf(&sb, "| Version | %s |\n", toolutil.EscapeMdTableCell(out.Version))
	fmt.Fprintf(&sb, "| Edition | %s |\n", toolutil.EscapeMdTableCell(out.Edition))
	fmt.Fprintf(&sb, "| Installation Type | %s |\n", toolutil.EscapeMdTableCell(out.InstallationType))
	fmt.Fprintf(&sb, "| Active Users | %d |\n", out.ActiveUserCount)
	fmt.Fprintf(&sb, "| Historical Max Users | %d |\n", out.HistoricalMaxUsers)
	fmt.Fprintf(&sb, "| License Plan | %s |\n", toolutil.EscapeMdTableCell(out.LicensePlan))
	fmt.Fprintf(&sb, "| Recorded At | %s |\n", toolutil.EscapeMdTableCell(toolutil.FormatTime(out.RecordedAt)))
	toolutil.WriteHints(&sb, "Use `gitlab_get_service_ping` for the full metrics overview")
	return sb.String()
}

// FormatQueriesMarkdown formats queries as markdown.
func FormatQueriesMarkdown(out QueriesOutput) string {
	var sb strings.Builder
	sb.WriteString("## Service Ping Queries\n\n")
	fmt.Fprintf(&sb, "**Version**: %s | **Edition**: %s | **Recorded At**: %s\n\n",
		out.Version, out.Edition, out.RecordedAt)

	if len(out.Counts) > 0 {
		sb.WriteString("### SQL Queries (first 20)\n\n")
		sb.WriteString("| Metric | Query |\n|---|---|\n")
		keys := sortedKeys(out.Counts)
		limit := min(len(keys), 20)
		for _, k := range keys[:limit] {
			fmt.Fprintf(&sb, "| %s | %s |\n",
				toolutil.EscapeMdTableCell(k), toolutil.EscapeMdTableCell(out.Counts[k]))
		}
		if len(keys) > 20 {
			fmt.Fprintf(&sb, "\n*...and %d more queries*\n", len(keys)-20)
		}
	}
	toolutil.WriteHints(&sb, "Review query patterns for optimization opportunities")
	return sb.String()
}

// FormatMetricDefinitionsMarkdown formats metric definitions as markdown.
func FormatMetricDefinitionsMarkdown(out MetricDefinitionsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Metric Definitions (YAML)\n\n")
	sb.WriteString("```yaml\n")
	// Truncate if very large
	yaml := out.YAML
	if len(yaml) > 10000 {
		yaml = yaml[:10000] + "\n# ... truncated (total " + strconv.Itoa(len(out.YAML)) + " bytes)"
	}
	sb.WriteString(yaml)
	sb.WriteString("\n```\n")
	toolutil.WriteHints(&sb, "Use metric key names to query specific usage data")
	return sb.String()
}

// FormatTrackEventMarkdown formats track event result as markdown.
func FormatTrackEventMarkdown(out TrackEventOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Track Event\n\n**Status**: %s\n", out.Status)
	toolutil.WriteHints(&b, "Use `gitlab_list_usage_data_metrics` to review available metrics")
	return b.String()
}

// FormatTrackEventsMarkdown formats track events result as markdown.
func FormatTrackEventsMarkdown(out TrackEventsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Track Events\n\n**Status**: %s | **Events**: %d\n", out.Status, out.Count)
	toolutil.WriteHints(&b, "Use `gitlab_list_usage_data_metrics` to review available metrics")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatServicePingMarkdown)
	toolutil.RegisterMarkdown(FormatNonSQLMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatQueriesMarkdown)
	toolutil.RegisterMarkdown(FormatMetricDefinitionsMarkdown)
	toolutil.RegisterMarkdown(FormatTrackEventMarkdown)
	toolutil.RegisterMarkdown(FormatTrackEventsMarkdown)
}

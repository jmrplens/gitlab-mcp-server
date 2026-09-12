package usagedata

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionServicePing       = "admin.usage_data_service_ping"
	actionNonSQLMetrics     = "admin.usage_data_non_sql_metrics"
	actionQueries           = "admin.usage_data_queries"
	actionMetricDefinitions = "admin.usage_data_metric_definitions"
	actionTrackEvents       = "admin.usage_data_track_events"
)

// maxRenderedMetrics is how many rows of a Service Ping map the card shows.
// The whole map reaches the caller in the structured result, and the note under
// the table says how much of it the card left out.
const maxRenderedMetrics = 20

// maxRenderedYAMLBytes is how much of the metric-definition document the card
// shows. The cut lands on a rune boundary: a byte-count cut through a
// multi-byte character leaves a replacement glyph at the end of the fence.
const maxRenderedYAMLBytes = 10000

// FormatServicePingMarkdown renders the Service Ping payload as a card: when it
// was recorded, then the license attributes and the metric counts as nested
// collections.
func FormatServicePingMarkdown(out GetServicePingOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Service Ping Data")
	c.Time("Recorded At", out.RecordedAt)

	if len(out.License) > 0 {
		table := c.Table("License", "Key", "Value")
		for _, key := range sortedKeys(out.License) {
			table.Row(toolutil.EscapeMdTableCell(key), toolutil.EscapeMdTableCell(out.License[key]))
		}
	}

	keys := sortedKeysInt64(out.Counts)
	if len(keys) > 0 {
		shown := min(len(keys), maxRenderedMetrics)
		table := c.Table("Counts", "Metric", "Count")
		for _, key := range keys[:shown] {
			table.Row(toolutil.EscapeMdTableCell(key), strconv.FormatInt(out.Counts[key], 10))
		}
		c.Note(truncationNote(shown, len(keys), "metrics"))
	}

	c.End(
		toolutil.HintAction(actionNonSQLMetrics, "read the non-SQL half of the same report"),
		toolutil.HintAction(actionMetricDefinitions, "look a metric key up in the definitions"),
	)
	return b.String()
}

// FormatNonSQLMetricsMarkdown renders the non-SQL half of the Service Ping
// report as the card of one object.
func FormatNonSQLMetricsMarkdown(out NonSQLMetricsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Non-SQL Metrics")
	c.Field("UUID", out.UUID)
	c.Field("Hostname", out.Hostname)
	c.Field("Version", out.Version)
	c.Field("Edition", out.Edition)
	c.Field("Installation Type", out.InstallationType)
	c.Int("Active Users", out.ActiveUserCount)
	c.Int("Historical Max Users", out.HistoricalMaxUsers)
	c.Field("License Plan", out.LicensePlan)
	c.Time("Recorded At", out.RecordedAt)
	c.End(toolutil.HintAction(actionServicePing, "read the full Service Ping report"))
	return b.String()
}

// FormatQueriesMarkdown renders the SQL behind the Service Ping counters as a
// card whose nested collection is the query per metric.
//
// The instance's version, edition and recording time used to share one line
// with no list marker in front of it and no escaping on any of the three, so a
// value carrying a pipe, a tag or a line break wrote whatever it liked into the
// response. Each is a row of its own now.
func FormatQueriesMarkdown(out QueriesOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Service Ping Queries")
	c.Field("Version", out.Version)
	c.Field("Edition", out.Edition)
	c.Time("Recorded At", out.RecordedAt)

	keys := sortedKeys(out.Counts)
	if len(keys) > 0 {
		shown := min(len(keys), maxRenderedMetrics)
		table := c.Table("SQL Queries", "Metric", "Query")
		for _, key := range keys[:shown] {
			table.Row(toolutil.EscapeMdTableCell(key), toolutil.EscapeMdTableCell(out.Counts[key]))
		}
		c.Note(truncationNote(shown, len(keys), "queries"))
	}

	c.End(
		toolutil.HintAction(actionServicePing, "read the counts these queries produce"),
		toolutil.HintAction(actionMetricDefinitions, "look a metric key up in the definitions"),
	)
	return b.String()
}

// FormatMetricDefinitionsMarkdown renders the metric dictionary as a card whose
// body is the YAML document inside a fence sized to it.
func FormatMetricDefinitionsMarkdown(out MetricDefinitionsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Metric Definitions (YAML)")
	c.Warn("Truncated", out.Truncated)

	document, shortened := displayYAML(out.YAML)
	c.Fence("", "yaml", document)

	// Two different cuts can reach this point and the model must not read the
	// second as the first: the display cut shortens a long document for this
	// response only, while Truncated means the server stopped reading and the
	// rest of the document is not in the structured result either.
	if shortened {
		c.Note(fmt.Sprintf("The card shows the first %d bytes of the document; the whole of what this action read is in the structured result.", maxRenderedYAMLBytes))
	}
	if out.Truncated {
		c.Note("The document exceeded the size this action returns, so it was cut short. Read the remainder from GitLab directly (GET /usage_data/metric_definitions).")
	}

	c.End(toolutil.HintAction(actionServicePing, "read the values these metrics are reported with"))
	return b.String()
}

// FormatTrackEventMarkdown renders the answer to one tracked event as a card.
func FormatTrackEventMarkdown(out TrackEventOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Track Event")
	c.Field("Status", out.Status)
	c.End(toolutil.HintAction(actionTrackEvents, "send a batch of events in one call"))
	return b.String()
}

// FormatTrackEventsMarkdown renders the answer to a batch of tracked events as
// a card.
func FormatTrackEventsMarkdown(out TrackEventsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Track Events")
	c.Field("Status", out.Status)
	c.Int("Events", int64(out.Count))
	c.End(toolutil.HintAction(actionMetricDefinitions, "review the metrics these events feed"))
	return b.String()
}

// truncationNote is the sentence under a table the card cut short, or nothing
// when it showed every row.
func truncationNote(shown, total int, noun string) string {
	if total <= shown {
		return ""
	}
	return fmt.Sprintf("Showing the first %d of %d %s; the rest are in the structured result.", shown, total, noun)
}

// displayYAML shortens the definition document for display and reports whether
// it had to, cutting on a rune boundary so the fence never ends in half a
// character.
func displayYAML(document string) (string, bool) {
	if len(document) <= maxRenderedYAMLBytes {
		return document, false
	}
	return string(truncateAtRuneBoundary([]byte(document), maxRenderedYAMLBytes)), true
}

func init() {
	toolutil.RegisterMarkdown(FormatServicePingMarkdown)
	toolutil.RegisterMarkdown(FormatNonSQLMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatQueriesMarkdown)
	toolutil.RegisterMarkdown(FormatMetricDefinitionsMarkdown)
	toolutil.RegisterMarkdown(FormatTrackEventMarkdown)
	toolutil.RegisterMarkdown(FormatTrackEventsMarkdown)
}

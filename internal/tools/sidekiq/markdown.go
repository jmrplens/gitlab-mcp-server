package sidekiq

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionQueueMetrics   = "admin.sidekiq_queue_metrics"
	actionProcessMetrics = "admin.sidekiq_process_metrics"
	actionJobStats       = "admin.sidekiq_job_stats"
	actionCompound       = "admin.sidekiq_compound_metrics"
)

// queueColumns are the columns of the queue table, shared by the standalone
// queue result and the queue section of the compound one so a reader meets one
// table in both.
var queueColumns = []string{"Queue", "Backlog", "Latency"}

// processColumns are the columns of the process table, shared the same way.
var processColumns = []string{"Hostname", "PID", "Tag", "Started At", "Concurrency", "Busy", "Queues"}

// queueCells renders one queue as the cells of a row. The name is whatever the
// instance called the queue.
func queueCells(q QueueItem) []string {
	return []string{
		toolutil.EscapeMdTableCell(q.Name),
		strconv.FormatInt(q.Backlog, 10),
		strconv.FormatInt(q.Latency, 10),
	}
}

// processCells renders one Sidekiq process as the cells of a row. The hostname,
// the tag and the queue names come from the instance's own configuration, and
// the start time is rendered in the display form rather than as the RFC 3339
// string GitLab sent.
func processCells(p ProcessItem) []string {
	return []string{
		toolutil.EscapeMdTableCell(p.Hostname),
		strconv.FormatInt(p.Pid, 10),
		toolutil.EscapeMdTableCell(p.Tag),
		toolutil.FormatTime(p.StartedAt),
		strconv.FormatInt(p.Concurrency, 10),
		strconv.FormatInt(p.Busy, 10),
		toolutil.EscapeMdTableCell(strings.Join(p.Queues, ", ")),
	}
}

// FormatQueueMetricsMarkdown renders the queues as a table: a collection of
// objects that share columns.
func FormatQueueMetricsMarkdown(out GetQueueMetricsOutput) string {
	if len(out.Queues) == 0 {
		return toolutil.EmptyMessage("Sidekiq queues")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Sidekiq Queue Metrics", len(out.Queues), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader(queueColumns...))
	for _, q := range out.Queues {
		b.WriteString(toolutil.MarkdownTableRow(queueCells(q)...))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		"Monitor queues with high backlog or latency for potential issues",
		toolutil.HintAction(actionCompound, "read the queues, processes and job counts in one call"))
	return b.String()
}

// FormatProcessMetricsMarkdown renders the Sidekiq processes as a table.
func FormatProcessMetricsMarkdown(out GetProcessMetricsOutput) string {
	if len(out.Processes) == 0 {
		return toolutil.EmptyMessage("Sidekiq processes")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Sidekiq Process Metrics", len(out.Processes), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader(processColumns...))
	for _, p := range out.Processes {
		b.WriteString(toolutil.MarkdownTableRow(processCells(p)...))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		"Check process resource usage to identify overloaded workers",
		toolutil.HintAction(actionCompound, "read the queues, processes and job counts in one call"))
	return b.String()
}

// FormatJobStatsMarkdown renders the job counters as the card of one object.
//
// The three counters used to open a "| Metric | Value |" table, which is the
// shape a collection takes; three fields of one object are card rows.
func FormatJobStatsMarkdown(out GetJobStatsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Sidekiq Job Statistics")
	writeJobStats(c, out.Jobs)
	c.End(toolutil.HintAction(actionCompound, "read the queues, processes and job counts in one call"))
	return b.String()
}

// FormatCompoundMetricsMarkdown renders queues, processes and job counters as
// one card with a section each.
func FormatCompoundMetricsMarkdown(out GetCompoundMetricsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Sidekiq Compound Metrics")

	queues := c.Section("Queues")
	if len(out.Queues) == 0 {
		queues.Note(toolutil.EmptyMessage("Sidekiq queues"))
	} else {
		table := queues.Table("", queueColumns...)
		for _, q := range out.Queues {
			table.Row(queueCells(q)...)
		}
	}

	processes := c.Section("Processes")
	if len(out.Processes) == 0 {
		processes.Note(toolutil.EmptyMessage("Sidekiq processes"))
	} else {
		table := processes.Table("", processColumns...)
		for _, p := range out.Processes {
			table.Row(processCells(p)...)
		}
	}

	writeJobStats(c.Section("Job Statistics"), out.Jobs)

	c.End(
		toolutil.HintAction(actionQueueMetrics, "read the queues on their own"),
		toolutil.HintAction(actionProcessMetrics, "read the worker processes on their own"),
		toolutil.HintAction(actionJobStats, "read the job counters on their own"),
	)
	return b.String()
}

// writeJobStats writes the three job counters as rows of c. Zero is an answer
// for every one of them, so each is written whatever GitLab sent.
func writeJobStats(c *toolutil.Card, jobs JobStatsItem) {
	c.Int("Processed", jobs.Processed)
	c.Int("Failed", jobs.Failed)
	c.Int("Enqueued", jobs.Enqueued)
}

func init() {
	toolutil.RegisterMarkdown(FormatQueueMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatProcessMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatJobStatsMarkdown)
	toolutil.RegisterMarkdown(FormatCompoundMetricsMarkdown)
}

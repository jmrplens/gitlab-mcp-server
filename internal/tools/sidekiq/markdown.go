// markdown.go provides Markdown formatting functions for Sidekiq MCP tool output.
package sidekiq

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatQueueMetricsMarkdown formats queue metrics as markdown.
func FormatQueueMetricsMarkdown(out GetQueueMetricsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Sidekiq Queue Metrics\n\n")
	if len(out.Queues) == 0 {
		sb.WriteString("No queues found.\n")
		return sb.String()
	}
	sb.WriteString("| Queue | Backlog | Latency |\n")
	sb.WriteString("|---|---|---|\n")
	for _, q := range out.Queues {
		fmt.Fprintf(&sb, "| %s | %d | %d |\n",
			toolutil.EscapeMdTableCell(q.Name), q.Backlog, q.Latency)
	}
	toolutil.WriteHints(&sb, "Monitor queues with high backlog or latency for potential issues")
	return sb.String()
}

// FormatProcessMetricsMarkdown formats process metrics as markdown.
func FormatProcessMetricsMarkdown(out GetProcessMetricsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Sidekiq Process Metrics\n\n")
	if len(out.Processes) == 0 {
		sb.WriteString("No processes found.\n")
		return sb.String()
	}
	sb.WriteString("| Hostname | PID | Tag | Started At | Concurrency | Busy | Queues |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	for _, p := range out.Processes {
		queues := strings.Join(p.Queues, ", ")
		fmt.Fprintf(&sb, "| %s | %d | %s | %s | %d | %d | %s |\n",
			toolutil.EscapeMdTableCell(p.Hostname),
			p.Pid,
			toolutil.EscapeMdTableCell(p.Tag),
			toolutil.EscapeMdTableCell(p.StartedAt),
			p.Concurrency,
			p.Busy,
			toolutil.EscapeMdTableCell(queues))
	}
	toolutil.WriteHints(&sb, "Check process resource usage to identify overloaded workers")
	return sb.String()
}

// FormatJobStatsMarkdown formats job statistics as markdown.
func FormatJobStatsMarkdown(out GetJobStatsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Sidekiq Job Statistics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| Processed | %d |\n", out.Jobs.Processed)
	fmt.Fprintf(&sb, "| Failed | %d |\n", out.Jobs.Failed)
	fmt.Fprintf(&sb, "| Enqueued | %d |\n", out.Jobs.Enqueued)
	toolutil.WriteHints(&sb, "Use `gitlab_get_sidekiq_compound_metrics` for a complete overview")
	return sb.String()
}

// FormatCompoundMetricsMarkdown formats compound metrics as markdown.
func FormatCompoundMetricsMarkdown(out GetCompoundMetricsOutput) string {
	var sb strings.Builder
	sb.WriteString("## Sidekiq Compound Metrics\n\n")

	// Queues section
	sb.WriteString("### Queues\n\n")
	if len(out.Queues) == 0 {
		sb.WriteString("No queues found.\n")
	} else {
		sb.WriteString("| Queue | Backlog | Latency |\n")
		sb.WriteString("|---|---|---|\n")
		for _, q := range out.Queues {
			fmt.Fprintf(&sb, "| %s | %d | %d |\n",
				toolutil.EscapeMdTableCell(q.Name), q.Backlog, q.Latency)
		}
		sb.WriteString("\n")
	}

	// Processes section
	sb.WriteString("### Processes\n\n")
	if len(out.Processes) == 0 {
		sb.WriteString("No processes found.\n")
	} else {
		sb.WriteString("| Hostname | PID | Tag | Started At | Concurrency | Busy |\n")
		sb.WriteString("|---|---|---|---|---|---|\n")
		for _, p := range out.Processes {
			fmt.Fprintf(&sb, "| %s | %d | %s | %s | %d | %d |\n",
				toolutil.EscapeMdTableCell(p.Hostname),
				p.Pid,
				toolutil.EscapeMdTableCell(p.Tag),
				toolutil.EscapeMdTableCell(p.StartedAt),
				p.Concurrency,
				p.Busy)
		}
		sb.WriteString("\n")
	}

	// Jobs section
	sb.WriteString("### Job Statistics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| Processed | %d |\n", out.Jobs.Processed)
	fmt.Fprintf(&sb, "| Failed | %d |\n", out.Jobs.Failed)
	fmt.Fprintf(&sb, "| Enqueued | %d |\n", out.Jobs.Enqueued)

	toolutil.WriteHints(&sb, "Use individual metric tools for detailed queue, process, or job analysis")
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatQueueMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatProcessMetricsMarkdown)
	toolutil.RegisterMarkdown(FormatJobStatsMarkdown)
	toolutil.RegisterMarkdown(FormatCompoundMetricsMarkdown)
}

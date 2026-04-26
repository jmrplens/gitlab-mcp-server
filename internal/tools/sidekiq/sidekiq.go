// Package sidekiq implements MCP tools for GitLab Sidekiq metrics API.
package sidekiq

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// hintSidekiqAdminRequired is the 403 hint shared by all Sidekiq metrics tools.
const hintSidekiqAdminRequired = "Sidekiq metrics require administrator access \u2014 verify your token has admin scope"

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// QueueItem represents a single Sidekiq queue with its metrics.
type QueueItem struct {
	Name    string `json:"name"`
	Backlog int64  `json:"backlog"`
	Latency int64  `json:"latency"`
}

// ProcessItem represents a single Sidekiq process.
type ProcessItem struct {
	Hostname    string   `json:"hostname"`
	Pid         int64    `json:"pid"`
	Tag         string   `json:"tag"`
	StartedAt   string   `json:"started_at"`
	Queues      []string `json:"queues"`
	Labels      []string `json:"labels"`
	Concurrency int64    `json:"concurrency"`
	Busy        int64    `json:"busy"`
}

// JobStatsItem represents Sidekiq job statistics.
type JobStatsItem struct {
	Processed int64 `json:"processed"`
	Failed    int64 `json:"failed"`
	Enqueued  int64 `json:"enqueued"`
}

// ---------------------------------------------------------------------------
// GetQueueMetrics
// ---------------------------------------------------------------------------.

// GetQueueMetricsInput is the input for the queue metrics tool.
type GetQueueMetricsInput struct{}

// GetQueueMetricsOutput is the output for the queue metrics tool.
type GetQueueMetricsOutput struct {
	toolutil.HintableOutput
	Queues []QueueItem `json:"queues"`
}

// GetQueueMetrics retrieves current Sidekiq queue metrics.
func GetQueueMetrics(ctx context.Context, client *gitlabclient.Client, _ GetQueueMetricsInput) (GetQueueMetricsOutput, error) {
	metrics, _, err := client.GL().Sidekiq.GetQueueMetrics(gl.WithContext(ctx))
	if err != nil {
		return GetQueueMetricsOutput{}, toolutil.WrapErrWithStatusHint("get_queue_metrics", err, http.StatusForbidden, hintSidekiqAdminRequired)
	}
	return GetQueueMetricsOutput{Queues: convertQueues(metrics.Queues)}, nil
}

// ---------------------------------------------------------------------------
// GetProcessMetrics
// ---------------------------------------------------------------------------.

// GetProcessMetricsInput is the input for the process metrics tool.
type GetProcessMetricsInput struct{}

// GetProcessMetricsOutput is the output for the process metrics tool.
type GetProcessMetricsOutput struct {
	toolutil.HintableOutput
	Processes []ProcessItem `json:"processes"`
}

// GetProcessMetrics retrieves current Sidekiq process metrics.
func GetProcessMetrics(ctx context.Context, client *gitlabclient.Client, _ GetProcessMetricsInput) (GetProcessMetricsOutput, error) {
	metrics, _, err := client.GL().Sidekiq.GetProcessMetrics(gl.WithContext(ctx))
	if err != nil {
		return GetProcessMetricsOutput{}, toolutil.WrapErrWithStatusHint("get_process_metrics", err, http.StatusForbidden, hintSidekiqAdminRequired)
	}
	return GetProcessMetricsOutput{Processes: convertProcesses(metrics.Processes)}, nil
}

// ---------------------------------------------------------------------------
// GetJobStats
// ---------------------------------------------------------------------------.

// GetJobStatsInput is the input for the job stats tool.
type GetJobStatsInput struct{}

// GetJobStatsOutput is the output for the job stats tool.
type GetJobStatsOutput struct {
	toolutil.HintableOutput
	Jobs JobStatsItem `json:"jobs"`
}

// GetJobStats retrieves current Sidekiq job statistics.
func GetJobStats(ctx context.Context, client *gitlabclient.Client, _ GetJobStatsInput) (GetJobStatsOutput, error) {
	stats, _, err := client.GL().Sidekiq.GetJobStats(gl.WithContext(ctx))
	if err != nil {
		return GetJobStatsOutput{}, toolutil.WrapErrWithStatusHint("get_job_stats", err, http.StatusForbidden, hintSidekiqAdminRequired)
	}
	return GetJobStatsOutput{
		Jobs: JobStatsItem{
			Processed: stats.Jobs.Processed,
			Failed:    stats.Jobs.Failed,
			Enqueued:  stats.Jobs.Enqueued,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// GetCompoundMetrics
// ---------------------------------------------------------------------------.

// GetCompoundMetricsInput is the input for the compound metrics tool.
type GetCompoundMetricsInput struct{}

// GetCompoundMetricsOutput is the output for the compound metrics tool.
type GetCompoundMetricsOutput struct {
	toolutil.HintableOutput
	Queues    []QueueItem   `json:"queues"`
	Processes []ProcessItem `json:"processes"`
	Jobs      JobStatsItem  `json:"jobs"`
}

// GetCompoundMetrics retrieves all Sidekiq metrics in a single compound response.
func GetCompoundMetrics(ctx context.Context, client *gitlabclient.Client, _ GetCompoundMetricsInput) (GetCompoundMetricsOutput, error) {
	metrics, _, err := client.GL().Sidekiq.GetCompoundMetrics(gl.WithContext(ctx))
	if err != nil {
		return GetCompoundMetricsOutput{}, toolutil.WrapErrWithStatusHint("get_compound_metrics", err, http.StatusForbidden, hintSidekiqAdminRequired)
	}
	return GetCompoundMetricsOutput{
		Queues:    convertQueues(metrics.Queues),
		Processes: convertProcesses(metrics.Processes),
		Jobs: JobStatsItem{
			Processed: metrics.Jobs.Processed,
			Failed:    metrics.Jobs.Failed,
			Enqueued:  metrics.Jobs.Enqueued,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------.

// convertQueues is an internal helper for the sidekiq package.
func convertQueues(queues map[string]gl.QueueMetricsQueue) []QueueItem {
	items := make([]QueueItem, 0, len(queues))
	for name, q := range queues {
		items = append(items, QueueItem{
			Name:    name,
			Backlog: q.Backlog,
			Latency: q.Latency,
		})
	}
	return items
}

// convertProcesses is an internal helper for the sidekiq package.
func convertProcesses(procs []gl.ProcessMetricsProcess) []ProcessItem {
	items := make([]ProcessItem, 0, len(procs))
	for _, p := range procs {
		startedAt := ""
		if p.StartedAt != nil {
			startedAt = p.StartedAt.Format(time.RFC3339)
		}
		items = append(items, ProcessItem{
			Hostname:    p.Hostname,
			Pid:         p.Pid,
			Tag:         p.Tag,
			StartedAt:   startedAt,
			Queues:      p.Queues,
			Labels:      p.Labels,
			Concurrency: p.Concurrency,
			Busy:        p.Busy,
		})
	}
	return items
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

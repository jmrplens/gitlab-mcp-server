package sidekiq

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical IDs every Sidekiq action points a model at: the instance
// metadata read, and the server's own status check. The second is
// `server.status` and not `health.status`, because the catalog names an action
// after the group that publishes it (`gitlab_server`) rather than after the
// package the handler lives in.
const (
	actionMetadataGet  = "admin.metadata_get"
	actionServerStatus = "server.status"
)

// ActionSpecs returns canonical specs for Sidekiq metrics tools.
//
// Each action's usage sentence is written beside the action it describes rather
// than resolved from the action name again inside the options builder: that
// chain of comparisons could hand any action a sibling's sentence, and since
// nothing read which sentence came back, all three of its tests were decided
// one way for ever.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_sidekiq_queue_metrics — read Sidekiq queue backlog and latency metrics.
		sidekiqReadSpec("sidekiq_queue_metrics", toolutil.RouteAction(client, GetQueueMetrics), "gitlab_get_sidekiq_queue_metrics",
			"Read Sidekiq queue metrics for backlog and latency monitoring."),
		// gitlab_get_sidekiq_process_metrics — read Sidekiq worker process metrics.
		sidekiqReadSpec("sidekiq_process_metrics", toolutil.RouteAction(client, GetProcessMetrics), "gitlab_get_sidekiq_process_metrics",
			"Read Sidekiq worker process metrics for concurrency and busy slots."),
		// gitlab_get_sidekiq_job_stats — read aggregate Sidekiq job counts.
		sidekiqReadSpec("sidekiq_job_stats", toolutil.RouteAction(client, GetJobStats), "gitlab_get_sidekiq_job_stats",
			"Read Sidekiq aggregate job stats such as processed, failed, and enqueued counts."),
		// gitlab_get_sidekiq_compound_metrics — read combined Sidekiq queue, process, and job metrics.
		sidekiqReadSpec("sidekiq_compound_metrics", toolutil.RouteAction(client, GetCompoundMetrics), "gitlab_get_sidekiq_compound_metrics",
			"Read combined Sidekiq metrics payload for queue/process/job monitoring."),
	}
}

// sidekiqReadSpec builds the canonical read-only spec for a Sidekiq metrics tool.
func sidekiqReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"admin", "sidekiq", "metrics"},
		Usage:          usage,
		RelatedActions: []string{actionMetadataGet, actionServerStatus},
		OpenWorld:      true,
		OwnerPackage:   "sidekiq",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}

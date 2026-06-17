package sidekiq

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Sidekiq metrics tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_sidekiq_queue_metrics — read Sidekiq queue backlog and latency metrics.
		sidekiqReadSpec("sidekiq_queue_metrics", toolutil.RouteAction(client, GetQueueMetrics), "gitlab_get_sidekiq_queue_metrics"),
		// gitlab_get_sidekiq_process_metrics — read Sidekiq worker process metrics.
		sidekiqReadSpec("sidekiq_process_metrics", toolutil.RouteAction(client, GetProcessMetrics), "gitlab_get_sidekiq_process_metrics"),
		// gitlab_get_sidekiq_job_stats — read aggregate Sidekiq job counts.
		sidekiqReadSpec("sidekiq_job_stats", toolutil.RouteAction(client, GetJobStats), "gitlab_get_sidekiq_job_stats"),
		// gitlab_get_sidekiq_compound_metrics — read combined Sidekiq queue, process, and job metrics.
		sidekiqReadSpec("sidekiq_compound_metrics", toolutil.RouteAction(client, GetCompoundMetrics), "gitlab_get_sidekiq_compound_metrics"),
	}
}

// sidekiqReadSpec builds the canonical read-only spec for a Sidekiq metrics tool.
func sidekiqReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, sidekiqOptions(name, individualTool))
}

func sidekiqOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Read Sidekiq queue metrics for backlog and latency monitoring."
	if actionName == "sidekiq_process_metrics" {
		usage = "Read Sidekiq worker process metrics for concurrency and busy slots."
	}
	if actionName == "sidekiq_job_stats" {
		usage = "Read Sidekiq aggregate job stats such as processed, failed, and enqueued counts."
	}
	if actionName == "sidekiq_compound_metrics" {
		usage = "Read combined Sidekiq metrics payload for queue/process/job monitoring."
	}

	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"admin", "sidekiq", "metrics"},
		Usage:          usage,
		RelatedActions: []string{"admin.metadata_get", "health.status"},
		OpenWorld:      true,
		OwnerPackage:   "sidekiq",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

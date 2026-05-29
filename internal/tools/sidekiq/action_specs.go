package sidekiq

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Sidekiq metrics tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		sidekiqReadSpec("sidekiq_queue_metrics", toolutil.RouteAction(client, GetQueueMetrics), "gitlab_get_sidekiq_queue_metrics"),
		sidekiqReadSpec("sidekiq_process_metrics", toolutil.RouteAction(client, GetProcessMetrics), "gitlab_get_sidekiq_process_metrics"),
		sidekiqReadSpec("sidekiq_job_stats", toolutil.RouteAction(client, GetJobStats), "gitlab_get_sidekiq_job_stats"),
		sidekiqReadSpec("sidekiq_compound_metrics", toolutil.RouteAction(client, GetCompoundMetrics), "gitlab_get_sidekiq_compound_metrics"),
	}
}

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

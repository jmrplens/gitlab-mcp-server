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
	return toolutil.NewReadActionSpec(name, route, sidekiqOptions(individualTool))
}

func sidekiqOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin"},
		OpenWorld:      true,
		OwnerPackage:   "sidekiq",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

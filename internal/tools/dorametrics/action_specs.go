package dorametrics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for DORA metric actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		doraMetricReadSpec("project", toolutil.RouteAction(client, GetProjectMetrics), "gitlab_get_project_dora_metrics"),
		doraMetricReadSpec("group", toolutil.RouteAction(client, GetGroupMetrics), "gitlab_get_group_dora_metrics"),
	}
}

func doraMetricReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"analytics", "dora"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "dorametrics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	switch name {
	case "project":
		options.Usage = "Retrieves one DORA metric (deployment_frequency, lead_time_for_changes, time_to_restore_service, or change_failure_rate) for a project_id over a date window. interval is the bucket size (daily, monthly, all); for a prompt like `last 30 days`, compute start_date and end_date as YYYY-MM-DD and pass them — there is no `days` or `days_back` parameter."
		options.RelatedActions = []string{"dora_metrics.group"}
	case "group":
		options.Usage = "Retrieves one DORA metric (deployment_frequency, lead_time_for_changes, time_to_restore_service, or change_failure_rate) for a group_id over a date window. interval is the bucket size (daily, monthly, all); for a prompt like `last 30 days`, compute start_date and end_date as YYYY-MM-DD and pass them — there is no `days` or `days_back` parameter."
		options.RelatedActions = []string{"dora_metrics.project"}
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

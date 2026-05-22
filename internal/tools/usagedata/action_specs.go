package usagedata

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for usage data tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		usageDataReadSpec("usage_data_service_ping", toolutil.RouteAction(client, GetServicePing), "gitlab_get_service_ping"),
		usageDataReadSpec("usage_data_non_sql_metrics", toolutil.RouteAction(client, GetNonSQLMetrics), "gitlab_get_non_sql_metrics"),
		usageDataReadSpec("usage_data_queries", toolutil.RouteAction(client, GetQueries), "gitlab_get_usage_queries"),
		usageDataReadSpec("usage_data_metric_definitions", toolutil.RouteAction(client, GetMetricDefinitions), "gitlab_get_metric_definitions"),
		usageDataCreateSpec("usage_data_track_event", toolutil.RouteAction(client, TrackEvent), "gitlab_track_event"),
		usageDataCreateSpec("usage_data_track_events", toolutil.RouteAction(client, TrackEvents), "gitlab_track_events"),
	}
}

func usageDataReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, usageDataOptions(individualTool))
}

func usageDataCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, usageDataOptions(individualTool))
}

func usageDataOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "usage-data"},
		OpenWorld:      true,
		OwnerPackage:   "usagedata",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

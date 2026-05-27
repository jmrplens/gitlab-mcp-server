package appstatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for application statistics tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		applicationStatisticsReadSpec("app_statistics_get", toolutil.RouteAction(client, Get), "gitlab_get_application_statistics"),
	}
}

func applicationStatisticsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, applicationStatisticsOptions(individualTool))
}

func applicationStatisticsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"application statistics", "instance statistics", "gitlab statistics", "admin statistics"},
		Tags:           []string{"admin", "statistics", "instance"},
		Usage:          "Read GitLab instance-wide application statistics such as totals for users, groups, projects, issues, and merge requests. Requires administrator access.",
		RelatedActions: []string{"admin.metadata_get", "server.health_check"},
		OpenWorld:      true,
		OwnerPackage:   "appstatistics",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "Get GitLab application statistics for the current instance. Returns: aggregate counts for users, groups, projects, issues, merge requests, and related records. See also: gitlab_get_metadata, gitlab_server_status.",
		},
	}
}

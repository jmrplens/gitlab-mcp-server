package issuestatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue statistics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueStatisticsReadSpec("statistics_get", toolutil.RouteAction(client, Get), "gitlab_get_issue_statistics"),
		issueStatisticsReadSpec("statistics_get_group", toolutil.RouteAction(client, GetGroup), "gitlab_get_group_issue_statistics"),
		issueStatisticsReadSpec("statistics_get_project", toolutil.RouteAction(client, GetProject), "gitlab_get_project_issue_statistics"),
	}
}

func issueStatisticsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueStatisticsOptions(individualTool))
}

func issueStatisticsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issuestatistics domain action.", Tags: []string{"issue", "statistics"},
		OpenWorld:      true,
		OwnerPackage:   "issuestatistics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

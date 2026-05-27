package groupanalytics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group analytics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupAnalyticsReadSpec("analytics_issues_count", toolutil.RouteAction(client, GetIssuesCount), "gitlab_get_recently_created_issues_count"),
		groupAnalyticsReadSpec("analytics_mr_count", toolutil.RouteAction(client, GetMRCount), "gitlab_get_recently_created_mr_count"),
		groupAnalyticsReadSpec("analytics_members_count", toolutil.RouteAction(client, GetMembersCount), "gitlab_get_recently_added_members_count"),
	}
}

func groupAnalyticsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupAnalyticsOptions(individualTool))
}

func groupAnalyticsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupanalytics domain action.", Tags: []string{"group", "analytics"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupanalytics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

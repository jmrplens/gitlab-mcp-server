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
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute dorametrics domain action.", Tags: []string{"analytics", "dora"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "dorametrics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}

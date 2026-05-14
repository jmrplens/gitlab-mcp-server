package dorametrics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for DORA metric actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		doraMetricReadSpec("project", toolutil.RouteAction(client, GetProjectMetrics), "gitlab_get_project_dora_metrics"),
		doraMetricReadSpec("group", toolutil.RouteAction(client, GetGroupMetrics), "gitlab_get_group_dora_metrics"),
	}
}

func doraMetricReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"analytics", "dora"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "dorametrics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}

package freezeperiods

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deploy freeze period actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		freezePeriodReadSpec("freeze_list", toolutil.RouteAction(client, List), "gitlab_list_freeze_periods"),
		freezePeriodReadSpec("freeze_get", toolutil.RouteAction(client, Get), "gitlab_get_freeze_period"),
		freezePeriodCreateSpec("freeze_create", toolutil.RouteAction(client, Create), "gitlab_create_freeze_period"),
		freezePeriodUpdateSpec("freeze_update", toolutil.RouteAction(client, Update), "gitlab_update_freeze_period"),
		freezePeriodDeleteSpec("freeze_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_freeze_period"),
	}
}

func freezePeriodReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, freezePeriodOptions(individualTool))
}

func freezePeriodCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, freezePeriodOptions(individualTool))
}

func freezePeriodUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, freezePeriodOptions(individualTool))
}

func freezePeriodDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, freezePeriodOptions(individualTool))
}

func freezePeriodOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"environment", "freeze_period", "deployment"},
		RelatedActions: []string{"environment.list", "deployment.list", "pipeline.list"},
		OpenWorld:      true,
		OwnerPackage:   "freezeperiods",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

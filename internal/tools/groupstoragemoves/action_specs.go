package groupstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupStorageMoveReadSpec("retrieve_all_group", toolutil.RouteAction(client, RetrieveAll), "gitlab_retrieve_all_group_storage_moves"),
		groupStorageMoveReadSpec("retrieve_group", toolutil.RouteAction(client, RetrieveForGroup), "gitlab_retrieve_group_storage_moves"),
		groupStorageMoveReadSpec("get_group", toolutil.RouteAction(client, Get), "gitlab_get_group_storage_move"),
		groupStorageMoveReadSpec("get_group_for_group", toolutil.RouteAction(client, GetForGroup), "gitlab_get_group_storage_move_for_group"),
		groupStorageMoveCreateSpec("schedule_group", toolutil.RouteAction(client, Schedule), "gitlab_schedule_group_storage_move"),
		groupStorageMoveCreateSpec("schedule_all_group", toolutil.RouteAction(client, ScheduleAll), "gitlab_schedule_all_group_storage_moves"),
	}
}

func groupStorageMoveReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupStorageMoveOptions(individualTool))
}

func groupStorageMoveCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupStorageMoveOptions(individualTool))
}

func groupStorageMoveOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupstoragemoves domain action.", Tags: []string{"storage_move", "group"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "groupstoragemoves",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

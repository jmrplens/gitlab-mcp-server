package projectstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectStorageMoveReadSpec("retrieve_all_project", toolutil.RouteAction(client, RetrieveAll), "gitlab_retrieve_all_project_storage_moves"),
		projectStorageMoveReadSpec("retrieve_project", toolutil.RouteAction(client, RetrieveForProject), "gitlab_retrieve_project_storage_moves"),
		projectStorageMoveReadSpec("get_project", toolutil.RouteAction(client, Get), "gitlab_get_project_storage_move"),
		projectStorageMoveReadSpec("get_project_for_project", toolutil.RouteAction(client, GetForProject), "gitlab_get_project_storage_move_for_project"),
		projectStorageMoveCreateSpec("schedule_project", toolutil.RouteAction(client, Schedule), "gitlab_schedule_project_storage_move"),
		projectStorageMoveCreateSpec("schedule_all_project", toolutil.RouteAction(client, ScheduleAll), "gitlab_schedule_all_project_storage_moves"),
	}
}

func projectStorageMoveReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := projectStorageMoveOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectStorageMoveCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, projectStorageMoveOptions(individualTool))
}

func projectStorageMoveOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"storage_move", "project"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "projectstoragemoves",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

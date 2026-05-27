package externalstatuschecks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for external status check actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		externalStatusCheckReadSpec("list_project_checks", toolutil.RouteAction(client, ListProjectStatusChecks), "gitlab_list_project_status_checks"),
		externalStatusCheckReadSpec("list_project_mr_checks", toolutil.RouteAction(client, ListProjectMRExternalStatusChecks), "gitlab_list_project_mr_external_status_checks"),
		externalStatusCheckReadSpec("list_project", toolutil.RouteAction(client, ListProjectExternalStatusChecks), "gitlab_list_project_external_status_checks"),
		externalStatusCheckCreateSpec("create_project", toolutil.RouteAction(client, CreateProjectExternalStatusCheck), "gitlab_create_project_external_status_check"),
		externalStatusCheckDeleteSpec("delete_project", toolutil.DestructiveVoidAction(client, DeleteProjectExternalStatusCheck), "gitlab_delete_project_external_status_check"),
		externalStatusCheckUpdateSpec("update_project", toolutil.RouteAction(client, UpdateProjectExternalStatusCheck), "gitlab_update_project_external_status_check"),
		externalStatusCheckUpdateSpec("retry_project", toolutil.RouteVoidAction(client, RetryFailedExternalStatusCheckForProjectMR), "gitlab_retry_failed_external_status_check_for_project_mr"),
		externalStatusCheckUpdateSpec("set_project_mr_status", toolutil.RouteVoidAction(client, SetProjectMRExternalStatusCheckStatus), "gitlab_set_project_mr_external_status_check_status"),
	}
}

func externalStatusCheckReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, externalStatusCheckOptions(individualTool))
}

func externalStatusCheckCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, externalStatusCheckOptions(individualTool))
}

func externalStatusCheckUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, externalStatusCheckOptions(individualTool))
}

func externalStatusCheckDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, externalStatusCheckOptions(individualTool))
}

func externalStatusCheckOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute externalstatuschecks domain action.", Tags: []string{"external_status_check", "status_check"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "externalstatuschecks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

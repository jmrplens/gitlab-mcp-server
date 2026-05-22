package mrapprovalsettings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request approval settings actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		approvalSettingsReadSpec("approval_settings_group_get", toolutil.RouteAction(client, GetGroupSettings), "gitlab_get_group_mr_approval_settings"),
		approvalSettingsUpdateSpec("approval_settings_group_update", toolutil.RouteAction(client, UpdateGroupSettings), "gitlab_update_group_mr_approval_settings"),
		approvalSettingsReadSpec("approval_settings_project_get", toolutil.RouteAction(client, GetProjectSettings), "gitlab_get_project_mr_approval_settings"),
		approvalSettingsUpdateSpec("approval_settings_project_update", toolutil.RouteAction(client, UpdateProjectSettings), "gitlab_update_project_mr_approval_settings"),
	}
}

func approvalSettingsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, approvalSettingsOptions(individualTool))
}

func approvalSettingsUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, approvalSettingsOptions(individualTool))
}

func approvalSettingsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "approval_settings"},
		OpenWorld:      true,
		OwnerPackage:   "mrapprovalsettings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

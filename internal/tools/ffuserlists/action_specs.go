package ffuserlists

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for feature flag user list actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userListReadSpec("ff_user_list_list", toolutil.RouteAction(client, ListUserLists), "gitlab_ff_user_list_list"),
		userListReadSpec("ff_user_list_get", toolutil.RouteAction(client, GetUserList), "gitlab_ff_user_list_get"),
		userListCreateSpec("ff_user_list_create", toolutil.RouteAction(client, CreateUserList), "gitlab_ff_user_list_create"),
		userListUpdateSpec("ff_user_list_update", toolutil.RouteAction(client, UpdateUserList), "gitlab_ff_user_list_update"),
		userListDeleteSpec("ff_user_list_delete", toolutil.DestructiveAction(client, deleteUserListOutput), "gitlab_ff_user_list_delete"),
	}
}

func userListReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userListOptions(name, individualTool))
}

func userListCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userListOptions(name, individualTool))
}

func userListUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, userListOptions(name, individualTool))
}

func userListDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userListOptions(name, individualTool))
}

func userListOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute ffuserlists domain action.", Tags: []string{"feature_flags", "user_list", "rollout"},
		RelatedActions: []string{"feature_flags.feature_flag_get", "feature_flags.feature_flag_update"},
		OpenWorld:      true,
		OwnerPackage:   "ffuserlists",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "ff_user_list_get" || actionName == "ff_user_list_update" || actionName == "ff_user_list_delete" {
		options.Usage = "Read, update, or delete a feature flag user list by its user_list_iid returned from list or create operations."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"user_list_iid": {
				SemanticRole: "feature_flag_user_list_iid",
				ValueSource:  "Use the iid/user_list_iid returned by ff_user_list_list or ff_user_list_create.",
				CommonConfusions: []string{
					"Do not use the user list name as user_list_iid.",
					"The ID is project-scoped and distinct from the feature flag name.",
				},
			},
		}
	}
	return options
}

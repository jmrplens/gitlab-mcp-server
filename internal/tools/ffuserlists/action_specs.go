package ffuserlists

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for feature flag user list actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userListReadSpec("ff_user_list_list", toolutil.RouteAction(client, ListUserLists), "gitlab_ff_user_list_list"),
		userListReadSpec("ff_user_list_get", toolutil.RouteAction(client, GetUserList), "gitlab_ff_user_list_get"),
		userListCreateSpec("ff_user_list_create", toolutil.RouteAction(client, CreateUserList), "gitlab_ff_user_list_create"),
		userListUpdateSpec("ff_user_list_update", toolutil.RouteAction(client, UpdateUserList), "gitlab_ff_user_list_update"),
		userListDeleteSpec("ff_user_list_delete", toolutil.DestructiveVoidAction(client, DeleteUserList), "gitlab_ff_user_list_delete"),
	}
}

func userListReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userListOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userListCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, userListOptions(individualTool))
}

func userListUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userListOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userListDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userListOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userListOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"feature_flags", "user_list", "rollout"},
		RelatedActions: []string{"feature_flags.feature_flag_get", "feature_flags.feature_flag_update"},
		OpenWorld:      true,
		OwnerPackage:   "ffuserlists",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

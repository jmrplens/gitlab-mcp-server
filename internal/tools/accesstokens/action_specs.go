package accesstokens

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project, group, and personal access token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		accessTokenReadSpec("token_project_list", toolutil.RouteAction(client, ProjectList), "gitlab_project_access_token_list"),
		accessTokenReadSpec("token_project_get", toolutil.RouteAction(client, ProjectGet), "gitlab_project_access_token_get"),
		accessTokenCreateSpec("token_project_create", toolutil.RouteAction(client, ProjectCreate), "gitlab_project_access_token_create"),
		accessTokenCreateSpec("token_project_rotate", toolutil.RouteAction(client, ProjectRotate), "gitlab_project_access_token_rotate"),
		accessTokenCreateSpec("token_project_rotate_self", toolutil.RouteAction(client, ProjectRotateSelf), "gitlab_project_access_token_rotate_self"),
		accessTokenDeleteSpec("token_project_revoke", toolutil.DestructiveVoidAction(client, ProjectRevoke), "gitlab_project_access_token_revoke"),
		accessTokenReadSpec("token_group_list", toolutil.RouteAction(client, GroupList), "gitlab_group_access_token_list"),
		accessTokenReadSpec("token_group_get", toolutil.RouteAction(client, GroupGet), "gitlab_group_access_token_get"),
		accessTokenCreateSpec("token_group_create", toolutil.RouteAction(client, GroupCreate), "gitlab_group_access_token_create"),
		accessTokenCreateSpec("token_group_rotate", toolutil.RouteAction(client, GroupRotate), "gitlab_group_access_token_rotate"),
		accessTokenCreateSpec("token_group_rotate_self", toolutil.RouteAction(client, GroupRotateSelf), "gitlab_group_access_token_rotate_self"),
		accessTokenDeleteSpec("token_group_revoke", toolutil.DestructiveVoidAction(client, GroupRevoke), "gitlab_group_access_token_revoke"),
		accessTokenReadSpec("token_personal_list", toolutil.RouteAction(client, PersonalList), "gitlab_personal_access_token_list"),
		accessTokenReadSpec("token_personal_get", toolutil.RouteAction(client, PersonalGet), "gitlab_personal_access_token_get"),
		accessTokenCreateSpec("token_personal_rotate", toolutil.RouteAction(client, PersonalRotate), "gitlab_personal_access_token_rotate"),
		accessTokenCreateSpec("token_personal_rotate_self", toolutil.RouteAction(client, PersonalRotateSelf), "gitlab_personal_access_token_rotate_self"),
		accessTokenDeleteSpec("token_personal_revoke", toolutil.DestructiveVoidAction(client, PersonalRevoke), "gitlab_personal_access_token_revoke"),
		accessTokenDeleteSpec("token_personal_revoke_self", toolutil.DestructiveVoidAction(client, PersonalRevokeSelf), "gitlab_personal_access_token_revoke_self"),
	}
}

func accessTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := accessTokenOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func accessTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, accessTokenOptions(individualTool))
}

func accessTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := accessTokenOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func accessTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"access", "token"},
		OpenWorld:      true,
		OwnerPackage:   "accesstokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

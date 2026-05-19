package impersonationtokens

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for impersonation and user PAT actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userTokenReadSpec("list_impersonation_tokens", toolutil.RouteAction(client, List), "gitlab_list_impersonation_tokens"),
		userTokenReadSpec("get_impersonation_token", toolutil.RouteAction(client, Get), "gitlab_get_impersonation_token"),
		userTokenCreateSpec("create_impersonation_token", toolutil.RouteAction(client, Create), "gitlab_create_impersonation_token"),
		userTokenDeleteSpec("revoke_impersonation_token", toolutil.DestructiveAction(client, Revoke), "gitlab_revoke_impersonation_token"),
		userTokenCreateSpec("create_personal_access_token", toolutil.RouteAction(client, CreatePAT), "gitlab_create_personal_access_token"),
	}
}

func userTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userTokenOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, userTokenOptions(individualTool))
}

func userTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userTokenOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"user", "token"},
		OpenWorld:      true,
		OwnerPackage:   "impersonationtokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

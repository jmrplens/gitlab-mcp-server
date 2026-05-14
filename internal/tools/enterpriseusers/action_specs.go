package enterpriseusers

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for enterprise user actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		enterpriseUserReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_enterprise_users"),
		enterpriseUserReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_enterprise_user"),
		enterpriseUserDestructiveSpec("disable_2fa", toolutil.DestructiveVoidAction(client, Disable2FA), "gitlab_disable_2fa_enterprise_user"),
		enterpriseUserDestructiveSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_enterprise_user"),
	}
}

func enterpriseUserReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := enterpriseUserOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func enterpriseUserDestructiveSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := enterpriseUserOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func enterpriseUserOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"enterprise_user"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "enterpriseusers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

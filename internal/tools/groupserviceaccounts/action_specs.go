package groupserviceaccounts

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group service account actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupServiceAccountReadSpec("service_account_list", toolutil.RouteAction(client, List), "gitlab_group_service_account_list"),
		groupServiceAccountCreateSpec("service_account_create", toolutil.RouteAction(client, Create), "gitlab_group_service_account_create"),
		groupServiceAccountUpdateSpec("service_account_update", toolutil.RouteAction(client, Update), "gitlab_group_service_account_update"),
		groupServiceAccountDeleteSpec("service_account_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_service_account_delete"),
		groupServiceAccountReadSpec("service_account_pat_list", toolutil.RouteAction(client, ListPATs), "gitlab_group_service_account_pat_list"),
		groupServiceAccountCreateSpec("service_account_pat_create", toolutil.RouteAction(client, CreatePAT), "gitlab_group_service_account_pat_create"),
		groupServiceAccountDeleteSpec("service_account_pat_revoke", toolutil.DestructiveVoidAction(client, RevokePAT), "gitlab_group_service_account_pat_revoke"),
	}
}

func groupServiceAccountReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupServiceAccountOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupServiceAccountCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupServiceAccountOptions(individualTool))
}

func groupServiceAccountUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupServiceAccountOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupServiceAccountDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupServiceAccountOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupServiceAccountOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "service-account"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupserviceaccounts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

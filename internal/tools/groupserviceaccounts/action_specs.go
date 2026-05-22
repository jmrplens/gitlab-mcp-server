package groupserviceaccounts

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, groupServiceAccountOptions(individualTool))
}

func groupServiceAccountCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupServiceAccountOptions(individualTool))
}

func groupServiceAccountUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupServiceAccountOptions(individualTool))
}

func groupServiceAccountDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupServiceAccountOptions(individualTool))
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

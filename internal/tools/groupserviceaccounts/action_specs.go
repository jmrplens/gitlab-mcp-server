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
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"group", "service-account"},
		Usage:          "Use for GitLab group service accounts and their personal access tokens. Do not use group members, SCIM identities, enterprise users, or generic group access tokens for service account CRUD. Requires GitLab Premium/Ultimate and Owner permissions.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupserviceaccounts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if individualTool == "gitlab_group_service_account_create" || individualTool == "gitlab_group_service_account_update" {
		options.Usage += " Omit email unless the task gives an explicit valid email address."
	}
	if individualTool == "gitlab_group_service_account_pat_create" {
		options.Usage += " Omit expires_at unless the task gives an explicit expiry date; if provided, use YYYY-MM-DD within the instance maximum token lifetime."
	}
	if individualTool == "gitlab_group_service_account_pat_revoke" {
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"token_id": {
				SemanticRole:     "access_token",
				ValueSource:      "Group service account personal access token ID returned by service_account_pat_list or service_account_pat_create.",
				CommonConfusions: []string{"Do not use service_account_id as token_id; token_id identifies the personal access token itself."},
			},
		}
	}
	return options
}

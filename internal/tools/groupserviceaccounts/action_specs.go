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
	return toolutil.NewReadActionSpec(name, route, groupServiceAccountOptions(name, individualTool))
}

func groupServiceAccountCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupServiceAccountOptions(name, individualTool))
}

func groupServiceAccountUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupServiceAccountOptions(name, individualTool))
}

func groupServiceAccountDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupServiceAccountOptions(name, individualTool))
}

func groupServiceAccountOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        groupServiceAccountAliases(actionName),
		Tags:           groupServiceAccountTags(actionName),
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

func groupServiceAccountTags(actionName string) []string {
	tags := []string{"group", "service-account"}
	switch actionName {
	case "service_account_pat_list", "service_account_pat_create", "service_account_pat_revoke":
		tags = append(tags, "service-account-pat")
	}
	return tags
}

func groupServiceAccountAliases(actionName string) []string {
	var aliases []string
	switch actionName {
	case "service_account_list":
		aliases = append(aliases, "group service account list", "list group service accounts")
	case "service_account_create":
		aliases = append(aliases, "group service account create", "create group service account")
	case "service_account_update":
		aliases = append(aliases, "group service account update", "update group service account")
	case "service_account_delete":
		aliases = append(aliases, "group service account delete", "delete group service account")
	case "service_account_pat_list":
		aliases = append(aliases, groupServiceAccountPATAliases("list")...)
	case "service_account_pat_create":
		aliases = append(aliases, groupServiceAccountPATAliases("create")...)
	case "service_account_pat_revoke":
		aliases = append(aliases, groupServiceAccountPATAliases("revoke")...)
	}
	return aliases
}

func groupServiceAccountPATAliases(verb string) []string {
	return []string{
		"group service account personal access token " + verb,
		verb + " group service account personal access tokens",
		verb + " group service account personal access token",
		"group service account pat " + verb,
		verb + " token for group service account",
	}
}

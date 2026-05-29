package projectserviceaccounts

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project service account actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectServiceAccountReadSpec("service_account_list", toolutil.RouteAction(client, List), "gitlab_project_service_account_list"),
		projectServiceAccountCreateSpec("service_account_create", toolutil.RouteAction(client, Create), "gitlab_project_service_account_create"),
		projectServiceAccountUpdateSpec("service_account_update", toolutil.RouteAction(client, Update), "gitlab_project_service_account_update"),
		projectServiceAccountDeleteSpec("service_account_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_project_service_account_delete"),
		projectServiceAccountReadSpec("service_account_pat_list", toolutil.RouteAction(client, ListPATs), "gitlab_project_service_account_pat_list"),
		projectServiceAccountCreateSpec("service_account_pat_create", toolutil.RouteAction(client, CreatePAT), "gitlab_project_service_account_pat_create"),
		projectServiceAccountCreateSpec("service_account_pat_rotate", toolutil.RouteAction(client, RotatePAT), "gitlab_project_service_account_pat_rotate"),
		projectServiceAccountDeleteSpec("service_account_pat_revoke", toolutil.DestructiveVoidAction(client, RevokePAT), "gitlab_project_service_account_pat_revoke"),
	}
}

func projectServiceAccountReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectServiceAccountOptions(name, individualTool))
}

func projectServiceAccountCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectServiceAccountOptions(name, individualTool))
}

func projectServiceAccountUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, projectServiceAccountOptions(name, individualTool))
}

func projectServiceAccountDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, projectServiceAccountOptions(name, individualTool))
}

func projectServiceAccountOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        projectServiceAccountAliases(actionName),
		Tags:           projectServiceAccountTags(actionName),
		Usage:          "Use for GitLab project service accounts and their personal access tokens. Requires GitLab Premium/Ultimate and sufficient project permissions.",
		RelatedActions: []string{"project.get", "project.members"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "projectserviceaccounts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: projectServiceAccountDescription(actionName)},
	}
	if actionName == "service_account_pat_rotate" || actionName == "service_account_pat_revoke" {
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"token_id": {
				SemanticRole:     "access_token",
				ValueSource:      "Project service account personal access token ID returned by service_account_pat_list or service_account_pat_create.",
				CommonConfusions: []string{"Do not use service_account_id as token_id; token_id identifies the personal access token itself."},
			},
		}
	}
	if actionName == "service_account_create" || actionName == "service_account_update" {
		options.Usage += " Omit email unless the task gives an explicit valid email address."
	}
	if actionName == "service_account_pat_create" || actionName == "service_account_pat_rotate" {
		options.Usage += " Omit expires_at unless the task gives an explicit expiry date; if provided, use YYYY-MM-DD within the instance maximum token lifetime."
	}
	return options
}

func projectServiceAccountDescription(actionName string) string {
	switch actionName {
	case "service_account_list":
		return "List GitLab project service accounts. Returns: paginated project service account records. See also: gitlab_get_project, gitlab_project_service_account_create, gitlab_project_service_account_pat_list."
	case "service_account_create":
		return "Create a GitLab project service account. Returns: the created project service account object. See also: gitlab_project_service_account_list, gitlab_project_service_account_update, gitlab_project_service_account_pat_create."
	case "service_account_update":
		return "Update a GitLab project service account. Returns: the updated project service account object. See also: gitlab_project_service_account_list, gitlab_project_service_account_delete."
	case "service_account_delete":
		return "Delete a GitLab project service account. Returns: a success status confirming deletion. See also: gitlab_project_service_account_list, gitlab_project_service_account_update."
	case "service_account_pat_list":
		return "List personal access tokens for a GitLab project service account. Returns: paginated token records. See also: gitlab_project_service_account_list, gitlab_project_service_account_pat_create, gitlab_project_service_account_pat_revoke."
	case "service_account_pat_create":
		return "Create a personal access token for a GitLab project service account. Returns: the created token record and token value when GitLab returns it. See also: gitlab_project_service_account_pat_list, gitlab_project_service_account_pat_rotate."
	case "service_account_pat_rotate":
		return "Rotate a personal access token for a GitLab project service account. Returns: the rotated token record and token value when GitLab returns it. See also: gitlab_project_service_account_pat_list, gitlab_project_service_account_pat_revoke."
	case "service_account_pat_revoke":
		return "Revoke a personal access token for a GitLab project service account. Returns: a success status confirming revocation. See also: gitlab_project_service_account_pat_list, gitlab_project_service_account_pat_create."
	default:
		return "Manage GitLab project service accounts. Returns: project service account data or operation status. See also: gitlab_get_project."
	}
}

func projectServiceAccountTags(actionName string) []string {
	tags := []string{"project", "service-account"}
	switch actionName {
	case "service_account_pat_list", "service_account_pat_create", "service_account_pat_rotate", "service_account_pat_revoke":
		tags = append(tags, "service-account-pat")
	}
	return tags
}

func projectServiceAccountAliases(actionName string) []string {
	var aliases []string
	switch actionName {
	case "service_account_list":
		aliases = append(aliases, "project service account list", "list project service accounts")
	case "service_account_create":
		aliases = append(aliases, "project service account create", "create project service account")
	case "service_account_update":
		aliases = append(aliases, "project service account update", "update project service account")
	case "service_account_delete":
		aliases = append(aliases, "project service account delete", "delete project service account")
	case "service_account_pat_list":
		aliases = append(aliases, projectServiceAccountPATAliases("list")...)
	case "service_account_pat_create":
		aliases = append(aliases, projectServiceAccountPATAliases("create")...)
	case "service_account_pat_rotate":
		aliases = append(aliases, projectServiceAccountPATAliases("rotate")...)
	case "service_account_pat_revoke":
		aliases = append(aliases, projectServiceAccountPATAliases("revoke")...)
	}
	return aliases
}

func projectServiceAccountPATAliases(verb string) []string {
	return []string{
		"project service account personal access token " + verb,
		verb + " project service account personal access tokens",
		verb + " project service account personal access token",
		"project service account pat " + verb,
		verb + " token for project service account",
	}
}

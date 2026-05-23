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
		Tags:           []string{"project", "service-account"},
		Usage:          "Use for GitLab project service accounts and their personal access tokens. Requires GitLab Premium/Ultimate and sufficient project permissions.",
		RelatedActions: []string{"project.get", "project.members"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "projectserviceaccounts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
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

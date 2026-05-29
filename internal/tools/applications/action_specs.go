package applications

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for OAuth application tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		applicationReadSpec("application_list", toolutil.RouteAction(client, List), "gitlab_list_applications"),
		applicationCreateSpec("application_create", toolutil.RouteAction(client, Create), "gitlab_create_application"),
		applicationDeleteSpec("application_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_application"),
	}
}

func applicationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, applicationOptionsForAction(name, individualTool))
}

func applicationCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, applicationOptionsForAction(name, individualTool))
}

func applicationDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, applicationOptionsForAction(name, individualTool))
}

func applicationOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute applications domain action.", Tags: []string{"admin", "application"},
		RelatedActions: []string{"admin.settings_get", "admin.metadata_get"},
		OpenWorld:      true,
		OwnerPackage:   "applications",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "application_list":
		options.Usage = "List OAuth applications configured in the GitLab instance. Use this before rotating secrets or deleting stale OAuth clients."
		options.Aliases = []string{"list applications", "list oauth apps", "show oauth clients"}
		options.IndividualTool.Description = "List OAuth applications configured in GitLab. Returns: application IDs, names, callback URLs, and confidentiality flags. See also: gitlab_create_application, gitlab_delete_application."
	case "application_create":
		options.Usage = "Create an OAuth application with name, redirect URI, and scopes. Use for integrating external clients with GitLab OAuth."
		options.Aliases = []string{"create application", "create oauth app", "register oauth client"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"name": {
				SemanticRole:   "oauth_application_name",
				ValueSource:    "Human-readable OAuth app name requested by the task.",
				ExampleBinding: `params.name:"CI Dashboard"`,
			},
			"redirect_uri": {
				SemanticRole:   "oauth_redirect_uri",
				ValueSource:    "Authorized callback URL for OAuth flow.",
				ExampleBinding: `params.redirect_uri:"https://example.com/oauth/callback"`,
			},
			"scopes": {
				SemanticRole:   "oauth_scopes",
				ValueSource:    "Space-delimited scope list supported by GitLab OAuth.",
				ExampleBinding: `params.scopes:"read_user api"`,
			},
		}
		options.IndividualTool.Description = "Create an OAuth application in GitLab. Returns: created application record including client ID and secret. See also: gitlab_list_applications, gitlab_delete_application."
	case "application_delete":
		options.Usage = "Delete an OAuth application by numeric id. Use only after confirming the client is no longer in use."
		options.Aliases = []string{"delete application", "remove oauth app", "revoke oauth client"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"id": {
				SemanticRole:   "oauth_application_id",
				ValueSource:    "Numeric application ID from list output.",
				ExampleBinding: "params.id:12",
			},
		}
		options.IndividualTool.Description = "Delete an OAuth application by id. Returns: success confirmation when deletion completes. See also: gitlab_list_applications."
	}

	return options
}

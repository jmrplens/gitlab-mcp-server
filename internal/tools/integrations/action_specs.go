package integrations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project integration actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		integrationReadSpec("integration_list", toolutil.RouteAction(client, List), "gitlab_list_integrations"),
		integrationReadSpec("integration_get", toolutil.RouteAction(client, Get), "gitlab_get_integration"),
		integrationDeleteSpec("integration_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_integration"),
		integrationCreateSpec("integration_set_jira", toolutil.RouteAction(client, SetJira), "gitlab_set_jira_integration"),
	}
}

func integrationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := integrationOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func integrationCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, integrationOptions(individualTool))
}

func integrationDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := integrationOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func integrationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "integration"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "integrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

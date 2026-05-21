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
	return toolutil.NewReadActionSpec(name, route, applicationOptions(individualTool))
}

func applicationCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, applicationOptions(individualTool))
}

func applicationDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, applicationOptions(individualTool))
}

func applicationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "application"},
		OpenWorld:      true,
		OwnerPackage:   "applications",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

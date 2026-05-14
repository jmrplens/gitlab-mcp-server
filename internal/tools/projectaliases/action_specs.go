package projectaliases

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project alias actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectAliasReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_project_aliases"),
		projectAliasReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_project_alias"),
		projectAliasCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_project_alias"),
		projectAliasDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_project_alias"),
	}
}

func projectAliasReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := projectAliasOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectAliasCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := projectAliasOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectAliasOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "alias"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "projectaliases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

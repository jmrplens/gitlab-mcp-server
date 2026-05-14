package epics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group epic actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicReadSpec("epic_list", toolutil.RouteAction(client, List), "gitlab_epic_list"),
		epicReadSpec("epic_get", toolutil.RouteAction(client, Get), "gitlab_epic_get"),
		epicReadSpec("epic_get_links", toolutil.RouteAction(client, GetLinks), "gitlab_epic_get_links"),
		epicCreateSpec("epic_create", toolutil.RouteAction(client, Create), "gitlab_epic_create"),
		epicUpdateSpec("epic_update", toolutil.RouteAction(client, Update), "gitlab_epic_update"),
		epicDeleteSpec("epic_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_epic_delete"),
	}
}

func epicReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, epicOptions(individualTool))
}

func epicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "epic"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

package releases

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for release actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		releaseCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_release_create"),
		releaseReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_release_get"),
		releaseReadSpec("get_latest", toolutil.RouteAction(client, GetLatest), "gitlab_release_latest"),
		releaseReadSpec("list", toolutil.RouteAction(client, List), "gitlab_release_list"),
		releaseUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_release_update"),
		releaseDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_release_delete"),
	}
}

func releaseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, releaseOptions(individualTool))
}

func releaseUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"release", "tag", "asset"},
		RelatedActions: []string{"tag.get", "package.list", "project.milestone_list"},
		OpenWorld:      true,
		OwnerPackage:   "releases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

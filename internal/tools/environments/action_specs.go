package environments

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		environmentReadSpec("list", toolutil.RouteAction(client, List), "gitlab_environment_list"),
		environmentReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_environment_get"),
		environmentCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_environment_create"),
		environmentUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_environment_update"),
		environmentDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_environment_delete"),
		environmentStopSpec(client),
	}
}

func environmentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := environmentOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func environmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, environmentOptions(individualTool))
}

func environmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := environmentOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func environmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := environmentOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func environmentStopSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := environmentOptions("gitlab_environment_stop")
	options.Destructive = true
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewActionSpec("stop", toolutil.DestructiveAction(client, Stop), options)
}

func environmentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"environment", "deployment"},
		RelatedActions: []string{"deployment.list", "ci_variable.list", "feature_flags.strategy_list"},
		OpenWorld:      true,
		OwnerPackage:   "environments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

package environments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		environmentReadSpec("list", toolutil.RouteAction(client, List), "gitlab_environment_list"),
		environmentReadSpec("get", environmentGetRoute(client), "gitlab_environment_get"),
		environmentCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_environment_create"),
		environmentUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_environment_update"),
		environmentDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_environment_delete"),
		environmentStopSpec(client),
	}
}

func environmentGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return environmentNotFoundOutput{Identifier: fmt.Sprintf("ID %v in project %v", input["environment_id"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func environmentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, environmentOptions(individualTool))
}

func environmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, environmentOptions(individualTool))
}

func environmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, environmentOptions(individualTool))
}

func environmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, environmentOptions(individualTool))
}

func environmentStopSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := environmentOptions("gitlab_environment_stop")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("stop", toolutil.DestructiveAction(client, Stop), options)
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

package releases

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for release actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		releaseCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_release_create"),
		releaseReadSpec("get", releaseGetRoute(client), "gitlab_release_get"),
		releaseReadSpec("get_latest", toolutil.RouteAction(client, GetLatest), "gitlab_release_latest"),
		releaseReadSpec("list", toolutil.RouteAction(client, List), "gitlab_release_list"),
		releaseUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_release_update"),
		releaseDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_release_delete"),
	}
}

func releaseGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			tagName, _ := input["tag_name"].(string)
			return releaseNotFoundOutput{Identifier: fmt.Sprintf("tag %q in project %v", tagName, input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func releaseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, releaseOptions(individualTool))
}

func releaseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, releaseOptions(individualTool))
}

func releaseUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, releaseOptions(individualTool))
}

func releaseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, releaseOptions(individualTool))
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

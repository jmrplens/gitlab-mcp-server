package snippets

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for personal and project snippet actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	createRoute := toolutil.RouteAction(client, Create)
	createRoute.InputSchema = CreateInputSchemaMap()
	projectCreateRoute := toolutil.RouteAction(client, ProjectCreate)
	projectCreateRoute.InputSchema = ProjectCreateInputSchemaMap()

	return []toolutil.ActionSpec{
		snippetReadSpec("list", toolutil.RouteAction(client, List), "gitlab_snippet_list"),
		snippetReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_snippet_list_all"),
		snippetReadSpec("get", snippetGetRoute(client), "gitlab_snippet_get"),
		snippetReadSpec("content", toolutil.RouteAction(client, Content), "gitlab_snippet_content"),
		snippetReadSpec("file_content", toolutil.RouteAction(client, FileContent), "gitlab_snippet_file_content"),
		snippetCreateSpec("create", createRoute, "gitlab_snippet_create"),
		snippetUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_snippet_update"),
		snippetDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_snippet_delete"),
		snippetReadSpec("explore", toolutil.RouteAction(client, Explore), "gitlab_snippet_explore"),
		snippetReadSpec("project_list", toolutil.RouteAction(client, ProjectList), "gitlab_project_snippet_list"),
		snippetReadSpec("project_get", toolutil.RouteAction(client, ProjectGet), "gitlab_project_snippet_get"),
		snippetReadSpec("project_content", toolutil.RouteAction(client, ProjectContent), "gitlab_project_snippet_content"),
		snippetCreateSpec("project_create", projectCreateRoute, "gitlab_project_snippet_create"),
		snippetUpdateSpec("project_update", toolutil.RouteAction(client, ProjectUpdate), "gitlab_project_snippet_update"),
		snippetDeleteSpec("project_delete", toolutil.DestructiveVoidAction(client, ProjectDelete), "gitlab_project_snippet_delete"),
	}
}

func snippetGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return snippetNotFoundOutput{Identifier: fmt.Sprintf("ID %v", input["snippet_id"])}, nil
		}
		return result, err
	}
	return route
}

func snippetReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetOptions(individualTool))
}

func snippetCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetOptions(individualTool))
}

func snippetUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, snippetOptions(individualTool))
}

func snippetDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetOptions(individualTool))
}

func snippetOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute snippets domain action.", Tags: []string{"snippet"},
		OpenWorld:      true,
		OwnerPackage:   "snippets",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

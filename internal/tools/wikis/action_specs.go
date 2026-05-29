package wikis

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		wikiReadSpec("list", toolutil.RouteAction(client, List), "gitlab_wiki_list"),
		wikiReadSpec("get", wikiGetRoute(client), "gitlab_wiki_get"),
		wikiCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_wiki_create"),
		wikiUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_wiki_update"),
		wikiDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_wiki_delete"),
		wikiCreateSpec("upload_attachment", toolutil.RouteAction(client, UploadAttachment), "gitlab_wiki_upload_attachment"),
	}
}

func wikiGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			slug, _ := input["slug"].(string)
			return wikiNotFoundOutput{Identifier: fmt.Sprintf("slug %q in project %v", slug, input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func wikiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, wikiOptions(individualTool))
}

func wikiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, wikiOptions(individualTool))
}

func wikiUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, wikiOptions(individualTool))
}

func wikiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, wikiOptions(individualTool))
}

func wikiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute wikis domain action.", Tags: []string{"wiki"},
		RelatedActions: []string{"wiki.list", "wiki.get", "project.get", "repository.file_get"},
		OpenWorld:      true,
		OwnerPackage:   "wikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

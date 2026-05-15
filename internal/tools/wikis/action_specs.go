package wikis

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		wikiSpec("list", toolutil.RouteAction(client, List), "gitlab_wiki_list", true, true),
		wikiSpec("get", wikiGetRoute(client), "gitlab_wiki_get", true, true),
		wikiSpec("create", toolutil.RouteAction(client, Create), "gitlab_wiki_create", false, false),
		wikiSpec("update", toolutil.RouteAction(client, Update), "gitlab_wiki_update", false, true),
		wikiSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_wiki_delete", false, true),
		wikiSpec("upload_attachment", toolutil.RouteAction(client, UploadAttachment), "gitlab_wiki_upload_attachment", false, false),
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

func wikiSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"wiki"},
		RelatedActions: []string{"wiki.list", "wiki.get", "project.get", "repository.file_get"},
		ReadOnly:       readOnly,
		Idempotent:     idempotent,
		OpenWorld:      true,
		OwnerPackage:   "wikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}

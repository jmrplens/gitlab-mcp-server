package tags

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for tag and protected tag actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		tagSpec("create", toolutil.RouteAction(client, Create), "gitlab_tag_create", false, false),
		tagSpec("get", tagGetRoute(client), "gitlab_tag_get", true, true),
		tagSpec("list", toolutil.RouteAction(client, List), "gitlab_tag_list", true, true),
		tagSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_tag_delete", false, true),
		tagSpec("get_signature", toolutil.RouteAction(client, GetSignature), "gitlab_tag_get_signature", true, true),
		tagSpec("list_protected", toolutil.RouteAction(client, ListProtectedTags), "gitlab_tag_list_protected", true, true),
		tagSpec("get_protected", toolutil.RouteAction(client, GetProtectedTag), "gitlab_tag_get_protected", true, true),
		tagSpec("protect", toolutil.RouteAction(client, ProtectTag), "gitlab_tag_protect", false, false),
		tagSpec("unprotect", toolutil.DestructiveVoidAction(client, UnprotectTag), "gitlab_tag_unprotect", false, true),
	}
}

func tagGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			tagName, _ := input["tag_name"].(string)
			projectID, _ := input["project_id"].(string)
			return tagNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", tagName, projectID)}, nil
		}
		return result, err
	}
	return route
}

func tagSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"tag"},
		RelatedActions: []string{"tag.list", "tag.get", "release.get", "repository.commit_get"},
		ReadOnly:       readOnly,
		Idempotent:     idempotent,
		OpenWorld:      true,
		OwnerPackage:   "tags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}

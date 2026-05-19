package labels

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project label actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		labelReadSpec("label_list", toolutil.RouteAction(client, List), "gitlab_label_list"),
		labelReadSpec("label_get", labelGetRoute(client), "gitlab_label_get"),
		labelCreateSpec("label_create", toolutil.RouteAction(client, Create), "gitlab_label_create"),
		labelUpdateSpec("label_update", toolutil.RouteAction(client, Update), "gitlab_label_update"),
		labelDeleteSpec("label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_label_delete"),
		labelUpdateSpec("label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_label_subscribe"),
		labelUpdateSpec("label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_label_unsubscribe"),
		labelUpdateSpec("label_promote", toolutil.RouteVoidAction(client, Promote), "gitlab_label_promote"),
	}
}

func labelGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			labelID, _ := input["label_id"].(string)
			projectID, _ := input["project_id"].(string)
			return labelNotFoundOutput{Identifier: fmt.Sprintf("ID %s in project %s", labelID, projectID)}, nil
		}
		return result, err
	}
	return route
}

func labelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, labelOptions(individualTool))
}

func labelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := labelOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func labelOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "label"},
		RelatedActions: []string{"project.get", "issue.list"},
		OpenWorld:      true,
		OwnerPackage:   "labels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

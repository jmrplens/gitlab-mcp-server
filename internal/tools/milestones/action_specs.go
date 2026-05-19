package milestones

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project milestone actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		milestoneReadSpec("milestone_list", toolutil.RouteAction(client, List), "gitlab_milestone_list"),
		milestoneReadSpec("milestone_get", milestoneGetRoute(client), "gitlab_milestone_get"),
		milestoneCreateSpec("milestone_create", toolutil.RouteAction(client, Create), "gitlab_milestone_create"),
		milestoneUpdateSpec("milestone_update", toolutil.RouteAction(client, Update), "gitlab_milestone_update"),
		milestoneDeleteSpec("milestone_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_milestone_delete"),
		milestoneReadSpec("milestone_issues", toolutil.RouteAction(client, GetIssues), "gitlab_milestone_issues"),
		milestoneReadSpec("milestone_merge_requests", toolutil.RouteAction(client, GetMergeRequests), "gitlab_milestone_merge_requests"),
	}
}

func milestoneGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			projectID, _ := input["project_id"].(string)
			return milestoneNotFoundOutput{Identifier: fmt.Sprintf("IID %v in project %s", input["milestone_iid"], projectID)}, nil
		}
		return result, err
	}
	return route
}

func milestoneReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := milestoneOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func milestoneCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, milestoneOptions(individualTool))
}

func milestoneUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := milestoneOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func milestoneDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := milestoneOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func milestoneOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "milestone"},
		RelatedActions: []string{"project.get", "issue.list"},
		OpenWorld:      true,
		OwnerPackage:   "milestones",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

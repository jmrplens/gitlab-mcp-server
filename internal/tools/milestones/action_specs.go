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
	return toolutil.NewReadActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

func milestoneCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

func milestoneUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

func milestoneDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

func milestoneOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute milestones domain action.", Tags: []string{"project", "milestone"},
		RelatedActions: []string{"project.get", "issue.list"},
		OpenWorld:      true,
		OwnerPackage:   "milestones",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "milestone_list":
		options.Usage = "List milestones in one project with optional state/search filters and pagination. Use for release planning and progress overviews."
		options.Aliases = []string{"list milestones", "show project milestones", "find milestones"}
		options.RelatedActions = []string{"milestone.get", "milestone.issues", "milestone.merge_requests"}
	case "milestone_get":
		options.Usage = "Get one milestone by project_id and milestone_iid. Use this when a specific milestone is referenced and detailed fields are required."
		options.Aliases = []string{"get milestone", "show milestone details", "lookup milestone"}
		options.RelatedActions = []string{"milestone.list", "milestone.issues", "milestone.merge_requests"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"milestone_iid": {
				SemanticRole:     "milestone_iid",
				ValueSource:      "Milestone IID from milestone list output or URL context.",
				ExampleBinding:   "params.milestone_iid:3",
				CommonConfusions: []string{"Use milestone_iid (project-scoped IID), not global milestone ID."},
			},
		}
	case "milestone_create":
		options.Usage = "Create a milestone in a project with title and optional description/start/due dates."
		options.Aliases = []string{"create milestone", "new milestone", "add milestone"}
		options.RelatedActions = []string{"milestone.get", "milestone.update", "issue.list"}
	}

	return options
}

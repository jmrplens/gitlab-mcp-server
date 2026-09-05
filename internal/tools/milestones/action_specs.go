package milestones

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionMilestoneGet           = "milestone.get"
	actionMilestoneList          = "milestone.list"
	actionMilestoneMergeRequests = "milestone.merge_requests"
	actionMilestoneIssues        = "milestone.issues"
	actionIssueList              = "issue.list"
	paramMilestoneIID            = "milestone_iid"
)

// ActionSpecs returns canonical specs for project milestone actions
// exposed as MCP tools. The list, get, create, update, delete, issues,
// and merge_requests routes are projected into the dynamic, meta,
// individual, and audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_milestone_list — list project milestones with optional filters.
		milestoneReadSpec("milestone_list", toolutil.RouteAction(client, List), "gitlab_milestone_list"),
		// gitlab_milestone_get — fetch a milestone by IID (returns a structured not-found result on 404).
		milestoneReadSpec("milestone_get", milestoneGetRoute(client), "gitlab_milestone_get").
			WithEmbeddedResource("gitlab://project/{project_id}/milestone/{milestone_iid}"),
		// gitlab_milestone_create — create a new milestone.
		milestoneCreateSpec("milestone_create", toolutil.RouteAction(client, Create), "gitlab_milestone_create"),
		// gitlab_milestone_update — update an existing milestone.
		milestoneUpdateSpec("milestone_update", toolutil.RouteAction(client, Update), "gitlab_milestone_update"),
		// gitlab_milestone_delete — remove a milestone (destructive).
		milestoneDeleteSpec("milestone_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_milestone_delete"),
		// gitlab_milestone_issues — list issues assigned to a milestone.
		milestoneReadSpec("milestone_issues", toolutil.RouteAction(client, GetIssues), "gitlab_milestone_issues"),
		// gitlab_milestone_merge_requests — list MRs assigned to a milestone.
		milestoneReadSpec("milestone_merge_requests", toolutil.RouteAction(client, GetMergeRequests), "gitlab_milestone_merge_requests"),
	}
}

// milestoneGetRoute wraps the [Get] route so a 404 response is
// converted into a structured [milestoneNotFoundOutput] hint rather
// than an error, matching the get-not-found pattern used across the
// project.
func milestoneGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			projectID, _ := input["project_id"].(string)
			return milestoneNotFoundOutput{Identifier: fmt.Sprintf("IID %v in project %s", input[paramMilestoneIID], projectID)}, nil
		}
		return result, err
	}
	return route
}

// milestoneReadSpec builds a read-only [toolutil.ActionSpec] for a
// milestone action using the package's default
// [milestoneOptionsForAction].
func milestoneReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

// milestoneCreateSpec builds a create-style [toolutil.ActionSpec] for
// a milestone action using the package's default
// [milestoneOptionsForAction].
func milestoneCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

// milestoneUpdateSpec builds an update-style [toolutil.ActionSpec] for
// a milestone action using the package's default
// [milestoneOptionsForAction].
func milestoneUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

// milestoneDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// milestone action using the package's default
// [milestoneOptionsForAction].
func milestoneDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, milestoneOptionsForAction(name, individualTool))
}

// milestoneOptionsForAction returns the base [toolutil.ActionSpecOptions]
// for a milestone action and customizes the Usage/Aliases/ParameterGuidance
// for the list, get, and create individual tools.
func milestoneOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute milestones domain action.", Tags: []string{"project", "milestone"},
		RelatedActions: []string{"project.get", actionIssueList},
		OpenWorld:      true,
		OwnerPackage:   "milestones",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "milestone_list":
		options.Usage = "List milestones in one project with optional state/search filters and pagination. Use for release planning and progress overviews."
		options.Aliases = []string{"list milestones", "show project milestones", "find milestones"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneIssues, actionMilestoneMergeRequests}
		options.IndividualTool.Description = "List milestones in one project with state/title/search filters, ancestor inclusion, ordering, and pagination. Returns: matching milestones with state, dates, expiry, web URL, and pagination metadata. See also: gitlab_milestone_get, gitlab_milestone_create, gitlab_milestone_issues."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"active", "closed"}}),
		}
	case "milestone_get":
		options.Usage = "Get one milestone by project_id and milestone_iid. Use this when a specific milestone is referenced and detailed fields are required."
		options.Aliases = []string{"get milestone", "show milestone details", "lookup milestone"}
		options.RelatedActions = []string{actionMilestoneList, actionMilestoneIssues, actionMilestoneMergeRequests}
		options.IndividualTool.Description = "Get a single milestone from a project by milestone IID. Returns: milestone title, description, state, start/due dates, expiry, group/project IDs, timestamps, and web URL. See also: gitlab_milestone_list, gitlab_milestone_update, gitlab_milestone_issues, gitlab_milestone_merge_requests."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramMilestoneIID: {
				SemanticRole:     paramMilestoneIID,
				ValueSource:      "Milestone IID from milestone list output or URL context.",
				ExampleBinding:   "params.milestone_iid:3",
				CommonConfusions: []string{"Use milestone_iid (project-scoped IID), not global milestone ID."},
			},
		}
	case "milestone_create":
		options.Usage = "Create a milestone in a project with title and optional description/start/due dates."
		options.Aliases = []string{"create milestone", "new milestone", "add milestone"}
		options.RelatedActions = []string{actionMilestoneGet, "milestone.update", actionIssueList}
		options.IndividualTool.Description = "Create a new milestone in a project with title and optional description, start date, and due date. Returns: the created milestone with IID, state, dates, and web URL. See also: gitlab_milestone_get, gitlab_milestone_update, gitlab_milestone_list."
	case "milestone_update":
		options.Usage = "Update an existing milestone's title, description, dates, or state (close/activate) by milestone_iid."
		options.Aliases = []string{"update milestone", "edit milestone", "close milestone", "reopen milestone"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneList, "milestone.delete"}
		options.IndividualTool.Description = "Update an existing milestone's title, description, start/due dates, or state (state_event close/activate) by milestone IID. Returns: the updated milestone with state, dates, and web URL. See also: gitlab_milestone_get, gitlab_milestone_delete, gitlab_milestone_list."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state_event", map[string]any{"enum": []any{"close", "activate"}}),
		}
	case "milestone_delete":
		options.Usage = "Delete a milestone from a project by milestone_iid. Requires Maintainer or Owner role."
		options.Aliases = []string{"delete milestone", "remove milestone", "destroy milestone", "drop milestone"}
		options.RelatedActions = []string{actionMilestoneGet, "milestone.update", actionMilestoneList}
		options.IndividualTool.Description = "Delete a milestone permanently from a project by milestone IID. Returns: a success confirmation naming the milestone and project. See also: gitlab_milestone_get, gitlab_milestone_update, gitlab_milestone_list."
	case "milestone_issues":
		options.Usage = "List the issues assigned to a milestone by milestone_iid, with ordering and pagination."
		options.Aliases = []string{"list milestone issues", "issues in milestone", "show milestone issues"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneMergeRequests, actionIssueList}
		options.IndividualTool.Description = "List issues assigned to a single milestone with ordering and pagination. Returns: assigned issues with IID, title, state, web URL, creation time, and pagination metadata. See also: gitlab_milestone_get, gitlab_milestone_merge_requests, gitlab_issue_list."
	case "milestone_merge_requests":
		options.Usage = "List the merge requests assigned to a milestone by milestone_iid, with ordering and pagination."
		options.Aliases = []string{"list milestone merge requests", "MRs in milestone", "show milestone merge requests"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneIssues, "merge_request.list"}
		options.IndividualTool.Description = "List merge requests assigned to a single milestone with ordering and pagination. Returns: assigned merge requests with IID, title, state, source/target branches, web URL, creation time, and pagination metadata. See also: gitlab_milestone_get, gitlab_milestone_issues, gitlab_mr_list."
	}

	return options
}

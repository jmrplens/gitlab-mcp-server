package milestones

import (
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs this package publishes, to its related-action
// lists and to the hints its Markdown writes alike. A project milestone is an
// action of the project catalog group, so the ID a model is handed carries that
// prefix; spelling it "milestone.get" names nothing and is answered with
// "unknown action". One block, read from both files, because the two used to
// be kept apart and drifted.
const (
	actionMilestoneList          = "project.milestone_list"
	actionMilestoneGet           = "project.milestone_get"
	actionMilestoneCreate        = "project.milestone_create"
	actionMilestoneUpdate        = "project.milestone_update"
	actionMilestoneDelete        = "project.milestone_delete"
	actionMilestoneIssues        = "project.milestone_issues"
	actionMilestoneMergeRequests = "project.milestone_merge_requests"
	actionMilestonePromote       = "project.milestone_promote"
	actionMilestoneBurndown      = "project.milestone_burndown"

	// A promoted milestone lives on in the group, read from the group catalog.
	actionGroupMilestoneList = "group.group_milestone_list"

	actionProjectGet       = "project.get"
	actionIssueList        = "issue.list"
	actionIssueGet         = "issue.get"
	actionMergeRequestList = "merge_request.list"
	actionMergeRequestGet  = "merge_request.get"

	paramMilestoneIID = "milestone_iid"
)

// ActionSpecs returns canonical specs for project milestone actions
// exposed as MCP tools. The list, get, create, update, delete, issues,
// merge_requests, promote and burndown routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
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
		// gitlab_milestone_promote — promote a project milestone to its group (destructive).
		milestonePromoteSpec("milestone_promote", toolutil.DestructiveAction(client, Promote), "gitlab_milestone_promote"),
		// gitlab_milestone_burndown_events — list a milestone's burndown chart events (Premium).
		milestoneBurndownSpec("milestone_burndown", toolutil.RouteAction(client, GetBurndownChartEvents), "gitlab_milestone_burndown_events"),
	}
}

// milestonePromoteSpec builds the promotion's [toolutil.ActionSpec]. It is an
// update that is destructive: GitLab deletes the promoted milestone and every
// milestone of its title in the group's projects, moving their issues and
// merge requests to the new group milestone, and no route demotes it back, so
// the confirmation guard asks first. A second call finds nothing left to
// promote, which is why it stays idempotent. Destructive is declared here as
// well as carried by the route so the spec's agreement check reads both
// sides of the same intent.
func milestonePromoteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := milestoneOptionsForAction(name, individualTool)
	options.Destructive = true
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// milestoneBurndownSpec builds the burndown events read. The route is
// Premium and Ultimate (milestones.md, "Tier: Premium, Ultimate" on the
// burndown section), so the whole action is tier-gated.
func milestoneBurndownSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := milestoneOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewReadActionSpec(name, route, options)
}

// milestoneGetRoute wraps the [Get] route so a 404 response is
// converted into a structured [milestoneNotFoundOutput] hint rather
// than an error, matching the get-not-found pattern used across the
// project.
func milestoneGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapNotFound(func(params map[string]any) any {
		// The project is named as the caller wrote it. Reading it as a string
		// named a project given as a JSON number as no project at all.
		return milestoneNotFoundOutput{Identifier: fmt.Sprintf("IID %s in project %s",
			toolutil.ParamText(params[paramMilestoneIID]), toolutil.ParamText(params["project_id"]))}
	})
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
		RelatedActions: []string{actionProjectGet, actionIssueList},
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
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneUpdate, actionIssueList}
		options.IndividualTool.Description = "Create a new milestone in a project with title and optional description, start date, and due date. Returns: the created milestone with IID, state, dates, and web URL. See also: gitlab_milestone_get, gitlab_milestone_update, gitlab_milestone_list."
	case "milestone_update":
		options.Usage = "Update an existing milestone's title, description, dates, or state (close/activate) by milestone_iid."
		options.Aliases = []string{"update milestone", "edit milestone", "close milestone", "reopen milestone"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneList, actionMilestoneDelete, actionMilestonePromote}
		options.IndividualTool.Description = "Update an existing milestone's title, description, start/due dates, or state (state_event close/activate) by milestone IID. Returns: the updated milestone with state, dates, and web URL. See also: gitlab_milestone_get, gitlab_milestone_delete, gitlab_milestone_list."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state_event", map[string]any{"enum": []any{"close", "activate"}}),
		}
	case "milestone_delete":
		options.Usage = "Delete a milestone from a project by milestone_iid. Requires Maintainer or Owner role."
		options.Aliases = []string{"delete milestone", "remove milestone", "destroy milestone", "drop milestone"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneUpdate, actionMilestoneList}
		options.IndividualTool.Description = "Delete a milestone permanently from a project by milestone IID. Returns: a success confirmation naming the milestone and project. See also: gitlab_milestone_get, gitlab_milestone_update, gitlab_milestone_list."
	case "milestone_issues":
		options.Usage = "List the issues assigned to a milestone by milestone_iid, with ordering and pagination."
		options.Aliases = []string{"list milestone issues", "issues in milestone", "show milestone issues"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneMergeRequests, actionIssueList, actionMilestoneBurndown}
		options.IndividualTool.Description = "List issues assigned to a single milestone with ordering and pagination. Returns: assigned issues with IID, project, title, state, labels, author, assignees, confidentiality, weight, due date, web URL, timestamps, and pagination metadata. See also: gitlab_milestone_get, gitlab_milestone_merge_requests, gitlab_issue_list."
	case "milestone_merge_requests":
		options.Usage = "List the merge requests assigned to a milestone by milestone_iid, with ordering and pagination."
		options.Aliases = []string{"list milestone merge requests", "MRs in milestone", "show milestone merge requests"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneIssues, actionMergeRequestList}
		options.IndividualTool.Description = "List merge requests assigned to a single milestone with ordering and pagination. Returns: assigned merge requests with IID, project, title, state, draft flag, merge status, source/target branches, labels, author, assignees, reviewers, web URL, timestamps, and pagination metadata. See also: gitlab_milestone_get, gitlab_milestone_issues, gitlab_mr_list."
	case "milestone_promote":
		options.Usage = "Promote a project milestone to a milestone of the project's group by milestone_iid. GitLab creates the group milestone with the same title, description and dates, moves to it the issues and merge requests of every milestone of that title in the group's own projects, and deletes those project milestones, this one included. It cannot be undone. The project must belong to a group, and the caller needs the Planner role or higher in that group."
		options.Aliases = []string{"promote milestone", "promote milestone to group", "make group milestone", "convert project milestone to group milestone"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneList, actionGroupMilestoneList}
		options.IndividualTool.Description = "Promote a project milestone to a group milestone by milestone IID. GitLab deletes the project milestone and every milestone of the same title in the group's projects, moving their issues and merge requests to the new group milestone. Returns: a confirmation naming the promoted title, since GitLab sends back no milestone. See also: gitlab_milestone_get, gitlab_milestone_list, gitlab_group_milestone_list."
	case "milestone_burndown":
		options.Usage = "List the burndown chart events of a project milestone by milestone_iid: each issue's creation, closing and reopening with its weight. Requires GitLab Premium or higher. GitLab sends every event in one answer, and none for a milestone without both a start date and a due date."
		options.Aliases = []string{"milestone burndown", "project milestone burndown events", "milestone burndown chart"}
		options.RelatedActions = []string{actionMilestoneGet, actionMilestoneIssues, actionMilestoneMergeRequests}
		options.IndividualTool.Description = "List the burndown chart events of a project milestone (Premium or higher). Returns: every dated event with the issue's weight and the action (created, closed, reopened). See also: gitlab_milestone_get, gitlab_milestone_issues, gitlab_milestone_merge_requests."
	}

	return options
}

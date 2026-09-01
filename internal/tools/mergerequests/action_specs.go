package mergerequests

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	pipelineGetAction       = "pipeline.get"
	pipelineWaitAction      = "pipeline.wait"
	actionMRGet             = "merge_request.get"
	actionMRList            = "merge_request.list"
	actionMRMerge           = "merge_request.merge"
	actionMRUpdate          = "merge_request.update"
	actionMRApprove         = "merge_request.approve"
	actionMRTimeStats       = "merge_request.time_stats"
	actionMRTimeEstimateSet = "merge_request.time_estimate_set"
	actionMRSpentTimeAdd    = "merge_request.spent_time_add"
)

// ActionSpecs returns canonical specs for merge request actions exposed
// as MCP tools. The full MR lifecycle (create, read, update, merge,
// approve, delete, rebase, subscribe, time tracking, dependencies) is
// projected into the dynamic, meta, individual, and audit surfaces by
// the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_mr_create — open a new merge request from source into target branch.
		toolutil.NewCreateActionSpec("create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Aliases: []string{"create merge request", "open mr", "new merge request", "raise pull request"}, Tags: []string{"merge-request", "branch"},
				Usage:          "Use to open a merge request from a source branch into the target branch in a project.",
				RelatedActions: []string{actionMRGet, actionMRList, "branch.create", "project.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"source_branch": {
						SemanticRole:     "source_branch",
						ValueSource:      "Branch named after 'from'.",
						CommonConfusions: []string{"Do not use ref, tag_name, target_branch, or value for the source branch."},
						ExampleBinding:   "from feature/eval into main => source_branch=feature/eval.",
					},
					"target_branch": {
						SemanticRole:     "target_branch",
						ValueSource:      "Branch named after 'into' or the merge target.",
						CommonConfusions: []string{"Do not use source_branch, ref, tag_name, or to for the target branch."},
						ExampleBinding:   "from feature/eval into main => target_branch=main.",
					},
				},
				OpenWorld:    true,
				OwnerPackage: "mergerequests",
				IndividualTool: toolutil.IndividualToolSpec{
					Name:        "gitlab_mr_create",
					Title:       toolutil.TitleFromName("gitlab_mr_create"),
					Description: "Open a new merge request from a source branch into a target branch. Returns: the created MR with IID, state, source/target branches, merge status, assignees, reviewers, labels, and web URL. See also: gitlab_mr_get, gitlab_mr_list, gitlab_mr_merge.",
				},
			}),
		// gitlab_mr_get — fetch a single MR (returns a structured not-found result on 404).
		mergeRequestReadSpec("get", mergeRequestGetRoute(client), "gitlab_mr_get"),
		// gitlab_mr_list — list project MRs.
		mergeRequestReadSpec("list", toolutil.RouteAction(client, List), "gitlab_mr_list"),
		// gitlab_mr_list_global — list MRs across all projects visible to the caller.
		mergeRequestReadSpec("list_global", toolutil.RouteAction(client, ListGlobal), "gitlab_mr_list_global"),
		// gitlab_mr_list_group — list MRs across a group and its projects.
		mergeRequestReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_mr_list_group"),
		// gitlab_mr_update — update MR fields.
		mergeRequestUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_mr_update"),
		// gitlab_mr_merge — merge an MR now or schedule merge-when-pipeline-succeeds.
		mergeRequestDestructiveUpdateIndividualSpec("merge", toolutil.DestructiveAction(client, Merge), "gitlab_mr_merge"),
		// gitlab_mr_approve — add the caller's approval to the MR.
		mergeRequestUpdateSpec("approve", toolutil.RouteAction(client, Approve), "gitlab_mr_approve"),
		// gitlab_mr_unapprove — remove the caller's approval (destructive, idempotent).
		mergeRequestDestructiveUpdateIndividualSpec("unapprove", toolutil.RouteAction(client, UnapproveOutput), "gitlab_mr_unapprove"),
		// gitlab_mr_commits — list commits on the MR.
		mergeRequestReadSpec("commits", toolutil.RouteAction(client, Commits), "gitlab_mr_commits"),
		// gitlab_mr_pipelines — list pipelines attached to the MR.
		mergeRequestReadSpec("pipelines", toolutil.RouteAction(client, Pipelines), "gitlab_mr_pipelines"),
		// gitlab_mr_delete — delete an MR (destructive).
		mergeRequestDeleteSpec("delete", toolutil.RouteAction(client, DeleteOutput), "gitlab_mr_delete"),
		// gitlab_mr_rebase — rebase the MR's source branch.
		mergeRequestUpdateSpec("rebase", toolutil.RouteAction(client, Rebase), "gitlab_mr_rebase"),
		// gitlab_mr_participants — list MR participants.
		mergeRequestReadSpec("participants", toolutil.RouteAction(client, Participants), "gitlab_mr_participants"),
		// gitlab_mr_reviewers — list MR reviewers.
		mergeRequestReadSpec("reviewers", toolutil.RouteAction(client, Reviewers), "gitlab_mr_reviewers"),
		// gitlab_mr_create_pipeline — trigger a new pipeline for the MR.
		mergeRequestCreateSpec("create_pipeline", toolutil.RouteAction(client, CreatePipeline), "gitlab_mr_create_pipeline"),
		// gitlab_mr_issues_closed — list issues closed by the MR.
		mergeRequestReadSpec("issues_closed", toolutil.RouteAction(client, IssuesClosed), "gitlab_mr_issues_closed"),
		// gitlab_mr_cancel_auto_merge — cancel merge-when-pipeline-succeeds.
		mergeRequestUpdateSpec("cancel_auto_merge", toolutil.RouteAction(client, CancelAutoMerge), "gitlab_mr_cancel_auto_merge"),
		// gitlab_mr_subscribe — subscribe the caller to MR notifications.
		mergeRequestUpdateSpec("subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_mr_subscribe"),
		// gitlab_mr_unsubscribe — remove the caller's subscription.
		mergeRequestUpdateSpec("unsubscribe", toolutil.RouteAction(client, Unsubscribe), "gitlab_mr_unsubscribe"),
		// gitlab_mr_set_time_estimate — set the MR's time estimate.
		mergeRequestUpdateSpec("time_estimate_set", toolutil.RouteAction(client, SetTimeEstimate), "gitlab_mr_set_time_estimate"),
		// gitlab_mr_reset_time_estimate — clear the MR's time estimate.
		mergeRequestUpdateSpec("time_estimate_reset", toolutil.RouteAction(client, ResetTimeEstimate), "gitlab_mr_reset_time_estimate"),
		// gitlab_mr_add_spent_time — log time spent on the MR.
		// Additive rather than an update: GitLab adds the duration to what is
		// already logged, so two identical calls leave twice the time.
		mergeRequestAdditiveSpec("spent_time_add", toolutil.RouteAction(client, AddSpentTime), "gitlab_mr_add_spent_time"),
		// gitlab_mr_reset_spent_time — clear the MR's spent time.
		mergeRequestUpdateSpec("spent_time_reset", toolutil.RouteAction(client, ResetSpentTime), "gitlab_mr_reset_spent_time"),
		// gitlab_mr_time_stats — read MR time tracking totals.
		mergeRequestReadSpec("time_stats", toolutil.RouteAction(client, GetTimeStats), "gitlab_mr_time_stats"),
		// gitlab_mr_related_issues — list issues that mention or are closed by the MR.
		mergeRequestReadSpec("related_issues", toolutil.RouteAction(client, RelatedIssues), "gitlab_mr_related_issues"),
		// gitlab_mr_create_todo — add a to-do item on the MR for the caller.
		mergeRequestCreateSpec("create_todo", toolutil.RouteAction(client, CreateTodo), "gitlab_mr_create_todo"),
		// gitlab_mr_dependency_create — add a blocking dependency (Premium/Ultimate).
		mergeRequestPremiumSpec(mergeRequestCreateSpec("dependency_create", toolutil.RouteAction(client, CreateDependency), "gitlab_mr_dependency_create")),
		// gitlab_mr_dependency_delete — remove a blocking dependency (Premium/Ultimate, destructive).
		mergeRequestPremiumSpec(mergeRequestDeleteSpec("dependency_delete", toolutil.RouteAction(client, DeleteDependencyOutput), "gitlab_mr_dependency_delete")),
		// gitlab_mr_dependencies_list — list blocking dependencies (Premium/Ultimate).
		mergeRequestPremiumSpec(mergeRequestReadSpec("dependencies_list", toolutil.RouteAction(client, GetDependencies), "gitlab_mr_dependencies_list")),
	}
}

// mergeRequestGetRoute wraps the [Get] route so a 404 response is
// converted into a structured [mergeRequestNotFoundOutput] hint rather
// than an error, matching the get-not-found pattern used across the
// project.
func mergeRequestGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return mergeRequestNotFoundOutput{Identifier: fmt.Sprintf("!%v in project %v", input["merge_request_iid"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

// UnapproveOutput removes approval from a merge request and returns the
// legacy [toolutil.DeleteOutput] success shape required by the
// destructive-action contract.
func UnapproveOutput(ctx context.Context, client *gitlabclient.Client, input ApproveInput) (toolutil.DeleteOutput, error) {
	if err := Unapprove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted approval from MR !%d in project %s.", input.MRIID, input.ProjectID)}, nil
}

// DeleteOutput deletes a merge request and returns the legacy
// [toolutil.DeleteOutput] success shape required by the
// destructive-action contract.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted MR !%d from project %s.", input.MRIID, input.ProjectID)}, nil
}

// DeleteDependencyOutput removes a merge request dependency and
// returns the legacy [toolutil.DeleteOutput] success shape required by
// the destructive-action contract.
func DeleteDependencyOutput(ctx context.Context, client *gitlabclient.Client, input DeleteDependencyInput) (toolutil.DeleteOutput, error) {
	if err := DeleteDependency(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted dependency on blocking MR %d from MR !%d in project %s.", input.BlockingMergeRequestID, input.MRIID, input.ProjectID)}, nil
}

// mergeRequestReadSpec builds a read-only [toolutil.ActionSpec] for a
// merge request action using the package's default
// [mergeRequestOptions].
func mergeRequestReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

// mergeRequestCreateSpec builds a create-style [toolutil.ActionSpec]
// for a merge request action using the package's default
// [mergeRequestOptions].
func mergeRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

// mergeRequestUpdateSpec builds an update-style [toolutil.ActionSpec]
// for a merge request action using the package's default
// [mergeRequestOptions].
func mergeRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

// mergeRequestAdditiveSpec is mergeRequestUpdateSpec for an action that
// accumulates rather than sets, so repeating it is not the same as doing it
// once.
func mergeRequestAdditiveSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewAdditiveActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

// mergeRequestDeleteSpec builds a destructive [toolutil.ActionSpec]
// for a merge request action using the package's default
// [mergeRequestOptions].
func mergeRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

// mergeRequestPremiumSpec marks a spec as a GitLab Premium/Ultimate action.
// Merge request dependencies require Premium per
// doc/user/project/merge_requests/dependencies.md.
func mergeRequestPremiumSpec(spec toolutil.ActionSpec) toolutil.ActionSpec {
	spec.Edition = "premium"
	return spec
}

// mergeRequestDestructiveUpdateIndividualSpec builds a destructive
// [toolutil.ActionSpec] whose individual tool is NOT marked
// destructive, used for actions that are destructive through the
// dynamic surface but a normal update through the individual tool
// (for example merge and unapprove).
func mergeRequestDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := mergeRequestOptions(name, individualTool)
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// mergeRequestActionMetadata holds the per-action discovery metadata that
// replaces the generic placeholders for every merge request action: an
// action-specific Usage sentence, natural-language Aliases, canonical
// RelatedActions cross-links, and the "Returns: … See also: …" individual-tool
// description required by the 1:1 metadata norm (R-META).
type mergeRequestActionMetadata struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// mergeRequestActionMetadataTable maps each merge request action name to its
// curated discovery metadata. Keys correspond to the action names passed to
// mergeRequestOptions from ActionSpecs.
func mergeRequestActionMetadataTable() map[string]mergeRequestActionMetadata {
	return map[string]mergeRequestActionMetadata{
		"get": {
			usage:       "Fetch a single merge request by its project-scoped IID, optionally including diverged-commit count, rebase-in-progress state, or HTML-rendered title/description.",
			aliases:     []string{"get merge request", "show mr", "view merge request"},
			related:     []string{actionMRList, "merge_request.commits", actionMRUpdate, actionMRMerge},
			description: "Get a single merge request from a project by its IID. Returns: MR metadata, state, source/target branches, merge status, assignees, reviewers, labels, milestone, and web URL. See also: gitlab_mr_list, gitlab_mr_update, gitlab_mr_merge, gitlab_mr_commits.",
		},
		"list": {
			usage:       "List merge requests in one project with rich filtering (state, labels, author/assignee/reviewer, approvals, environment, dates) and offset or keyset pagination.",
			aliases:     []string{"list merge requests", "list mrs", "list project merge requests"},
			related:     []string{actionMRGet, "merge_request.create", "merge_request.list_group", "search.merge_requests"},
			description: "List merge requests in one project with filtering and pagination. Returns: matching MRs with state, branches, merge status, assignees, reviewers, labels, and pagination metadata. See also: gitlab_mr_get, gitlab_mr_create, gitlab_mr_list_group.",
		},
		"list_global": {
			usage:       "List merge requests across all projects visible to the caller. Narrow with scope=created_by_me or scope=assigned_to_me, author/reviewer, approvals, or label filters.",
			aliases:     []string{"list all merge requests", "list my merge requests", "list merge requests across projects"},
			related:     []string{actionMRList, "merge_request.list_group", "search.merge_requests"},
			description: "List merge requests across all accessible projects. Returns: visible MRs with project context, state, branches, and pagination metadata. See also: gitlab_mr_list, gitlab_mr_list_group, gitlab_search_merge_requests.",
		},
		"list_group": {
			usage:       "List merge requests across a group and its subgroups/projects with state, author, reviewer, approval, and label filters plus offset or keyset pagination.",
			aliases:     []string{"list group merge requests", "list mrs in group", "list merge requests for group"},
			related:     []string{actionMRList, "merge_request.list_global", "group.get"},
			description: "List merge requests across a group and its projects. Returns: visible MRs with project context, state, branches, and pagination metadata. See also: gitlab_mr_list, gitlab_mr_list_global, gitlab_group_get.",
		},
		"update": {
			usage:       "Update merge request fields (title, description, target branch, assignees, reviewers, labels, milestone) or transition state with state_event=close/reopen.",
			aliases:     []string{"update merge request", "edit mr", "change merge request", "close merge request", "reopen merge request"},
			related:     []string{actionMRGet, actionMRMerge, actionMRApprove, "merge_request.delete"},
			description: "Update an existing merge request's fields or state. Returns: the updated MR with its new field values, state, labels, assignees, and reviewers. See also: gitlab_mr_get, gitlab_mr_merge, gitlab_mr_approve.",
		},
		"approve": {
			usage:       "Add the caller's approval to a merge request. Pass sha to approve only if the MR HEAD still matches (a safety check against new pushes).",
			aliases:     []string{"approve merge request", "approve mr", "add my approval to mr", "sign off on mr", "lgtm this mr"},
			related:     []string{"merge_request.unapprove", actionMRMerge, actionMRGet},
			description: "Approve a merge request on behalf of the caller. Returns: the approval state with required-approvals count, approved-by count, and overall approved flag. See also: gitlab_mr_unapprove, gitlab_mr_merge, gitlab_mr_get.",
		},
		"unapprove": {
			usage:       "Remove the caller's previously granted approval from a merge request (idempotent. Requires the caller to have approved first).",
			aliases:     []string{"unapprove merge request", "remove approval", "revoke mr approval"},
			related:     []string{actionMRApprove, actionMRGet},
			description: "Remove the caller's approval from a merge request. Returns: a success confirmation naming the MR and project. See also: gitlab_mr_approve, gitlab_mr_get.",
		},
		"commits": {
			usage:       "List the commits contained in a merge request, ordered and paginated (offset or keyset).",
			aliases:     []string{"list merge request commits", "list mr commits", "show commits in mr"},
			related:     []string{actionMRGet, "merge_request.changes_get", "commit.get"},
			description: "List the commits that make up a merge request. Returns: commit SHAs, titles, authors, and timestamps with pagination metadata. See also: gitlab_mr_get, gitlab_commit_get.",
		},
		"delete": {
			usage:       "Permanently delete a merge request (owner-only). To merely stop work, use merge_request.update with state_event=close instead.",
			aliases:     []string{"delete merge request", "remove mr", "destroy merge request"},
			related:     []string{actionMRUpdate, actionMRGet},
			description: "Delete a merge request permanently. Returns: a success confirmation naming the MR and project. See also: gitlab_mr_update, gitlab_mr_get.",
		},
		"rebase": {
			usage:       "Rebase a merge request's source branch onto the latest target branch. Set skip_ci to avoid triggering a pipeline after the rebase.",
			aliases:     []string{"rebase merge request", "rebase mr", "rebase source branch"},
			related:     []string{actionMRGet, actionMRMerge, "merge_request.commits"},
			description: "Rebase a merge request's source branch onto its target. Returns: whether a rebase is now in progress (poll merge_request.get to track completion). See also: gitlab_mr_get, gitlab_mr_merge.",
		},
		"participants": {
			usage:       "List every user participating in a merge request (author, assignees, reviewers, commenters).",
			aliases:     []string{"list merge request participants", "list mr participants", "who is involved in mr"},
			related:     []string{"merge_request.reviewers", actionMRGet},
			description: "List the participants of a merge request. Returns: participating users with username, name, and state. See also: gitlab_mr_reviewers, gitlab_mr_get.",
		},
		"reviewers": {
			usage:       "List the assigned reviewers of a merge request along with each reviewer's review state.",
			aliases:     []string{"list merge request reviewers", "list mr reviewers", "show reviewers"},
			related:     []string{"merge_request.participants", actionMRUpdate, actionMRApprove},
			description: "List the reviewers of a merge request. Returns: reviewer users with username, name, and review state. See also: gitlab_mr_participants, gitlab_mr_update.",
		},
		"issues_closed": {
			usage:       "List issues that will be closed when this merge request is merged (issues referenced via 'Closes #N' in the MR description or commits).",
			aliases:     []string{"list issues closed by mr", "issues closed on merge", "what issues does this mr close"},
			related:     []string{"merge_request.related_issues", actionMRGet, "issue.get"},
			description: "List issues that will be closed when the merge request is merged. Returns: issues with state, labels, assignees, and web URL plus pagination metadata. See also: gitlab_mr_related_issues, gitlab_issue_get.",
		},
		"subscribe": {
			usage:       "Subscribe the caller to a merge request to receive notifications about its activity.",
			aliases:     []string{"subscribe to merge request", "watch mr", "follow merge request"},
			related:     []string{"merge_request.unsubscribe", actionMRGet},
			description: "Subscribe the caller to a merge request's notifications. Returns: the MR with the updated subscription state. See also: gitlab_mr_unsubscribe, gitlab_mr_get.",
		},
		"unsubscribe": {
			usage:       "Unsubscribe the caller from a merge request to stop receiving its notifications.",
			aliases:     []string{"unsubscribe from merge request", "unwatch mr", "stop following merge request"},
			related:     []string{"merge_request.subscribe", actionMRGet},
			description: "Unsubscribe the caller from a merge request's notifications. Returns: the MR with the updated subscription state. See also: gitlab_mr_subscribe, gitlab_mr_get.",
		},
		"time_estimate_set": {
			usage:       "Set the time estimate for a merge request using a GitLab duration string such as '3h30m', '1d', or '2w'.",
			aliases:     []string{"set merge request time estimate", "estimate mr time", "set mr estimate"},
			related:     []string{"merge_request.time_estimate_reset", actionMRSpentTimeAdd, actionMRTimeStats},
			description: "Set a merge request's time estimate. Returns: the updated time tracking stats (estimate and spent time, seconds and human-readable). See also: gitlab_mr_reset_time_estimate, gitlab_mr_add_spent_time, gitlab_mr_time_stats.",
		},
		"time_estimate_reset": {
			usage:       "Clear the time estimate previously set on a merge request.",
			aliases:     []string{"reset merge request time estimate", "clear mr estimate", "remove mr time estimate"},
			related:     []string{actionMRTimeEstimateSet, actionMRTimeStats},
			description: "Clear a merge request's time estimate. Returns: the updated time tracking stats. See also: gitlab_mr_set_time_estimate, gitlab_mr_time_stats.",
		},
		"spent_time_add": {
			usage:       "Log spent time on a merge request with a GitLab duration string. Use a leading '-' (e.g. '-1h') to subtract previously logged time.",
			aliases:     []string{"add merge request spent time", "log time on mr", "record mr time"},
			related:     []string{"merge_request.spent_time_reset", actionMRTimeEstimateSet, actionMRTimeStats},
			description: "Add spent time to a merge request. Returns: the updated time tracking stats (estimate and spent time, seconds and human-readable). See also: gitlab_mr_reset_spent_time, gitlab_mr_set_time_estimate, gitlab_mr_time_stats.",
		},
		"spent_time_reset": {
			usage:       "Reset the total spent time logged on a merge request back to zero.",
			aliases:     []string{"reset merge request spent time", "clear mr spent time", "zero mr time spent"},
			related:     []string{actionMRSpentTimeAdd, actionMRTimeStats},
			description: "Reset a merge request's spent time. Returns: the updated time tracking stats. See also: gitlab_mr_add_spent_time, gitlab_mr_time_stats.",
		},
		"time_stats": {
			usage:       "Read the time tracking totals (estimate and total spent time) for a merge request.",
			aliases:     []string{"get merge request time stats", "show mr time tracking", "mr time totals"},
			related:     []string{actionMRTimeEstimateSet, actionMRSpentTimeAdd},
			description: "Read a merge request's time tracking totals. Returns: estimate and spent time in seconds and human-readable form. See also: gitlab_mr_set_time_estimate, gitlab_mr_add_spent_time.",
		},
		"related_issues": {
			usage:       "List issues referenced by a merge request in its description, commits, or notes (a superset of issues_closed).",
			aliases:     []string{"list merge request related issues", "issues related to mr", "show related issues"},
			related:     []string{"merge_request.issues_closed", actionMRGet, "issue.get"},
			description: "List issues related to a merge request. Returns: issues with state, labels, assignees, and web URL plus pagination metadata. See also: gitlab_mr_issues_closed, gitlab_issue_get.",
		},
		"create_todo": {
			usage:       "Create a to-do item on a merge request for the caller so it appears in their GitLab to-do list.",
			aliases:     []string{"create merge request todo", "add mr todo", "mark mr as todo"},
			related:     []string{actionMRGet, "merge_request.subscribe"},
			description: "Create a to-do for a merge request for the caller. Returns: the created to-do item with action, target, and state. See also: gitlab_mr_get, gitlab_mr_subscribe.",
		},
		"dependency_create": {
			usage:       "Add a blocking dependency so this merge request cannot merge until the specified blocking MR is merged.",
			aliases:     []string{"create merge request dependency", "add mr dependency", "block mr on another mr"},
			related:     []string{"merge_request.dependency_delete", "merge_request.dependencies_list", actionMRMerge},
			description: "Add a blocking dependency to a merge request. Returns: the created dependency linking the MR to its blocking MR. See also: gitlab_mr_dependency_delete, gitlab_mr_dependencies_list.",
		},
		"dependency_delete": {
			usage:       "Remove a previously added blocking dependency from a merge request.",
			aliases:     []string{"delete merge request dependency", "remove mr dependency", "unblock mr"},
			related:     []string{"merge_request.dependency_create", "merge_request.dependencies_list"},
			description: "Remove a blocking dependency from a merge request. Returns: a success confirmation naming the MR, blocking MR, and project. See also: gitlab_mr_dependency_create, gitlab_mr_dependencies_list.",
		},
		"dependencies_list": {
			usage:       "List the blocking dependencies of a merge request (the MRs that must merge first).",
			aliases:     []string{"list merge request dependencies", "list mr dependencies", "show mr blockers"},
			related:     []string{"merge_request.dependency_create", "merge_request.dependency_delete"},
			description: "List a merge request's blocking dependencies. Returns: the dependency links with blocking MR references and their states. See also: gitlab_mr_dependency_create, gitlab_mr_dependency_delete.",
		},
	}
}

// mergeRequestOptions returns the base [toolutil.ActionSpecOptions]
// for a merge request action with action-specific Usage, natural-language
// Aliases, canonical RelatedActions, and the "Returns: … See also: …"
// individual-tool description (R-META). The merge, pipelines,
// create_pipeline, and cancel_auto_merge actions additionally receive
// workflow-specific guidance separating auto-merge from pipeline waiting.
func mergeRequestOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mergerequests domain action.", Tags: []string{"merge_request"},
		OpenWorld:      true,
		OwnerPackage:   "mergerequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := mergeRequestActionMetadataTable()[actionName]; ok {
		options.Usage = meta.usage
		options.Aliases = meta.aliases
		options.RelatedActions = meta.related
		options.IndividualTool.Description = meta.description
	}
	switch actionName {
	case "list":
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"opened", "closed", "locked", "merged", "all"}}),
			toolutil.SchemaPropertyOverride("scope", map[string]any{"enum": []any{"created_by_me", "assigned_to_me", "all"}}),
			toolutil.SchemaPropertyOverride("order_by", map[string]any{"enum": []any{"created_at", "updated_at", "title"}}),
			toolutil.SchemaPropertyOverride("wip", map[string]any{"enum": []any{"yes", "no"}}),
			toolutil.SchemaPropertyOverride("view", map[string]any{"enum": []any{"simple"}}),
			toolutil.SchemaApproverIDsOverride("approver_ids"),
			toolutil.SchemaApproverIDsOverride("approved_by_ids"),
		}
	case "list_global":
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"opened", "closed", "locked", "merged", "all"}}),
			toolutil.SchemaPropertyOverride("scope", map[string]any{"enum": []any{"created_by_me", "assigned_to_me", "all"}}),
			toolutil.SchemaPropertyOverride("order_by", map[string]any{"enum": []any{"created_at", "updated_at"}}),
			toolutil.SchemaPropertyOverride("wip", map[string]any{"enum": []any{"yes", "no"}}),
			toolutil.SchemaPropertyOverride("view", map[string]any{"enum": []any{"simple"}}),
			toolutil.SchemaPropertyOverride("approved", map[string]any{"enum": []any{"yes", "no"}}),
			toolutil.SchemaApproverIDsOverride("approver_ids"),
			toolutil.SchemaApproverIDsOverride("approved_by_ids"),
		}
	case "list_group":
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state", map[string]any{"enum": []any{"opened", "closed", "locked", "merged", "all"}}),
			toolutil.SchemaPropertyOverride("scope", map[string]any{"enum": []any{"created_by_me", "assigned_to_me", "all"}}),
			toolutil.SchemaPropertyOverride("order_by", map[string]any{"enum": []any{"created_at", "updated_at"}}),
			toolutil.SchemaPropertyOverride("wip", map[string]any{"enum": []any{"yes", "no"}}),
			toolutil.SchemaPropertyOverride("view", map[string]any{"enum": []any{"simple"}}),
			toolutil.SchemaApproverIDsOverride("approver_ids"),
			toolutil.SchemaApproverIDsOverride("approved_by_ids"),
		}
	case "update":
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("state_event", map[string]any{"enum": []any{"close", "reopen"}}),
		}
	case "merge":
		options.Usage = "Use to merge a merge request now, or set params.auto_merge=true when the task asks to merge when pipeline succeeds. Do not use " + pipelineWaitAction + " unless the task only asks to wait for an existing pipeline."
		options.Aliases = []string{"merge merge request", "merge mr", "merge when pipeline succeeds"}
		options.RelatedActions = []string{"merge_request.pipelines", pipelineWaitAction, pipelineGetAction, "merge_request.cancel_auto_merge"}
		options.IndividualTool.Description = "Merge a merge request now, or schedule auto-merge when its pipeline succeeds. Returns: the merged MR with merge_commit_sha and updated state. See also: gitlab_mr_get, gitlab_mr_pipelines, gitlab_mr_cancel_auto_merge."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"auto_merge": {
				SemanticRole:     "merge_scheduling",
				ValueSource:      "Set true only when the user asks to merge when the pipeline succeeds or enable auto-merge.",
				CommonConfusions: []string{"Do not call " + pipelineWaitAction + " for merge-when-pipeline-succeeds requests. Use merge_request.merge with auto_merge=true."},
				ExampleBinding:   "merge !7 when pipeline succeeds => action merge with auto_merge=true.",
			},
			"merge_request_iid": {
				SemanticRole:     "merge_request_iid",
				ValueSource:      "Project-scoped MR IID from merge_request.list or merge_request.get.",
				CommonConfusions: []string{"Do not use pipeline_id. merge_request.merge requires merge_request_iid."},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			{PropertyPath: "auto_merge", Values: map[string]any{"description": "Set true only when the user asks to merge when the pipeline succeeds or enable auto-merge."}},
		}
	case "pipelines":
		options.Usage = "Lists pipelines attached to a merge request. Use " + pipelineWaitAction + " with the returned pipeline_id only when the task asks to wait for CI completion."
		options.Aliases = []string{"list merge request pipelines", "list mr pipelines", "show mr pipelines"}
		options.RelatedActions = []string{pipelineWaitAction, pipelineGetAction, actionMRMerge, "merge_request.create_pipeline"}
		options.IndividualTool.Description = "List the CI/CD pipelines attached to a merge request. Returns: pipelines with id, status, ref, sha, and web URL. See also: gitlab_mr_create_pipeline, gitlab_mr_merge, gitlab_pipeline_get."
	case "create_pipeline":
		options.Usage = "Creates a new pipeline for a merge request. Use " + pipelineWaitAction + " after receiving pipeline_id if the task asks to wait for completion."
		options.Aliases = []string{"create merge request pipeline", "trigger mr pipeline", "run pipeline for mr"}
		options.RelatedActions = []string{"merge_request.pipelines", pipelineWaitAction, pipelineGetAction}
		options.IndividualTool.Description = "Trigger a new CI/CD pipeline for a merge request. Returns: the created pipeline with id, status, ref, sha, and web URL. See also: gitlab_mr_pipelines, gitlab_pipeline_get."
	case "cancel_auto_merge":
		options.Usage = "Cancels auto-merge on a merge request. It does not cancel a running pipeline."
		options.Aliases = []string{"cancel auto merge", "cancel merge when pipeline succeeds", "stop auto merge"}
		options.RelatedActions = []string{actionMRMerge, actionMRGet}
		options.IndividualTool.Description = "Cancel auto-merge (merge-when-pipeline-succeeds) on a merge request. Returns: the MR with auto-merge disabled. See also: gitlab_mr_merge, gitlab_mr_get."
	}
	return options
}

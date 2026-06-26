package issues

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionIssueList         = "issue.list"
	actionSearchIssues      = "search.issues"
	actionIssueGet          = "issue.get"
	actionIssueUpdate       = "issue.update"
	actionIssueTimeStatsGet = "issue.time_stats_get"
	actionIssueTimeEstSet   = "issue.time_estimate_set"
	actionIssueSpentTimeAdd = "issue.spent_time_add"
	toolIssueListGroup      = "gitlab_issue_list_group"
	paramStateEvent         = "state_event"
	roleScopeProject        = "scope_project"
	paramProjectID          = "project_id"
	domainIssues            = "issues"
)

// ActionSpecs returns canonical specs for issue lifecycle actions exposed
// as MCP tools. The create, read, update, delete, reorder, move, subscribe,
// todo, and time-tracking routes are projected into the dynamic, meta,
// individual, and audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_issue_create — open a new issue in a project.
		issueCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_issue_create"),
		// gitlab_issue_get — fetch a single issue by IID (returns a structured not-found result on 404).
		issueReadSpec("get", toolutil.RouteAction(client, getWithEmbeddedResource), "gitlab_issue_get"),
		// gitlab_issue_get_by_id — fetch a single issue by its global database ID.
		issueReadSpec("get_by_id", toolutil.RouteAction(client, GetByID), "gitlab_issue_get_by_id"),
		// gitlab_issue_list — list project issues with optional filtering and pagination.
		issueReadSpec("list", toolutil.RouteAction(client, List), "gitlab_issue_list"),
		// gitlab_issue_list_all — list issues visible to the caller across all projects.
		issueReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_issue_list_all"),
		// gitlab_issue_list_group — list issues across a group and its projects.
		issueReadSpec("list_group", toolutil.RouteAction(client, ListGroup), toolIssueListGroup),
		// gitlab_issue_update — update issue fields; supports close/reopen via state_event.
		issueUpdateActionSpec(client),
		// gitlab_issue_delete — permanently delete an issue (destructive, requires confirmation).
		issueDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_issue_delete"),
		// gitlab_issue_reorder — move an issue before/after another issue in the list.
		issueUpdateSpec("reorder", toolutil.RouteAction(client, Reorder), "gitlab_issue_reorder"),
		// gitlab_issue_move — move an issue to a different project.
		issueUpdateSpec("move", toolutil.RouteAction(client, Move), "gitlab_issue_move"),
		// gitlab_issue_subscribe — subscribe the caller to issue notifications.
		issueUpdateSpec("subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_issue_subscribe"),
		// gitlab_issue_unsubscribe — remove the caller's subscription from the issue.
		issueUpdateSpec("unsubscribe", toolutil.RouteAction(client, Unsubscribe), "gitlab_issue_unsubscribe"),
		// gitlab_issue_create_todo — add a to-do item for the caller on this issue.
		issueCreateSpec("create_todo", toolutil.RouteAction(client, CreateTodo), "gitlab_issue_create_todo"),
		// gitlab_issue_time_estimate_set — set the time estimate (e.g. "3h30m").
		issueUpdateSpec("time_estimate_set", toolutil.RouteAction(client, SetTimeEstimate), "gitlab_issue_time_estimate_set"),
		// gitlab_issue_time_estimate_reset — clear the time estimate.
		issueUpdateSpec("time_estimate_reset", toolutil.RouteAction(client, ResetTimeEstimate), "gitlab_issue_time_estimate_reset"),
		// gitlab_issue_spent_time_add — log time spent on the issue.
		issueUpdateSpec("spent_time_add", toolutil.RouteAction(client, AddSpentTime), "gitlab_issue_spent_time_add"),
		// gitlab_issue_spent_time_reset — clear total spent time for the issue.
		issueUpdateSpec("spent_time_reset", toolutil.RouteAction(client, ResetSpentTime), "gitlab_issue_spent_time_reset"),
		// gitlab_issue_time_stats_get — read time tracking totals for the issue.
		issueReadSpec("time_stats_get", toolutil.RouteAction(client, GetTimeStats), "gitlab_issue_time_stats_get"),
		// gitlab_issue_participants — list users participating in the issue.
		issueReadSpec("participants", toolutil.RouteAction(client, GetParticipants), "gitlab_issue_participants"),
		// gitlab_issue_mrs_closing — list MRs that will close this issue on merge.
		issueReadSpec("mrs_closing", toolutil.RouteAction(client, ListMRsClosing), "gitlab_issue_mrs_closing"),
		// gitlab_issue_mrs_related — list MRs related to this issue (broader than closing).
		issueReadSpec("mrs_related", toolutil.RouteAction(client, ListMRsRelated), "gitlab_issue_mrs_related"),
	}
}

// getOutput wraps [Output] so the issue.get route can return a distinct
// type that downstream formatters recognize as a single-issue payload.
type getOutput struct {
	Output
}

// getWithEmbeddedResource delegates to [Get] and embeds the result in a
// [getOutput]. Used by the get route so the formatter registry can pick
// the single-issue Markdown renderer.
func getWithEmbeddedResource(ctx context.Context, client *gitlabclient.Client, input GetInput) (getOutput, error) {
	out, err := Get(ctx, client, input)
	return getOutput{Output: out}, err
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that names the issue and project in the message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("issue #%d from project %s", input.IssueIID, input.ProjectID))
	return out, nil
}

// GroupActionSpecs returns canonical specs for issue actions exposed
// through the group meta-tool surface. Currently scoped to group issue
// listing.
func GroupActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_issue_list_group — list issues across a group's projects.
		groupIssueReadSpec(domainIssues, toolutil.RouteAction(client, ListGroup), toolIssueListGroup),
	}
}

// issueReadSpec builds a read-only [toolutil.ActionSpec] for an issue
// action and fills in the usage, related actions, and parameter guidance
// for the most common individual tools.
func issueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	switch individualTool {
	case "gitlab_issue_get":
		options.Usage = "Get one exact issue by project_id plus issue_iid. Use this after list/search results or when the prompt already names a concrete issue number; prefer issue.get over issue.list when the target issue is already known."
		options.Aliases = []string{"get issue", "show issue details", "fetch issue"}
		options.RelatedActions = []string{actionIssueList, actionIssueUpdate, "issue.delete", "issue.notes_list"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramProjectID: {
				SemanticRole:     roleScopeProject,
				ValueSource:      "Project ID or full namespace path that owns the issue.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Use the issue's parent project here, not a group path or global issue ID."},
			},
			"issue_iid": {
				SemanticRole:     "issue_iid",
				ValueSource:      "Issue number visible in the project, usually from the URL or prior issue list output.",
				ExampleBinding:   "params.issue_iid:42",
				CommonConfusions: []string{"Use issue_iid for project-scoped issue numbers; issue_id is only for the global issue ID action."},
			},
		}
		options.IndividualTool.Description = "Get a single issue from a project by issue IID. Returns: issue metadata, state, labels, assignees, author, due date, task completion, and web URL. See also: gitlab_issue_list, gitlab_issue_update, gitlab_issue_delete, gitlab_issue_notes_list."
	case "gitlab_issue_list":
		options.Usage = "List issues in one project. Use filters such as state, labels, search, assignee_username, milestone, order_by, sort, and pagination when the prompt asks for matching or recent issues in a known project."
		options.Aliases = []string{"list project issues", "find issues in project", "show project issues"}
		options.RelatedActions = []string{actionIssueGet, "issue.create", actionSearchIssues}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramProjectID: {
				SemanticRole:     roleScopeProject,
				ValueSource:      "Project ID or namespace path whose issues should be listed.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Use project_id for the project scope; use group_id only with issue.list_group."},
			},
			"search": {
				ValueSource:      "Keywords from the user's issue title or description request.",
				ExampleBinding:   `params.search:"oauth timeout"`,
				CommonConfusions: []string{"search narrows within the selected project; it does not replace project_id."},
			},
			"order_by": {
				SemanticRole:     "issue_list_sort_field",
				ValueSource:      "Field requested for sorting recent or oldest issues, such as created_at or updated_at.",
				ExampleBinding:   `params.order_by:"updated_at"`,
				CommonConfusions: []string{"Combine order_by with sort; do not pass natural-language phrases like newest first as the field value."},
			},
		}
		options.IndividualTool.Description = "List issues in one project with filtering and pagination. Returns: matching issues with state, labels, assignees, author, and pagination metadata. See also: gitlab_issue_get, gitlab_issue_create, gitlab_search_issues."
	case "gitlab_issue_list_all":
		options.Usage = "List issues visible to the authenticated user across all accessible projects. Use this when the user asks for their open issues, assigned issues, or a cross-project issue overview."
		options.Aliases = []string{"list all issues", "show my issues across projects", "list visible issues"}
		options.RelatedActions = []string{actionIssueList, "issue.list_group", actionSearchIssues}
		options.IndividualTool.Description = "List issues across accessible projects. Returns: visible issues with project context and pagination metadata. See also: gitlab_issue_list, gitlab_issue_list_group, gitlab_search_issues."
	default:
		decorateIssueMeta(&options, individualTool)
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateIssueMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for the issue actions that would otherwise inherit the generic placeholder
// metadata from issueOptions. It is a no-op for tools whose dedicated spec
// builders already set rich metadata (create, get, list, list_all, update).
func decorateIssueMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := issueActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// issueActionMetaEntry is the discovery metadata for one issue action.
type issueActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// issueActionMeta maps each individual issue tool to its discovery metadata.
var issueActionMeta = map[string]issueActionMetaEntry{
	"gitlab_issue_get_by_id": {
		usage:       "Fetch one issue by its global database ID rather than a project IID. Use when the prompt or prior output gives a numeric issue id with no project context.",
		aliases:     []string{"get issue by id", "fetch issue by global id"},
		related:     []string{actionIssueGet, actionIssueList, actionSearchIssues},
		description: "Get a single issue by its global ID. Returns: the issue with state, labels, assignees, author, and web URL. See also: gitlab_issue_get, gitlab_issue_list.",
	},
	// Note: gitlab_issue_list_group is dual-projected (issue.list_group and the
	// group surface group.issues). Both projections carry natural-language
	// aliases so neither canonical action is aliases_only (1:1 audit metadata).
	// The group surface owns the individual-tool description to avoid a
	// projected-description drift across the two canonical actions.
	toolIssueListGroup: {
		usage:   "List issues across a group and its subgroups and projects. Use when work is scoped to a group rather than a single project.",
		aliases: []string{"issues in a group", "group-scoped issue list", "list a group's issues", "issues across subgroups"},
		related: []string{actionIssueList, "group.get", actionSearchIssues},
	},
	"gitlab_issue_delete": {
		usage:       "Permanently delete an issue. Destructive and irreversible; confirm project_id and issue_iid before calling.",
		aliases:     []string{"delete issue", "remove issue"},
		related:     []string{actionIssueGet, actionIssueList, actionIssueUpdate},
		description: "Delete an issue permanently. Returns: a success confirmation naming the issue and project. See also: gitlab_issue_get, gitlab_issue_update.",
	},
	"gitlab_issue_reorder": {
		usage:       "Reposition an issue relative to another issue (move before or after) to change its manual order on a board or list.",
		aliases:     []string{"reorder issue", "move issue in list"},
		related:     []string{actionIssueGet, actionIssueList},
		description: "Reorder an issue within its list. Returns: the updated issue. See also: gitlab_issue_get, gitlab_issue_list.",
	},
	"gitlab_issue_move": {
		usage:       "Move an issue to a different project, preserving its discussion and metadata.",
		aliases:     []string{"move issue to project", "transfer issue"},
		related:     []string{actionIssueGet, actionIssueUpdate, actionIssueList},
		description: "Move an issue to another project. Returns: the issue in its new project. See also: gitlab_issue_get, gitlab_issue_update.",
	},
	"gitlab_issue_subscribe": {
		usage:       "Subscribe the authenticated user to notifications for an issue.",
		aliases:     []string{"subscribe to issue", "follow issue"},
		related:     []string{actionIssueGet, "issue.unsubscribe"},
		description: "Subscribe to an issue's notifications. Returns: the issue with the updated subscription state. See also: gitlab_issue_get, gitlab_issue_unsubscribe.",
	},
	"gitlab_issue_unsubscribe": {
		usage:       "Unsubscribe the authenticated user from an issue's notifications.",
		aliases:     []string{"unsubscribe from issue", "unfollow issue"},
		related:     []string{actionIssueGet, "issue.subscribe"},
		description: "Unsubscribe from an issue's notifications. Returns: the issue with the updated subscription state. See also: gitlab_issue_get, gitlab_issue_subscribe.",
	},
	"gitlab_issue_create_todo": {
		usage:       "Create a to-do item for the authenticated user on an issue so it appears in their GitLab to-do list.",
		aliases:     []string{"add issue todo", "create todo for issue"},
		related:     []string{actionIssueGet, actionIssueList},
		description: "Create a to-do for an issue. Returns: the created to-do item with action, target, and state. See also: gitlab_issue_get.",
	},
	"gitlab_issue_time_estimate_set": {
		usage:       "Set the time estimate for an issue using a human duration such as 3h30m or 1d.",
		aliases:     []string{"set issue estimate", "estimate issue time"},
		related:     []string{"issue.time_estimate_reset", actionIssueSpentTimeAdd, actionIssueTimeStatsGet},
		description: "Set an issue's time estimate. Returns: the updated time tracking stats. See also: gitlab_issue_time_estimate_reset, gitlab_issue_time_stats_get.",
	},
	"gitlab_issue_time_estimate_reset": {
		usage:       "Clear the time estimate previously set on an issue.",
		aliases:     []string{"reset issue estimate", "clear issue estimate"},
		related:     []string{actionIssueTimeEstSet, actionIssueTimeStatsGet},
		description: "Clear an issue's time estimate. Returns: the updated time tracking stats. See also: gitlab_issue_time_estimate_set, gitlab_issue_time_stats_get.",
	},
	"gitlab_issue_spent_time_add": {
		usage:       "Log time spent on an issue using a human duration such as 2h or 30m; logged values accumulate across calls.",
		aliases:     []string{"log issue time", "add spent time"},
		related:     []string{"issue.spent_time_reset", actionIssueTimeEstSet, actionIssueTimeStatsGet},
		description: "Add spent time to an issue. Returns: the updated time tracking stats. See also: gitlab_issue_spent_time_reset, gitlab_issue_time_stats_get.",
	},
	"gitlab_issue_spent_time_reset": {
		usage:       "Reset the total time spent on an issue back to zero.",
		aliases:     []string{"reset spent time", "clear issue spent time"},
		related:     []string{actionIssueSpentTimeAdd, actionIssueTimeStatsGet},
		description: "Reset an issue's spent time. Returns: the updated time tracking stats. See also: gitlab_issue_spent_time_add, gitlab_issue_time_stats_get.",
	},
	"gitlab_issue_time_stats_get": {
		usage:       "Read the time tracking totals (estimate and time spent) for an issue.",
		aliases:     []string{"get issue time stats", "show issue time tracking"},
		related:     []string{actionIssueTimeEstSet, "issue.time_estimate_reset", actionIssueSpentTimeAdd, "issue.spent_time_reset"},
		description: "Read an issue's time tracking totals. Returns: estimate and spent time in seconds and human-readable form. See also: gitlab_issue_time_estimate_set, gitlab_issue_spent_time_add.",
	},
	"gitlab_issue_participants": {
		usage:       "List the users participating in an issue (author, assignees, commenters, and subscribers).",
		aliases:     []string{"list issue participants", "who is on this issue"},
		related:     []string{actionIssueGet, "issue.notes_list"},
		description: "List an issue's participants. Returns: participating users with username and name. See also: gitlab_issue_get, gitlab_issue_notes_list.",
	},
	"gitlab_issue_mrs_closing": {
		usage:       "List merge requests that will close this issue when merged (those referencing it with a closing keyword).",
		aliases:     []string{"list mrs closing issue", "merge requests that close issue"},
		related:     []string{actionIssueGet, "merge_request.get", "issue.mrs_related"},
		description: "List MRs that close this issue on merge. Returns: related merge requests with state, author, and branches. See also: gitlab_issue_get, gitlab_issue_mrs_related.",
	},
	"gitlab_issue_mrs_related": {
		usage:       "List merge requests related to this issue (those mentioning it), a broader set than the closing MRs.",
		aliases:     []string{"list mrs related to issue", "merge requests mentioning issue"},
		related:     []string{actionIssueGet, "merge_request.get", "issue.mrs_closing"},
		description: "List MRs related to this issue. Returns: related merge requests with state, author, and branches. See also: gitlab_issue_get, gitlab_issue_mrs_closing.",
	},
}

// issueCreateSpec builds a create-style [toolutil.ActionSpec] for an issue
// action and fills in the usage, related actions, and parameter guidance
// for the create and create_todo individual tools.
func issueCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	if individualTool != "gitlab_issue_create" {
		decorateIssueMeta(&options, individualTool)
		return toolutil.NewCreateActionSpec(name, route, options)
	}
	options.Usage = "Create a new issue in a known project. Provide project_id and a clear title, then add description, labels, assignee_ids, milestone_id, due_date, confidential, or task metadata only when requested."
	options.Aliases = []string{"open issue", "create bug report", "file issue"}
	options.RelatedActions = []string{actionIssueGet, actionIssueList, actionIssueUpdate}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramProjectID: {
			SemanticRole:     roleScopeProject,
			ValueSource:      "Project where the issue should be created.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the target project path or numeric ID; do not substitute group_id or repository URL."},
		},
		"title": {
			SemanticRole:   "issue_title",
			ValueSource:    "Short issue summary from the user's request.",
			ExampleBinding: `params.title:"OAuth login fails after redirect"`,
		},
		"due_date": {
			SemanticRole:     "calendar_date",
			ValueSource:      "Requested due date in ISO format when the user specifies one.",
			ExampleBinding:   `params.due_date:"2026-06-01"`,
			CommonConfusions: []string{"Use YYYY-MM-DD; natural-language dates must be normalized before calling the tool."},
		},
	}
	options.IndividualTool.Description = "Create a new issue in a project. Returns: the created issue with IID, state, labels, assignees, milestone, due date, and web URL. See also: gitlab_issue_get, gitlab_issue_list, gitlab_issue_update."
	return toolutil.NewCreateActionSpec(name, route, options)
}

// issueUpdateSpec builds an update-style [toolutil.ActionSpec] for an
// issue action using the package's default [issueOptions].
func issueUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	decorateIssueMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// issueUpdateActionSpec builds the special update spec for the
// gitlab_issue_update individual tool, adding the state_event schema
// override and explicit guidance for the close/reopen aliases.
func issueUpdateActionSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := issueOptions("gitlab_issue_update")
	options.Usage = "Update issue fields. To close or reopen an issue with issue.update, set params.state_event to close or reopen; dynamic execute also accepts issue.close and issue.reopen aliases that fill state_event automatically."
	options.Aliases = []string{"close issue", "reopen issue", "change issue state", "transition issue"}
	options.RelatedActions = []string{actionIssueGet, "issue.delete", actionIssueList}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramStateEvent: {
			SemanticRole:     "issue_state_transition",
			ValueSource:      "task intent when closing or reopening an issue",
			CommonConfusions: []string{"Do not use state=closed/opened for transitions; use state_event=close or state_event=reopen."},
			ExampleBinding:   `{paramStateEvent:"close"}`,
		},
	}
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		{
			PropertyPath: paramStateEvent,
			Values: map[string]any{
				"enum":        []any{"close", "reopen"},
				"description": "State transition; set to close or reopen when changing issue state.",
			},
		},
	}
	return toolutil.NewUpdateActionSpec("update", toolutil.RouteAction(client, Update), options)
}

// issueDeleteSpec builds a destructive [toolutil.ActionSpec] for an issue
// action using the package's default [issueOptions].
func issueDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	decorateIssueMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// issueOptions returns the base [toolutil.ActionSpecOptions] shared by
// every issue action (tags, owner, individual tool metadata).
func issueOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issues domain action.", Tags: []string{"issue"},
		OpenWorld:      true,
		OwnerPackage:   domainIssues,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// groupIssueReadSpec builds a read-only [toolutil.ActionSpec] for a group
// issue action and fills in the usage and related actions for the
// gitlab_issue_list_group individual tool.
func groupIssueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupIssueOptions(individualTool)
	if individualTool == toolIssueListGroup {
		options.Usage = "List issues across a group and its projects. Use this when the prompt scopes work to a group or subgroup rather than a single project."
		options.Aliases = []string{"list group issues", "show issues in group", "find issues across group"}
		options.RelatedActions = []string{actionIssueList, "group.get", actionSearchIssues}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group ID or full group path from the user's request.",
				ExampleBinding:   `params.group_id:"platform/backend"`,
				CommonConfusions: []string{"Use group_id for group paths or IDs; use project_id only with issue.list for a single project."},
			},
		}
		options.IndividualTool.Description = "List issues across a group. Returns: matching issues from projects in the group with pagination metadata. See also: gitlab_issue_list, gitlab_group_get, gitlab_search_issues."
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

// groupIssueOptions returns the base [toolutil.ActionSpecOptions] shared
// by every group-scoped issue action (tags, owner, individual tool
// metadata).
func groupIssueOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issues domain action.", Tags: []string{"group", "issue"},
		OpenWorld:      true,
		OwnerPackage:   domainIssues,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

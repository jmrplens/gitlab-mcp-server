package workitemsavedviews

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs. Saved views are routes on gitlab_issue, the catalog
// group that already owns the work item actions, so every ID is namespaced
// under the issue domain.
const (
	actionGet         = "issue.work_item_saved_view_get"
	actionList        = "issue.work_item_saved_view_list"
	actionCreate      = "issue.work_item_saved_view_create"
	actionUpdate      = "issue.work_item_saved_view_update"
	actionDelete      = "issue.work_item_saved_view_delete"
	actionSubscribe   = "issue.work_item_saved_view_subscribe"
	actionUnsubscribe = "issue.work_item_saved_view_unsubscribe"

	actionWorkItemList = "issue.work_item_list"

	// experimentalNote appears in every description because upstream ships the
	// API with the same warning: the shape may change between minor versions.
	experimentalNote = "Experimental: the Work Item Saved Views API is a work in progress and may introduce breaking changes even between minor versions."
)

// ActionSpecs returns canonical specs for work item saved view actions exposed
// through gitlab_issue.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_work_item_saved_view_get — read one saved view, filters included.
		getSpec(toolutil.RouteAction(client, Get)),
		// gitlab_work_item_saved_view_list — page through a namespace's saved views.
		listSpec(toolutil.RouteAction(client, List)),
		// gitlab_work_item_saved_view_create — store a new named filter.
		createSpec(toolutil.RouteAction(client, Create)),
		// gitlab_work_item_saved_view_update — change an existing saved view.
		updateSpec(toolutil.RouteAction(client, Update)),
		// gitlab_work_item_saved_view_delete — remove a saved view (destructive).
		deleteSpec(toolutil.DestructiveAction(client, Delete)),
		// gitlab_work_item_saved_view_subscribe — follow a saved view.
		subscribeSpec(toolutil.RouteAction(client, Subscribe)),
		// gitlab_work_item_saved_view_unsubscribe — stop following a saved view.
		unsubscribeSpec(toolutil.RouteAction(client, Unsubscribe)),
	}
}

// getSpec builds the canonical read spec for a single saved view.
func getSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_get")
	opts.ContentKind = toolutil.ActionSpecContentDetail
	opts.Usage = "Get one work item saved view by namespace_path plus its numeric saved_view_id. This is the only action that returns the view's filters, so use it whenever the prompt asks what a view actually matches."
	opts.Aliases = append(opts.Aliases, "get work item saved view", "show saved view filters", "read saved view definition")
	opts.RelatedActions = []string{actionList, actionUpdate, actionDelete}
	opts.IndividualTool.Description = "Get a single work item saved view by namespace path and numeric ID. Returns: id, global id, name, description, private flag, subscription state, sort order, filters, and display settings. " + experimentalNote + " See also: gitlab_work_item_saved_view_list, gitlab_work_item_saved_view_update, gitlab_work_item_saved_view_delete."
	return toolutil.NewReadActionSpec("work_item_saved_view_get", route, opts)
}

// listSpec builds the canonical read spec for a page of saved views.
func listSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_list")
	opts.ContentKind = toolutil.ActionSpecContentList
	opts.Usage = "List the work item saved views under one group or project namespace, with cursor pagination. Use this to discover a view's numeric ID before getting, updating, subscribing to, or deleting it. Filters are omitted from every entry, so read them with work_item_saved_view.get."
	opts.Aliases = append(opts.Aliases, "list work item saved views", "show saved views for namespace", "find saved work item filters")
	opts.RelatedActions = []string{actionGet, actionCreate, actionWorkItemList}
	opts.IndividualTool.Description = "List work item saved views for a group or project namespace with cursor pagination. Returns: id, name, description, private flag, subscription state, and sort order per view, plus pagination cursors. Filters are omitted here, so read one view with gitlab_work_item_saved_view_get. " + experimentalNote + " See also: gitlab_work_item_saved_view_get, gitlab_work_item_saved_view_create, gitlab_list_work_items."
	return toolutil.NewReadActionSpec("work_item_saved_view_list", route, opts)
}

// createSpec builds the canonical create spec for a new saved view.
func createSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_create")
	opts.Usage = "Create a work item saved view under a group or project namespace. Provide namespace_path, name, and sort. Add filters to store the query the view represents, description for context, is_private=false to share it, and display_settings only when the consuming UI needs them. An omitted display_settings is stored as an empty object."
	opts.Aliases = append(opts.Aliases, "create work item saved view", "save a work item filter", "store a named work item query")
	opts.RelatedActions = []string{actionList, actionUpdate, actionGet}
	opts.InputSchemaOverrides = mutationSchemaOverrides()
	opts.IndividualTool.Description = "Create a work item saved view under a namespace. Returns: the created view with id, global id, name, private flag, subscription state, sort order, filters, and display settings. " + experimentalNote + " See also: gitlab_work_item_saved_view_list, gitlab_work_item_saved_view_update, gitlab_work_item_saved_view_get."
	return toolutil.NewCreateActionSpec("work_item_saved_view_create", route, opts)
}

// updateSpec builds the canonical update spec for an existing saved view.
func updateSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_update")
	opts.Usage = "Update a work item saved view by its numeric saved_view_id. Every field is optional and an omitted one is left unchanged. Supplying filters replaces the stored filter set wholesale, so read the current one with work_item_saved_view.get first when the intent is to add a condition rather than replace the query."
	opts.Aliases = append(opts.Aliases, "update work item saved view", "rename saved view", "change saved view filters")
	opts.RelatedActions = []string{actionGet, actionList, actionDelete}
	opts.InputSchemaOverrides = mutationSchemaOverrides()
	opts.IndividualTool.Description = "Update a work item saved view by numeric ID. Returns: the updated view with id, global id, name, private flag, subscription state, sort order, filters, and display settings. " + experimentalNote + " See also: gitlab_work_item_saved_view_get, gitlab_work_item_saved_view_list, gitlab_work_item_saved_view_delete."
	return toolutil.NewUpdateActionSpec("work_item_saved_view_update", route, opts)
}

// deleteSpec builds the canonical destructive spec for removing a saved view.
func deleteSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_delete")
	opts.Usage = "Permanently delete a work item saved view by its numeric saved_view_id. Deletion is irreversible and removes the view for everyone it was shared with, so confirm the ID with work_item_saved_view.list first."
	opts.Aliases = append(opts.Aliases, "delete work item saved view", "remove saved view", "discard saved work item filter")
	opts.RelatedActions = []string{actionGet, actionList}
	opts.IndividualTool.Description = "Delete a work item saved view permanently by numeric ID. Returns: a success confirmation naming the view. " + experimentalNote + " See also: gitlab_work_item_saved_view_get, gitlab_work_item_saved_view_list."
	return toolutil.NewDeleteActionSpec("work_item_saved_view_delete", route, opts)
}

// subscribeSpec builds the canonical update spec for subscribing to a view.
func subscribeSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_subscribe")
	opts.Usage = "Subscribe the authenticated user to a work item saved view by its numeric saved_view_id, so the view appears among the user's followed views."
	opts.Aliases = append(opts.Aliases, "subscribe to work item saved view", "follow saved view", "watch saved view")
	opts.RelatedActions = []string{actionUnsubscribe, actionGet, actionList}
	opts.IndividualTool.Description = "Subscribe the authenticated user to a work item saved view. Returns: the view with the updated subscription state. " + experimentalNote + " See also: gitlab_work_item_saved_view_unsubscribe, gitlab_work_item_saved_view_get, gitlab_work_item_saved_view_list."
	return toolutil.NewUpdateActionSpec("work_item_saved_view_subscribe", route, opts)
}

// unsubscribeSpec builds the canonical update spec for unsubscribing from a view.
func unsubscribeSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	opts := savedViewOptions("gitlab_work_item_saved_view_unsubscribe")
	opts.Usage = "Unsubscribe the authenticated user from a work item saved view by its numeric saved_view_id. The view itself is untouched, and only this user stops following it."
	opts.Aliases = append(opts.Aliases, "unsubscribe from work item saved view", "unfollow saved view", "stop watching saved view")
	opts.RelatedActions = []string{actionSubscribe, actionGet, actionList}
	opts.IndividualTool.Description = "Unsubscribe the authenticated user from a work item saved view. Returns: the view with the updated subscription state. " + experimentalNote + " See also: gitlab_work_item_saved_view_subscribe, gitlab_work_item_saved_view_get, gitlab_work_item_saved_view_list."
	return toolutil.NewUpdateActionSpec("work_item_saved_view_unsubscribe", route, opts)
}

// workItemSortValues is the WorkItemSort GraphQL enum, which create and update
// require verbatim. The deprecated lowercase aliases GitLab renamed in 13.5
// (created_asc and friends) are deliberately left out: they still work, and
// offering both spellings would double the list for nothing.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsort
var workItemSortValues = []any{
	"BLOCKING_ISSUES_ASC", "BLOCKING_ISSUES_DESC",
	"CLOSED_AT_ASC", "CLOSED_AT_DESC",
	"CREATED_ASC", "CREATED_DESC",
	"DUE_DATE_ASC", "DUE_DATE_DESC",
	"ESCALATION_STATUS_ASC", "ESCALATION_STATUS_DESC",
	"HEALTH_STATUS_ASC", "HEALTH_STATUS_DESC",
	"LABEL_PRIORITY_ASC", "LABEL_PRIORITY_DESC",
	"MILESTONE_DUE_ASC", "MILESTONE_DUE_DESC",
	"POPULARITY_ASC", "POPULARITY_DESC",
	"PRIORITY_ASC", "PRIORITY_DESC",
	"RELATIVE_POSITION_ASC",
	"SEVERITY_ASC", "SEVERITY_DESC",
	"START_DATE_ASC", "START_DATE_DESC",
	"STATUS_ASC", "STATUS_DESC",
	"TITLE_ASC", "TITLE_DESC",
	"UPDATED_ASC", "UPDATED_DESC",
	"WEIGHT_ASC", "WEIGHT_DESC",
}

// sortEnumOverride constrains the sort parameter to the WorkItemSort enum. An
// unknown value is otherwise forwarded verbatim and rejected by GitLab with an
// opaque GraphQL argument error.
func sortEnumOverride() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("sort", map[string]any{"enum": workItemSortValues}),
	}
}

// filterEnumOverrides constrains the eight filter fields whose GraphQL type is
// an enum to exactly the values that enum documents, so a view GitLab would
// reject is refused at the schema instead of stored and answered with an opaque
// argument error. Create and update share the Filters shape, so every path
// exists on both.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#workitemsavedviewfilterinput
func filterEnumOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		// https://docs.gitlab.com/api/graphql/reference/#assigneewildcardid
		toolutil.SchemaEnumOverride("filters.assignee_wildcard_id", "ANY", "ME", "NONE"),
		// https://docs.gitlab.com/api/graphql/reference/#healthstatusfilter
		toolutil.SchemaEnumOverride("filters.health_status_filter", "ANY", "NONE", "atRisk", "needsAttention", "onTrack"),
		// https://docs.gitlab.com/api/graphql/reference/#workitemparentwildcardid
		toolutil.SchemaEnumOverride("filters.hierarchy_filters.parent_wildcard_id", "ANY", "NONE"),
		// https://docs.gitlab.com/api/graphql/reference/#iterationwildcardid
		toolutil.SchemaEnumOverride("filters.iteration_wildcard_id", "ANY", "CURRENT", "NONE"),
		// https://docs.gitlab.com/api/graphql/reference/#milestonewildcardid
		toolutil.SchemaEnumOverride("filters.milestone_wildcard_id", "ANY", "NONE", "STARTED", "UPCOMING"),
		// The filter's state is IssuableState, not WorkItemState, so the values
		// are lowercase and include locked.
		// https://docs.gitlab.com/api/graphql/reference/#issuablestate
		toolutil.SchemaEnumOverride("filters.state", "all", "closed", "locked", "opened"),
		// https://docs.gitlab.com/api/graphql/reference/#subscriptionstatus
		toolutil.SchemaEnumOverride("filters.subscribed", "EXPLICITLY_SUBSCRIBED", "EXPLICITLY_UNSUBSCRIBED"),
		// https://docs.gitlab.com/api/graphql/reference/#weightwildcardid
		toolutil.SchemaEnumOverride("filters.weight_wildcard_id", "ANY", "NONE"),
	}
}

// mutationSchemaOverrides is the full override set create and update share: the
// sort enum plus the enum-typed filter fields.
func mutationSchemaOverrides() []toolutil.InputSchemaOverride {
	return append(sortEnumOverride(), filterEnumOverrides()...)
}

// savedViewOptions returns the metadata every saved view action shares.
func savedViewOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute workitemsavedviews domain action.",
		Tags:           []string{"issue", "work_item", "saved_view", "graphql"},
		RelatedActions: []string{actionGet, actionList},
		OpenWorld:      true,
		OwnerPackage:   "workitemsavedviews",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
}

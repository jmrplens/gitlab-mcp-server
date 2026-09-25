package labels

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The one block every published action ID in this package is built from, so a
// spec name and the cross-links that point at it cannot name different
// actions.
//
// Project labels are routes on the gitlab_project catalog group, so a label
// action's canonical ID is its spec name under the "project" domain and never
// under a "label" domain of its own: "label.get" and its seven siblings
// resolved to nothing, and a model following one was answered unknown action.
// The group label a promoted project label becomes is a route on the group
// catalog group under its own "group_label_" prefix, which is why
// actionGroupLabelList is spelled out rather than derived here.
const (
	domainPrefix = "project."

	specLabelList        = "label_list"
	specLabelGet         = "label_get"
	specLabelCreate      = "label_create"
	specLabelUpdate      = "label_update"
	specLabelDelete      = "label_delete"
	specLabelSubscribe   = "label_subscribe"
	specLabelUnsubscribe = "label_unsubscribe"
	specLabelPromote     = "label_promote"

	actionLabelList        = domainPrefix + specLabelList
	actionLabelGet         = domainPrefix + specLabelGet
	actionLabelCreate      = domainPrefix + specLabelCreate
	actionLabelUpdate      = domainPrefix + specLabelUpdate
	actionLabelDelete      = domainPrefix + specLabelDelete
	actionLabelSubscribe   = domainPrefix + specLabelSubscribe
	actionLabelUnsubscribe = domainPrefix + specLabelUnsubscribe

	actionProjectGet     = domainPrefix + "get"
	actionIssueList      = "issue.list"
	actionGroupLabelList = "group.group_label_list"

	paramLabelID        = "label_id"
	roleLabelIdentifier = "label_identifier"
	hintLabelNameOrID   = "Label name or ID from task context or label list output."
)

// ActionSpecs returns canonical specs for project label actions exposed
// as MCP tools. The list, get, create, update, delete, subscribe,
// unsubscribe, and promote routes are projected into the dynamic,
// meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_label_list — list project labels with optional search and pagination.
		labelReadSpec(specLabelList, toolutil.RouteAction(client, List), "gitlab_label_list"),
		// gitlab_label_get — fetch a label by ID or name (returns a structured not-found result on 404).
		labelReadSpec(specLabelGet, labelGetRoute(client), "gitlab_label_get").
			WithEmbeddedResource("gitlab://project/{project_id}/label/{label_id}"),
		// gitlab_label_create — create a new project label.
		labelCreateSpec(specLabelCreate, toolutil.RouteAction(client, Create), "gitlab_label_create"),
		// gitlab_label_update — update an existing project label.
		labelUpdateSpec(specLabelUpdate, toolutil.RouteAction(client, Update), "gitlab_label_update"),
		// gitlab_label_delete — remove a project label (destructive).
		labelDeleteSpec(specLabelDelete, toolutil.DestructiveVoidAction(client, Delete), "gitlab_label_delete"),
		// gitlab_label_subscribe — subscribe the caller to label notifications.
		labelUpdateSpec(specLabelSubscribe, toolutil.RouteAction(client, Subscribe), "gitlab_label_subscribe"),
		// gitlab_label_unsubscribe — remove the caller's label subscription.
		labelUpdateSpec(specLabelUnsubscribe, toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_label_unsubscribe"),
		// gitlab_label_promote — promote a project label to a group label.
		labelUpdateSpec(specLabelPromote, toolutil.RouteVoidAction(client, Promote), "gitlab_label_promote"),
	}
}

// labelGetRoute wraps the [Get] route so a 404 response is converted
// into a structured [labelNotFoundOutput] hint rather than an error,
// matching the get-not-found pattern used across the project.
func labelGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				// Both are named as the caller wrote them. Reading them as
				// strings named a label or a project given as a JSON number
				// as none at all.
				return labelNotFoundOutput{Identifier: fmt.Sprintf("ID %s in project %s",
					toolutil.ParamText(input[paramLabelID]), toolutil.ParamText(input["project_id"]))}, nil
			}
			return result, err
		}
	})
}

// labelReadSpec builds a read-only [toolutil.ActionSpec] for a label
// action using the package's default [labelOptionsForAction].
func labelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

// labelCreateSpec builds a create-style [toolutil.ActionSpec] for a
// label action using the package's default [labelOptionsForAction].
func labelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

// labelUpdateSpec builds an update-style [toolutil.ActionSpec] for a
// label action using the package's default [labelOptionsForAction].
func labelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

// labelDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// label action using the package's default [labelOptionsForAction].
func labelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

// labelOptionsForAction returns the base [toolutil.ActionSpecOptions]
// for a label action and customizes the Usage/Aliases for the list,
// get, and create individual tools.
func labelOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute labels domain action.", Tags: []string{"project", "label"},
		RelatedActions: []string{actionProjectGet, actionIssueList},
		OpenWorld:      true,
		OwnerPackage:   "labels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case specLabelList:
		options.Usage = "List labels for a project with optional search and pagination. Use to discover taxonomy before issue/MR filtering or label maintenance."
		options.Aliases = []string{"list labels", "show project labels", "find labels"}
		options.RelatedActions = []string{actionLabelGet, actionLabelCreate, actionIssueList}
		options.IndividualTool.Description = "List labels in a project with optional search, counts, ancestor-group inclusion, ordering, and offset or keyset pagination. Returns: id, name, color, text_color, description, open/closed issue counts, open MR count, priority, subscribed, is_project_label, archived, and pagination metadata. See also: gitlab_label_get, gitlab_label_create, gitlab_issue_list."
	case specLabelGet:
		options.Usage = "Get one label by project_id and label_id (label name/ID route parameter). Use when exact label metadata is needed."
		options.Aliases = []string{"get label", "show label details", "lookup label"}
		options.RelatedActions = []string{actionLabelList, actionLabelUpdate, actionLabelDelete}
		options.IndividualTool.Description = "Get a single project label by ID or name. Returns: id, name, color, text_color, description, open/closed issue counts, open MR count, priority, subscribed, is_project_label, and archived. See also: gitlab_label_list, gitlab_label_update, gitlab_label_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case specLabelCreate:
		options.Usage = "Create a label in a project with required name and color, plus optional description and priority."
		options.Aliases = []string{"create label", "add label", "new label"}
		options.RelatedActions = []string{actionLabelGet, actionLabelUpdate, actionIssueList}
		options.IndividualTool.Description = "Create a project label with required name and hex color, plus optional description, priority, and archived state. Returns: the created label (id, name, color, text_color, description, counts, priority, subscribed, is_project_label, archived). See also: gitlab_label_get, gitlab_label_update, gitlab_issue_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"color": {
				SemanticRole:     "hex_color",
				ValueSource:      "Hex color string for label background (for example #d9534f).",
				ExampleBinding:   `params.color:"#d9534f"`,
				CommonConfusions: []string{"Provide hex color values. Avoid named colors."},
			},
		}
	case specLabelUpdate:
		options.Usage = "Update a project label's name, color, description, priority, or archived state. Identify the label by label_id (ID or name). At least one mutable field is required."
		options.Aliases = []string{"update label", "edit label", "rename label", "recolor label"}
		options.RelatedActions = []string{actionLabelGet, actionLabelList, actionLabelDelete}
		options.IndividualTool.Description = "Update an existing project label (new_name, color, description, priority, archived). Returns: the updated label (id, name, color, text_color, description, counts, priority, subscribed, is_project_label, archived). See also: gitlab_label_get, gitlab_label_list, gitlab_label_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case specLabelDelete:
		options.Usage = "Delete a project label by label_id (ID or name). Destructive and irreversible. Group-inherited labels must be deleted at the group level."
		options.Aliases = []string{"delete label", "remove label", "drop label"}
		options.RelatedActions = []string{actionLabelList, actionLabelGet, actionLabelCreate}
		options.IndividualTool.Description = "Delete a project label by ID or name. Destructive: the label is removed from the project and unassigned from issues and merge requests. Returns: a deletion confirmation. See also: gitlab_label_list, gitlab_label_get, gitlab_label_create."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case specLabelSubscribe:
		options.Usage = "Subscribe the authenticated user to a project label to receive notifications. Identify the label by label_id (ID or name)."
		options.Aliases = []string{"subscribe to label", "follow label", "watch label"}
		options.RelatedActions = []string{actionLabelUnsubscribe, actionLabelGet, actionLabelList}
		options.IndividualTool.Description = "Subscribe the authenticated user to a project label for notifications. Returns: the label with subscribed=true (already-subscribed yields 304 Not Modified). See also: gitlab_label_unsubscribe, gitlab_label_get, gitlab_label_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case specLabelUnsubscribe:
		options.Usage = "Unsubscribe the authenticated user from a project label to stop receiving notifications. Identify the label by label_id (ID or name)."
		options.Aliases = []string{"unsubscribe from label", "unfollow label", "unwatch label"}
		options.RelatedActions = []string{actionLabelSubscribe, actionLabelGet, actionLabelList}
		options.IndividualTool.Description = "Unsubscribe the authenticated user from a project label. Returns: no content on success (not-subscribed yields 304 Not Modified). See also: gitlab_label_subscribe, gitlab_label_get, gitlab_label_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case specLabelPromote:
		options.Usage = "Promote a project label to a group label so it is shared across the group's projects. The project must belong to a group. Personal-namespace projects cannot promote labels."
		options.Aliases = []string{"promote label", "promote to group label", "make group label"}
		options.RelatedActions = []string{actionLabelGet, actionLabelList, actionGroupLabelList}
		options.IndividualTool.Description = "Promote a project label to a group label, sharing it across the group's projects. Returns: no content on success. Requires group-level Maintainer or higher access. See also: gitlab_label_get, gitlab_label_list, gitlab_group_label_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	}

	return options
}

package labels

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionIssueList     = "issue.list"
	actionLabelGet      = "label.get"
	actionLabelList     = "label.list"
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
		labelReadSpec("label_list", toolutil.RouteAction(client, List), "gitlab_label_list"),
		// gitlab_label_get — fetch a label by ID or name (returns a structured not-found result on 404).
		labelReadSpec("label_get", labelGetRoute(client), "gitlab_label_get").
			WithEmbeddedResource("gitlab://project/{project_id}/label/{label_id}"),
		// gitlab_label_create — create a new project label.
		labelCreateSpec("label_create", toolutil.RouteAction(client, Create), "gitlab_label_create"),
		// gitlab_label_update — update an existing project label.
		labelUpdateSpec("label_update", toolutil.RouteAction(client, Update), "gitlab_label_update"),
		// gitlab_label_delete — remove a project label (destructive).
		labelDeleteSpec("label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_label_delete"),
		// gitlab_label_subscribe — subscribe the caller to label notifications.
		labelUpdateSpec("label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_label_subscribe"),
		// gitlab_label_unsubscribe — remove the caller's label subscription.
		labelUpdateSpec("label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_label_unsubscribe"),
		// gitlab_label_promote — promote a project label to a group label.
		labelUpdateSpec("label_promote", toolutil.RouteVoidAction(client, Promote), "gitlab_label_promote"),
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
				labelID, _ := input[paramLabelID].(string)
				projectID, _ := input["project_id"].(string)
				return labelNotFoundOutput{Identifier: fmt.Sprintf("ID %s in project %s", labelID, projectID)}, nil
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
		RelatedActions: []string{"project.get", actionIssueList},
		OpenWorld:      true,
		OwnerPackage:   "labels",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "label_list":
		options.Usage = "List labels for a project with optional search and pagination. Use to discover taxonomy before issue/MR filtering or label maintenance."
		options.Aliases = []string{"list labels", "show project labels", "find labels"}
		options.RelatedActions = []string{actionLabelGet, "label.create", actionIssueList}
		options.IndividualTool.Description = "List labels in a project with optional search, counts, ancestor-group inclusion, ordering, and offset or keyset pagination. Returns: id, name, color, text_color, description, open/closed issue counts, open MR count, priority, subscribed, is_project_label, archived, and pagination metadata. See also: gitlab_label_get, gitlab_label_create, gitlab_issue_list."
	case "label_get":
		options.Usage = "Get one label by project_id and label_id (label name/ID route parameter). Use when exact label metadata is needed."
		options.Aliases = []string{"get label", "show label details", "lookup label"}
		options.RelatedActions = []string{actionLabelList, "label.update", "label.delete"}
		options.IndividualTool.Description = "Get a single project label by ID or name. Returns: id, name, color, text_color, description, open/closed issue counts, open MR count, priority, subscribed, is_project_label, and archived. See also: gitlab_label_list, gitlab_label_update, gitlab_label_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_create":
		options.Usage = "Create a label in a project with required name and color, plus optional description and priority."
		options.Aliases = []string{"create label", "add label", "new label"}
		options.RelatedActions = []string{actionLabelGet, "label.update", actionIssueList}
		options.IndividualTool.Description = "Create a project label with required name and hex color, plus optional description, priority, and archived state. Returns: the created label (id, name, color, text_color, description, counts, priority, subscribed, is_project_label, archived). See also: gitlab_label_get, gitlab_label_update, gitlab_issue_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"color": {
				SemanticRole:     "hex_color",
				ValueSource:      "Hex color string for label background (for example #d9534f).",
				ExampleBinding:   `params.color:"#d9534f"`,
				CommonConfusions: []string{"Provide hex color values. Avoid named colors."},
			},
		}
	case "label_update":
		options.Usage = "Update a project label's name, color, description, priority, or archived state. Identify the label by label_id (ID or name). At least one mutable field is required."
		options.Aliases = []string{"update label", "edit label", "rename label", "recolor label"}
		options.RelatedActions = []string{actionLabelGet, actionLabelList, "label.delete"}
		options.IndividualTool.Description = "Update an existing project label (new_name, color, description, priority, archived). Returns: the updated label (id, name, color, text_color, description, counts, priority, subscribed, is_project_label, archived). See also: gitlab_label_get, gitlab_label_list, gitlab_label_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_delete":
		options.Usage = "Delete a project label by label_id (ID or name). Destructive and irreversible. Group-inherited labels must be deleted at the group level."
		options.Aliases = []string{"delete label", "remove label", "drop label"}
		options.RelatedActions = []string{actionLabelList, actionLabelGet, "label.create"}
		options.IndividualTool.Description = "Delete a project label by ID or name. Destructive: the label is removed from the project and unassigned from issues and merge requests. Returns: a deletion confirmation. See also: gitlab_label_list, gitlab_label_get, gitlab_label_create."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_subscribe":
		options.Usage = "Subscribe the authenticated user to a project label to receive notifications. Identify the label by label_id (ID or name)."
		options.Aliases = []string{"subscribe to label", "follow label", "watch label"}
		options.RelatedActions = []string{"label.unsubscribe", actionLabelGet, actionLabelList}
		options.IndividualTool.Description = "Subscribe the authenticated user to a project label for notifications. Returns: the label with subscribed=true (already-subscribed yields 304 Not Modified). See also: gitlab_label_unsubscribe, gitlab_label_get, gitlab_label_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_unsubscribe":
		options.Usage = "Unsubscribe the authenticated user from a project label to stop receiving notifications. Identify the label by label_id (ID or name)."
		options.Aliases = []string{"unsubscribe from label", "unfollow label", "unwatch label"}
		options.RelatedActions = []string{"label.subscribe", actionLabelGet, actionLabelList}
		options.IndividualTool.Description = "Unsubscribe the authenticated user from a project label. Returns: no content on success (not-subscribed yields 304 Not Modified). See also: gitlab_label_subscribe, gitlab_label_get, gitlab_label_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramLabelID: {
				SemanticRole:   roleLabelIdentifier,
				ValueSource:    hintLabelNameOrID,
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_promote":
		options.Usage = "Promote a project label to a group label so it is shared across the group's projects. The project must belong to a group. Personal-namespace projects cannot promote labels."
		options.Aliases = []string{"promote label", "promote to group label", "make group label"}
		options.RelatedActions = []string{actionLabelGet, actionLabelList, "group_label.list"}
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

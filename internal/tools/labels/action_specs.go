package labels

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionIssueList = "issue.list"

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
		labelReadSpec("label_get", labelGetRoute(client), "gitlab_label_get"),
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
		options.RelatedActions = []string{"label.get", "label.create", actionIssueList}
	case "label_get":
		options.Usage = "Get one label by project_id and label_id (label name/ID route parameter). Use when exact label metadata is needed."
		options.Aliases = []string{"get label", "show label details", "lookup label"}
		options.RelatedActions = []string{"label.list", "label.update", "label.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"label_id": {
				SemanticRole:   "label_identifier",
				ValueSource:    "Label name or ID from task context or label list output.",
				ExampleBinding: `params.label_id:"bug"`,
			},
		}
	case "label_create":
		options.Usage = "Create a label in a project with required name and color, plus optional description and priority."
		options.Aliases = []string{"create label", "add label", "new label"}
		options.RelatedActions = []string{"label.get", "label.update", actionIssueList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"color": {
				SemanticRole:     "hex_color",
				ValueSource:      "Hex color string for label background (for example #d9534f).",
				ExampleBinding:   `params.color:"#d9534f"`,
				CommonConfusions: []string{"Provide hex color values; avoid named colors."},
			},
		}
	}

	return options
}

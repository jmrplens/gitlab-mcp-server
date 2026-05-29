package labels

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionIssueList = "issue.list"

// ActionSpecs returns canonical specs for project label actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		labelReadSpec("label_list", toolutil.RouteAction(client, List), "gitlab_label_list"),
		labelReadSpec("label_get", labelGetRoute(client), "gitlab_label_get"),
		labelCreateSpec("label_create", toolutil.RouteAction(client, Create), "gitlab_label_create"),
		labelUpdateSpec("label_update", toolutil.RouteAction(client, Update), "gitlab_label_update"),
		labelDeleteSpec("label_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_label_delete"),
		labelUpdateSpec("label_subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_label_subscribe"),
		labelUpdateSpec("label_unsubscribe", toolutil.RouteVoidAction(client, Unsubscribe), "gitlab_label_unsubscribe"),
		labelUpdateSpec("label_promote", toolutil.RouteVoidAction(client, Promote), "gitlab_label_promote"),
	}
}

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

func labelReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

func labelCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

func labelUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

func labelDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, labelOptionsForAction(name, individualTool))
}

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

package freezeperiods

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deploy freeze period actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		freezePeriodReadSpec("freeze_list", toolutil.RouteAction(client, List), "gitlab_list_freeze_periods"),
		freezePeriodReadSpec("freeze_get", toolutil.RouteAction(client, Get), "gitlab_get_freeze_period"),
		freezePeriodCreateSpec("freeze_create", toolutil.RouteAction(client, Create), "gitlab_create_freeze_period"),
		freezePeriodUpdateSpec("freeze_update", toolutil.RouteAction(client, Update), "gitlab_update_freeze_period"),
		freezePeriodDeleteSpec("freeze_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_freeze_period"),
	}
}

func freezePeriodReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, freezePeriodOptions(name, individualTool))
}

func freezePeriodCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, freezePeriodOptions(name, individualTool))
}

func freezePeriodUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, freezePeriodOptions(name, individualTool))
}

func freezePeriodDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, freezePeriodOptions(name, individualTool))
}

func freezePeriodOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	aliases := []string{}
	usage := ""

	guidance := map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path where the freeze window is configured.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
	}

	switch actionName {
	case "freeze_list":
		aliases = []string{"list deploy freezes", "freeze window list", "deployment freeze list"}
		usage = "List deploy freeze periods for a project."
	case "freeze_get":
		aliases = []string{"get deploy freeze", "freeze window details", "deployment freeze get"}
		usage = "Get a specific deploy freeze period."
	case "freeze_create":
		aliases = []string{"create deploy freeze", "add freeze window", "deployment freeze create"}
		usage = "Create a deploy freeze period for a project."
	case "freeze_update":
		aliases = []string{"update deploy freeze", "edit freeze window", "deployment freeze update"}
		usage = "Update an existing deploy freeze period."
	case "freeze_delete":
		aliases = []string{"delete deploy freeze", "remove freeze window", "deployment freeze delete"}
		usage = "Delete a deploy freeze period."
	}

	if actionName != "freeze_list" && actionName != "freeze_create" {
		guidance["freeze_period_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "freeze_period_id",
			ValueSource:    "Freeze period numeric ID from list/get outputs.",
			ExampleBinding: "params.freeze_period_id:5",
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"environment", "freeze_period", "deployment"},
		Usage:             usage,
		RelatedActions:    []string{"environment.list", "deployment.list", "pipeline.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "freezeperiods",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

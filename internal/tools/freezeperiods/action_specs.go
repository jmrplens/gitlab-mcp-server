package freezeperiods

import (
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// freezePeriodActionMetaEntry is the discovery metadata of one freeze period
// action: the natural-language aliases find matches and its Usage line.
type freezePeriodActionMetaEntry struct {
	aliases []string
	usage   string
}

// freezePeriodActionMeta maps each freeze period action to its discovery
// metadata. It is a table rather than a switch so that every action is looked
// up the same way: the switch's last case was evaluated true by every action
// that reached it, since each other action matched an earlier case, and only
// a name the package never serves could have evaluated it false.
var freezePeriodActionMeta = map[string]freezePeriodActionMetaEntry{
	"freeze_list": {
		aliases: []string{"list deploy freezes", "freeze window list", "deployment freeze list"},
		usage:   "List deploy freeze periods for a project. Returns: each freeze period's id, freeze_start/freeze_end cron expressions, cron_timezone, and created/updated timestamps.",
	},
	"freeze_get": {
		aliases: []string{"get deploy freeze", "freeze window details", "deployment freeze get"},
		usage:   "Get a specific deploy freeze period. Returns: the freeze period's id, freeze_start/freeze_end cron expressions, cron_timezone, and created/updated timestamps.",
	},
	"freeze_create": {
		aliases: []string{"create deploy freeze", "add freeze window", "deployment freeze create"},
		usage:   "Create a deploy freeze period for a project. Returns: the created freeze period with id, freeze_start/freeze_end cron expressions, and cron_timezone.",
	},
	"freeze_update": {
		aliases: []string{"update deploy freeze", "edit freeze window", "deployment freeze update"},
		usage:   "Update an existing deploy freeze period. Returns: the updated freeze period with id, freeze_start/freeze_end cron expressions, and cron_timezone.",
	},
	"freeze_delete": {
		aliases: []string{"delete deploy freeze", "remove freeze window", "deployment freeze delete"},
		usage:   "Delete a deploy freeze period. Returns: no content on success (HTTP 204).",
	},
}

func freezePeriodOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	meta := freezePeriodActionMeta[actionName]

	guidance := map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path where the freeze window is configured.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
	}

	if actionName != "freeze_list" && actionName != "freeze_create" {
		guidance["freeze_period_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "freeze_period_id",
			ValueSource:    "Freeze period numeric ID from list/get outputs.",
			ExampleBinding: "params.freeze_period_id:5",
		}
	}

	// GitLab refuses a freeze period whose start or end Fugit cannot parse
	// (CronValidator on both), and find's example wrote "value" for each
	// (issue 1272).
	var overrides []toolutil.InputSchemaOverride
	if actionName == "freeze_create" || actionName == "freeze_update" {
		overrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPatternOverride("freeze_start", toolutil.CronPattern),
			toolutil.SchemaPatternOverride("freeze_end", toolutil.CronPattern),
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:              slices.Clone(meta.aliases),
		Tags:                 []string{"environment", "freeze_period", "deployment"},
		Usage:                meta.usage,
		RelatedActions:       []string{"environment.list", "environment.deployment_list", "pipeline.list"},
		ParameterGuidance:    guidance,
		InputSchemaOverrides: overrides,
		OpenWorld:            true,
		OwnerPackage:         "freezeperiods",
		IndividualTool:       toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

package pipelineschedules

import (
	"context"
	"fmt"
	"maps"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action IDs referenced across schedule discovery metadata.
const (
	actionScheduleList     = "pipeline_schedule.schedule_list"
	actionScheduleGet      = "pipeline_schedule.schedule_get"
	actionScheduleUpdate   = "pipeline_schedule.schedule_update"
	actionScheduleRun      = "pipeline_schedule.schedule_run"
	actionPipelineList     = "pipeline.list"
	paramScheduleID        = "schedule_id"
	rolePipelineScheduleID = "pipeline_schedule_id"
	exampleScheduleID      = "params.schedule_id:13"
)

// ActionSpecs returns canonical specs for pipeline schedule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineScheduleReadSpec("schedule_list", toolutil.RouteAction(client, List), "gitlab_pipeline_schedule_list"),
		pipelineScheduleReadSpec("schedule_get", toolutil.RouteAction(client, Get), "gitlab_pipeline_schedule_get"),
		pipelineScheduleCreateSpec("schedule_create", toolutil.RouteAction(client, Create), "gitlab_pipeline_schedule_create"),
		pipelineScheduleUpdateSpec("schedule_update", toolutil.RouteAction(client, Update), "gitlab_pipeline_schedule_update"),
		pipelineScheduleDeleteSpec("schedule_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_pipeline_schedule_delete"),
		pipelineScheduleUpdateSpec("schedule_run", toolutil.RouteAction(client, Run), "gitlab_pipeline_schedule_run"),
		pipelineScheduleUpdateSpec("schedule_take_ownership", toolutil.RouteAction(client, TakeOwnership), "gitlab_pipeline_schedule_take_ownership"),
		pipelineScheduleCreateSpec("schedule_create_variable", toolutil.RouteAction(client, CreateVariable), "gitlab_pipeline_schedule_create_variable"),
		pipelineScheduleUpdateSpec("schedule_edit_variable", toolutil.RouteAction(client, EditVariable), "gitlab_pipeline_schedule_edit_variable"),
		pipelineScheduleDeleteSpec("schedule_delete_variable", toolutil.DestructiveAction(client, deleteVariableOutput), "gitlab_pipeline_schedule_delete_variable"),
		pipelineScheduleReadSpec("schedule_list_triggered_pipelines", toolutil.RouteAction(client, ListTriggeredPipelines), "gitlab_pipeline_schedule_list_triggered_pipelines"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pipeline schedule")
	return out, nil
}

func deleteVariableOutput(ctx context.Context, client *gitlabclient.Client, input DeleteVariableInput) (toolutil.DeleteOutput, error) {
	if err := DeleteVariable(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("pipeline schedule variable %q", input.Key))
	return out, nil
}

func pipelineScheduleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, pipelineScheduleOptions(name, individualTool))
}

func pipelineScheduleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pipelineScheduleOptions(name, individualTool))
}

func pipelineScheduleUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pipelineScheduleOptions(name, individualTool))
}

func pipelineScheduleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pipelineScheduleOptions(name, individualTool))
}

// scheduleActionMetaEntry is the discovery metadata for one schedule action.
type scheduleActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
	guidance    map[string]toolutil.ParameterGuidance
}

// scheduleActionMeta maps each individual schedule tool to its non-generic
// discovery metadata (Usage, natural-language Aliases, canonical
// RelatedActions, and the "Returns: … See also: …" individual-tool
// description) per the 1:1 audit R-META requirement.
var scheduleActionMeta = map[string]scheduleActionMetaEntry{
	"gitlab_pipeline_schedule_list": {
		usage:   "List pipeline schedules in a project. Use scope to filter active or inactive schedules and order_by/sort with pagination='keyset' for large ordered result sets.",
		aliases: []string{"list pipeline schedules", "show scheduled pipelines", "list cron pipelines"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_create"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or full namespace path whose pipeline schedules should be listed.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"scope": {
				ValueSource:      "active or inactive when the prompt asks only for enabled or disabled schedules.",
				ExampleBinding:   `params.scope:"active"`,
				CommonConfusions: []string{"scope accepts only active or inactive. It does not filter by ref or owner."},
			},
		},
		description: "List pipeline schedules in a project. Returns: schedules with description, ref, cron, owner, and pagination metadata. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_create.",
	},
	"gitlab_pipeline_schedule_get": {
		usage:   "Get one pipeline schedule by schedule_id. Use after listing schedules or when the prompt names a concrete schedule. The single-schedule payload includes last_pipeline, variables, and inputs.",
		aliases: []string{"get pipeline schedule", "show schedule details", "fetch scheduled pipeline"},
		related: []string{actionScheduleList, actionScheduleUpdate, actionScheduleRun},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:     rolePipelineScheduleID,
				ValueSource:      "Numeric schedule ID from a prior list, usually shown in the schedule URL.",
				ExampleBinding:   exampleScheduleID,
				CommonConfusions: []string{"schedule_id is the database ID, not the schedule description or cron string."},
			},
		},
		description: "Get a single pipeline schedule by ID. Returns: the schedule with cron, ref, owner, last_pipeline, variables, and inputs. See also: gitlab_pipeline_schedule_list, gitlab_pipeline_schedule_update, gitlab_pipeline_schedule_run.",
	},
	"gitlab_pipeline_schedule_create": {
		usage:   "Create a pipeline schedule in a project. Provide description, ref, and a 5-field cron expression. Add cron_timezone, active, or inputs only when requested.",
		aliases: []string{"create pipeline schedule", "schedule a pipeline", "add cron pipeline"},
		related: []string{actionScheduleGet, actionScheduleList, actionScheduleUpdate},
		guidance: map[string]toolutil.ParameterGuidance{
			"cron": {
				SemanticRole:     "cron_expression",
				ValueSource:      "Standard 5-field cron expression for when the pipeline should run.",
				ExampleBinding:   `params.cron:"0 4 * * *"`,
				CommonConfusions: []string{"Use a 5-field cron string. Do not pass natural-language schedules like every night."},
			},
			"ref": {
				ValueSource:      "Branch or tag the scheduled pipeline runs against.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"ref must be an existing branch or tag in the project."},
			},
		},
		description: "Create a pipeline schedule. Returns: the created schedule with id, description, ref, cron, owner, and inputs. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_update, gitlab_pipeline_schedule_list.",
	},
	"gitlab_pipeline_schedule_update": {
		usage:   "Edit a pipeline schedule's description, ref, cron, cron_timezone, active state, or inputs. Only the schedule owner can edit. Take ownership first if needed.",
		aliases: []string{"update pipeline schedule", "edit scheduled pipeline", "change schedule cron"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_take_ownership", actionScheduleList},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:   rolePipelineScheduleID,
				ValueSource:    "Numeric ID of the schedule to edit, from a prior list or get.",
				ExampleBinding: exampleScheduleID,
			},
		},
		description: "Update a pipeline schedule. Returns: the updated schedule with cron, ref, owner, and inputs. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_take_ownership.",
	},
	"gitlab_pipeline_schedule_delete": {
		usage:   "Permanently delete a pipeline schedule. Destructive and irreversible. Confirm project_id and schedule_id before calling.",
		aliases: []string{"delete pipeline schedule", "remove scheduled pipeline", "destroy pipeline schedule", "drop pipeline schedule"},
		related: []string{actionScheduleGet, actionScheduleList},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:   rolePipelineScheduleID,
				ValueSource:    "Numeric ID of the schedule to delete, from a prior list or get.",
				ExampleBinding: exampleScheduleID,
			},
		},
		description: "Delete a pipeline schedule permanently. Returns: a success confirmation. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_list.",
	},
	"gitlab_pipeline_schedule_run": {
		usage:   "Trigger a pipeline schedule to run immediately. Only the schedule owner can run it. Runs are rate-limited to roughly one per minute.",
		aliases: []string{"run pipeline schedule", "trigger scheduled pipeline now", "play schedule"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_list_triggered_pipelines", "pipeline_schedule.schedule_take_ownership"},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:   rolePipelineScheduleID,
				ValueSource:    "Numeric ID of the schedule to trigger, from a prior list or get.",
				ExampleBinding: exampleScheduleID,
			},
		},
		description: "Run a pipeline schedule immediately. Returns: the schedule's current state after triggering. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_list_triggered_pipelines.",
	},
	"gitlab_pipeline_schedule_take_ownership": {
		usage:   "Take ownership of a pipeline schedule so the authenticated user becomes its owner. Requires Maintainer+ and is a prerequisite for editing or running schedules owned by others.",
		aliases: []string{"take ownership of pipeline schedule", "claim schedule ownership", "become schedule owner"},
		related: []string{actionScheduleGet, actionScheduleUpdate, actionScheduleRun},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:   rolePipelineScheduleID,
				ValueSource:    "Numeric ID of the schedule whose ownership to take.",
				ExampleBinding: exampleScheduleID,
			},
		},
		description: "Take ownership of a pipeline schedule. Returns: the schedule with the caller as owner. See also: gitlab_pipeline_schedule_update, gitlab_pipeline_schedule_run.",
	},
	"gitlab_pipeline_schedule_create_variable": {
		usage:   "Create a pipeline schedule variable. Provide key and value. Value is always required. Only the schedule owner can manage variables.",
		aliases: []string{"create pipeline schedule variable", "add schedule variable", "set schedule env var"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_edit_variable", "pipeline_schedule.schedule_delete_variable"},
		guidance: map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:     "ci_variable_key",
				ValueSource:      "Variable name matching /^[A-Za-z_][A-Za-z0-9_]*$/.",
				ExampleBinding:   `params.key:"DEPLOY_ENV"`,
				CommonConfusions: []string{"key must already not exist on the schedule. Use the edit action to change an existing key."},
			},
		},
		description: "Create a pipeline schedule variable. Returns: the created variable with key, value, and variable_type. See also: gitlab_pipeline_schedule_edit_variable, gitlab_pipeline_schedule_delete_variable.",
	},
	"gitlab_pipeline_schedule_edit_variable": {
		usage:   "Edit an existing pipeline schedule variable by key. Provide the new value. Value is always required. Only the schedule owner can manage variables.",
		aliases: []string{"edit pipeline schedule variable", "update schedule variable", "change schedule env var"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_create_variable", "pipeline_schedule.schedule_delete_variable"},
		guidance: map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Existing variable name on the schedule to update.",
				ExampleBinding: `params.key:"DEPLOY_ENV"`,
			},
		},
		description: "Edit a pipeline schedule variable. Returns: the updated variable with key, value, and variable_type. See also: gitlab_pipeline_schedule_create_variable, gitlab_pipeline_schedule_delete_variable.",
	},
	"gitlab_pipeline_schedule_delete_variable": {
		usage:   "Delete a pipeline schedule variable by key. Destructive. Confirm project_id, schedule_id, and key before calling.",
		aliases: []string{"delete pipeline schedule variable", "remove schedule variable", "destroy pipeline schedule variable", "drop schedule variable"},
		related: []string{actionScheduleGet, "pipeline_schedule.schedule_create_variable", "pipeline_schedule.schedule_edit_variable"},
		guidance: map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Existing variable name on the schedule to delete.",
				ExampleBinding: `params.key:"DEPLOY_ENV"`,
			},
		},
		description: "Delete a pipeline schedule variable. Returns: a success confirmation naming the key. See also: gitlab_pipeline_schedule_create_variable, gitlab_pipeline_schedule_edit_variable.",
	},
	"gitlab_pipeline_schedule_list_triggered_pipelines": {
		usage:   "List the pipelines that a schedule has triggered. Use order-independent offset or keyset pagination for long histories.",
		aliases: []string{"list pipelines triggered by schedule", "show scheduled pipeline runs", "schedule run history"},
		related: []string{actionScheduleGet, actionScheduleRun, actionPipelineList},
		guidance: map[string]toolutil.ParameterGuidance{
			paramScheduleID: {
				SemanticRole:   rolePipelineScheduleID,
				ValueSource:    "Numeric ID of the schedule whose triggered pipelines to list.",
				ExampleBinding: exampleScheduleID,
			},
		},
		description: "List pipelines triggered by a schedule. Returns: triggered pipelines with status, ref, sha, web URL, and pagination metadata. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_schedule_run.",
	},
}

// decorateScheduleMeta applies the non-generic discovery metadata for an
// individual schedule tool, falling back to the generic options for any tool
// not present in scheduleActionMeta.
func decorateScheduleMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := scheduleActionMeta[individualTool]
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
	maps.Copy(options.ParameterGuidance, meta.guidance)
}

func pipelineScheduleOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute pipelineschedules domain action.", Tags: []string{"ci", "pipeline", "schedule"},
		OpenWorld:         true,
		OwnerPackage:      "pipelineschedules",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{},
	}
	decorateScheduleMeta(&options, individualTool)
	if actionName == "schedule_create_variable" || actionName == "schedule_edit_variable" {
		options.ParameterGuidance["value"] = toolutil.ParameterGuidance{
			SemanticRole: "pipeline_schedule_variable_value",
			ValueSource:  "Required variable value to store on the schedule. Supply an explicit value even when the task only names the key.",
		}
	}
	return options
}

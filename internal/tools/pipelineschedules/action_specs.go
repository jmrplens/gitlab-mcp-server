package pipelineschedules

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

func pipelineScheduleOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute pipelineschedules domain action.", Tags: []string{"ci", "pipeline", "schedule"},
		OpenWorld:      true,
		OwnerPackage:   "pipelineschedules",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "schedule_create_variable" || actionName == "schedule_edit_variable" {
		options.Usage = "Create or update a pipeline schedule variable. The value parameter is required for both create and edit operations."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"value": {
				SemanticRole: "pipeline_schedule_variable_value",
				ValueSource:  "Required variable value to store on the schedule; supply an explicit value even when the task only names the key.",
			},
		}
	}
	return options
}

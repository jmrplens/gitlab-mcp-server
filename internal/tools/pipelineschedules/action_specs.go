package pipelineschedules

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for pipeline schedule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineScheduleReadSpec("schedule_list", toolutil.RouteAction(client, List), "gitlab_pipeline_schedule_list"),
		pipelineScheduleReadSpec("schedule_get", toolutil.RouteAction(client, Get), "gitlab_pipeline_schedule_get"),
		pipelineScheduleCreateSpec("schedule_create", toolutil.RouteAction(client, Create), "gitlab_pipeline_schedule_create"),
		pipelineScheduleUpdateSpec("schedule_update", toolutil.RouteAction(client, Update), "gitlab_pipeline_schedule_update"),
		pipelineScheduleDeleteSpec("schedule_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_pipeline_schedule_delete"),
		pipelineScheduleUpdateSpec("schedule_run", toolutil.RouteAction(client, Run), "gitlab_pipeline_schedule_run"),
		pipelineScheduleUpdateSpec("schedule_take_ownership", toolutil.RouteAction(client, TakeOwnership), "gitlab_pipeline_schedule_take_ownership"),
		pipelineScheduleCreateSpec("schedule_create_variable", toolutil.RouteAction(client, CreateVariable), "gitlab_pipeline_schedule_create_variable"),
		pipelineScheduleUpdateSpec("schedule_edit_variable", toolutil.RouteAction(client, EditVariable), "gitlab_pipeline_schedule_edit_variable"),
		pipelineScheduleDeleteSpec("schedule_delete_variable", toolutil.DestructiveVoidAction(client, DeleteVariable), "gitlab_pipeline_schedule_delete_variable"),
		pipelineScheduleReadSpec("schedule_list_triggered_pipelines", toolutil.RouteAction(client, ListTriggeredPipelines), "gitlab_pipeline_schedule_list_triggered_pipelines"),
	}
}

func pipelineScheduleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineScheduleOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineScheduleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, pipelineScheduleOptions(individualTool))
}

func pipelineScheduleUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineScheduleOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineScheduleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineScheduleOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineScheduleOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "pipeline", "schedule"},
		OpenWorld:      true,
		OwnerPackage:   "pipelineschedules",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

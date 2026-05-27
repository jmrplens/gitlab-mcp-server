package pipelinetriggers

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for pipeline trigger actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineTriggerReadSpec("trigger_list", toolutil.RouteAction(client, ListTriggers), "gitlab_pipeline_trigger_list"),
		pipelineTriggerReadSpec("trigger_get", toolutil.RouteAction(client, GetTrigger), "gitlab_pipeline_trigger_get"),
		pipelineTriggerCreateSpec("trigger_create", toolutil.RouteAction(client, CreateTrigger), "gitlab_pipeline_trigger_create"),
		pipelineTriggerUpdateSpec("trigger_update", toolutil.RouteAction(client, UpdateTrigger), "gitlab_pipeline_trigger_update"),
		pipelineTriggerDeleteSpec("trigger_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_pipeline_trigger_delete"),
		pipelineTriggerCreateSpec("trigger_run", toolutil.RouteAction(client, RunTrigger), "gitlab_pipeline_trigger_run"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTrigger(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("pipeline trigger")
	return out, nil
}

func pipelineTriggerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, pipelineTriggerOptions(individualTool))
}

func pipelineTriggerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pipelineTriggerOptions(individualTool))
}

func pipelineTriggerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pipelineTriggerOptions(individualTool))
}

func pipelineTriggerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pipelineTriggerOptions(individualTool))
}

func pipelineTriggerOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute pipelinetriggers domain action.", Tags: []string{"ci", "pipeline", "trigger"},
		OpenWorld:      true,
		OwnerPackage:   "pipelinetriggers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

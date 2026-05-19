package pipelines

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD pipeline actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineReadSpec("list", toolutil.RouteAction(client, List), "gitlab_pipeline_list"),
		pipelineReadSpec("get", pipelineGetRoute(client), "gitlab_pipeline_get"),
		pipelineUpdateSpec("cancel", toolutil.RouteAction(client, Cancel), "gitlab_pipeline_cancel"),
		pipelineUpdateSpec("retry", toolutil.RouteAction(client, Retry), "gitlab_pipeline_retry"),
		pipelineDeleteSpec("delete", toolutil.RouteAction(client, DeleteOutput), "gitlab_pipeline_delete"),
		pipelineReadSpec("variables", toolutil.RouteAction(client, GetVariables), "gitlab_pipeline_variables"),
		pipelineReadSpec("test_report", toolutil.RouteAction(client, GetTestReport), "gitlab_pipeline_test_report"),
		pipelineReadSpec("test_report_summary", toolutil.RouteAction(client, GetTestReportSummary), "gitlab_pipeline_test_report_summary"),
		pipelineReadSpec("latest", toolutil.RouteAction(client, GetLatest), "gitlab_pipeline_latest"),
		pipelineMutationSpec("create", toolutil.RouteAction(client, Create), "gitlab_pipeline_create"),
		pipelineUpdateSpec("update_metadata", toolutil.RouteAction(client, UpdateMetadata), "gitlab_pipeline_update_metadata"),
		pipelineReadSpec("wait", toolutil.RouteActionWithRequest(client, Wait), "gitlab_pipeline_wait"),
	}
}

func pipelineGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return pipelineNotFoundOutput{Identifier: fmt.Sprintf("ID %v in project %v", input["pipeline_id"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

// DeleteOutput deletes a pipeline and returns the legacy success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted pipeline %d from project %s.", input.PipelineID, input.ProjectID)}, nil
}

func pipelineReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(name, individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineMutationSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, pipelineOptions(name, individualTool))
}

func pipelineUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(name, individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(name, individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "pipeline"},
		OpenWorld:      true,
		OwnerPackage:   "pipelines",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "wait" {
		options.Usage = "Use only to poll an existing pipeline_id until a terminal status. For merge when pipeline succeeds, use merge_request.merge with auto_merge=true instead."
		options.Aliases = []string{"wait for pipeline", "poll pipeline status", "wait pipeline completion"}
		options.RelatedActions = []string{"pipeline.get", "pipeline.list", "merge_request.pipelines", "merge_request.merge"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"pipeline_id": {
				SemanticRole:     "pipeline_identifier",
				ValueSource:      "Pipeline ID returned by pipeline.list, pipeline.get, pipeline.latest, or merge_request.pipelines.",
				CommonConfusions: []string{"Do not use merge_request_iid; pipeline.wait requires pipeline_id."},
				ExampleBinding:   "MR !7 pipeline #123 => pipeline_id=123.",
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			{PropertyPath: "pipeline_id", Values: map[string]any{"description": "Pipeline ID returned by pipeline.list, pipeline.get, pipeline.latest, or merge_request.pipelines; do not use merge_request_iid."}},
		}
	}
	return options
}

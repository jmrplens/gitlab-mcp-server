package pipelines

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := pipelineOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineMutationSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, pipelineOptions(individualTool))
}

func pipelineUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := pipelineOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func pipelineOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "pipeline"},
		OpenWorld:      true,
		OwnerPackage:   "pipelines",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

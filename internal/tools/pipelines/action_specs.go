package pipelines

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionPipelineGet    = "pipeline.get"
	actionJobListProject = "job.list_project"
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
	return toolutil.NewReadActionSpec(name, route, pipelineOptions(name, individualTool))
}

func pipelineMutationSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, pipelineOptions(name, individualTool))
}

func pipelineUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, pipelineOptions(name, individualTool))
}

func pipelineDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, pipelineOptions(name, individualTool))
}

func pipelineOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute pipelines domain action.", Tags: []string{"ci", "pipeline"},
		OpenWorld:      true,
		OwnerPackage:   "pipelines",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "list":
		options.Usage = "List pipelines for one project. Use filters and pagination when the task asks for recent, failed, running, or branch-specific pipelines."
		options.Aliases = []string{"list pipelines", "show project pipelines", "find pipelines"}
		options.RelatedActions = []string{actionPipelineGet, "pipeline.latest", actionJobListProject}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or full path that owns the pipeline history.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"status": {
				SemanticRole:   "pipeline_status_filter",
				ValueSource:    "Pipeline status requested by the task, such as failed, running, or success.",
				ExampleBinding: `params.status:"failed"`,
			},
		}
		options.IndividualTool.Description = "List project pipelines with filters and pagination. Returns: pipeline IDs, refs, statuses, source, and timing metadata. See also: gitlab_pipeline_get, gitlab_pipeline_latest, gitlab_job_list_project."
	case "get":
		options.Usage = "Get one pipeline by project_id and pipeline_id. Use this when the target pipeline is already known and you need detailed status, ref, source, and web URL fields."
		options.Aliases = []string{"get pipeline", "show pipeline details", "lookup pipeline"}
		options.RelatedActions = []string{"pipeline.list", "pipeline.variables", actionJobListProject, "pipeline.cancel", "pipeline.retry"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"pipeline_id": {
				SemanticRole:     "pipeline_identifier",
				ValueSource:      "Numeric pipeline ID from pipeline.list, pipeline.latest, or merge-request pipeline context.",
				ExampleBinding:   "params.pipeline_id:12345",
				CommonConfusions: []string{"Use pipeline_id, not job_id or merge_request_iid."},
			},
		}
		options.IndividualTool.Description = "Get one pipeline by ID. Returns: full pipeline metadata, status lifecycle fields, and links. See also: gitlab_pipeline_list, gitlab_pipeline_variables, gitlab_job_list_project, gitlab_pipeline_retry."
	case "create":
		options.Usage = "Create a pipeline for a project ref (branch or tag). Use variables only when the task explicitly requires runtime overrides."
		options.Aliases = []string{"run pipeline", "trigger pipeline", "create pipeline"}
		options.RelatedActions = []string{actionPipelineGet, "pipeline.wait", actionJobListProject}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project where the pipeline should be created.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"ref": {
				SemanticRole:     "git_ref",
				ValueSource:      "Branch or tag to run the pipeline against.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"Use ref for branch/tag names; do not send commit SHA when branch intent is requested."},
			},
		}
		options.IndividualTool.Description = "Create a new pipeline. Returns: created pipeline metadata including ID, status, and target ref. See also: gitlab_pipeline_get, gitlab_pipeline_wait, gitlab_job_list_project."
	case "cancel":
		options.RelatedActions = []string{actionPipelineGet, "pipeline.retry", actionJobListProject}
	case "retry":
		options.RelatedActions = []string{actionPipelineGet, "pipeline.cancel", actionJobListProject}
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

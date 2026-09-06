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
	actionPipelineList   = "pipeline.list"
	actionPipelineRetry  = "pipeline.retry"
	actionPipelineCancel = "pipeline.cancel"
	actionJobListProject = "job.list_project"

	paramPipelineID = "pipeline_id"
)

// ActionSpecs returns canonical specs for CI/CD pipeline actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		pipelineReadSpec("list", toolutil.RouteAction(client, List), "gitlab_pipeline_list"),
		pipelineReadSpec("get", pipelineGetRoute(client), "gitlab_pipeline_get").
			WithEmbeddedResource("gitlab://project/{project_id}/pipeline/{pipeline_id}"),
		pipelineUpdateSpec("cancel", toolutil.RouteAction(client, Cancel), "gitlab_pipeline_cancel"),
		pipelineUpdateSpec("retry", toolutil.RouteAction(client, Retry), "gitlab_pipeline_retry"),
		pipelineDeleteSpec("delete", toolutil.RouteAction(client, DeleteOutput), "gitlab_pipeline_delete"),
		pipelineReadSpec("variables", toolutil.RouteAction(client, GetVariables), "gitlab_pipeline_variables"),
		pipelineReadSpec("test_report", toolutil.RouteAction(client, GetTestReport), "gitlab_pipeline_test_report"),
		pipelineReadSpec("test_report_summary", toolutil.RouteAction(client, GetTestReportSummary), "gitlab_pipeline_test_report_summary"),
		pipelineReadSpec("latest", toolutil.RouteAction(client, GetLatest), "gitlab_pipeline_latest"),
		pipelineMutationSpec("create", createRoute(client), "gitlab_pipeline_create"),
		pipelineUpdateSpec("update_metadata", toolutil.RouteAction(client, UpdateMetadata), "gitlab_pipeline_update_metadata"),
		pipelineReadSpec("wait", toolutil.RouteActionWithRequest(client, Wait), "gitlab_pipeline_wait"),
	}
}

// createRoute returns the create action route with its input schema
// constrained: inputs values are limited to the shapes
// toolutil.BuildPipelineInputs accepts instead of the open map the struct
// field alone would advertise.
func createRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Create)
	route.InputSchema = toolutil.PipelineInputsSchema[CreateInput]("inputs")
	return route
}

func pipelineGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				return pipelineNotFoundOutput{Identifier: fmt.Sprintf("ID %v in project %v", input["pipeline_id"], input["project_id"])}, nil
			}
			return result, err
		}
	})
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

// pipelineIDGuidanceEntry returns the shared parameter guidance for the
// project-scoped pipeline_id identifier used by the single-pipeline actions.
func pipelineIDGuidanceEntry() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "pipeline_identifier",
		ValueSource:      "Numeric pipeline ID from pipeline.list, pipeline.latest, or merge-request pipeline context.",
		ExampleBinding:   "params.pipeline_id:12345",
		CommonConfusions: []string{"Use pipeline_id, not job_id or merge_request_iid."},
	}
}

// pipelineIDGuidance returns a one-entry guidance map keyed by pipeline_id.
func pipelineIDGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{paramPipelineID: pipelineIDGuidanceEntry()}
}

// pipelineSourceValues are the pipeline sources a list or latest lookup can
// filter by: the CI_PIPELINE_SOURCE vocabulary GitLab documents at
// https://docs.gitlab.com/ci/jobs/job_rules/#ci_pipeline_source-predefined-variable
// (the page the pipelines API links its source parameter to), matching the
// gl.PipelineSource constants in client-go, in documentation order.
var pipelineSourceValues = []string{
	"api", "chat", "external", "external_pull_request_event", "merge_request_event",
	"ondemand_dast_scan", "ondemand_dast_validation", "parent_pipeline", "pipeline", "push",
	"schedule", "security_orchestration_policy", "trigger", "web", "webide",
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
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("status", "created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed", "canceled", "skipped", "manual", "scheduled"),
			toolutil.SchemaEnumOverride("scope", "running", "pending", "finished", "branches", "tags"),
			toolutil.SchemaEnumOverride("order_by", "id", "status", "ref", "updated_at", "user_id"),
			toolutil.SchemaEnumOverride("source", pipelineSourceValues...),
		}
	case "get":
		options.Usage = "Get one pipeline by project_id and pipeline_id. Use this when the target pipeline is already known and you need detailed status, ref, source, and web URL fields."
		options.Aliases = []string{"get pipeline", "show pipeline details", "lookup pipeline"}
		options.RelatedActions = []string{actionPipelineList, "pipeline.variables", actionJobListProject, "pipeline.cancel", "pipeline.retry"}
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
				CommonConfusions: []string{"Use ref for branch/tag names. Do not send commit SHA when branch intent is requested."},
			},
		}
		options.IndividualTool.Description = "Create a new pipeline. Returns: created pipeline metadata including ID, status, and target ref. See also: gitlab_pipeline_get, gitlab_pipeline_wait, gitlab_job_list_project."
		// The central variable_type enum covers only a top-level parameter;
		// each variables[] element carries its own copy (gl.VariableTypeValue).
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("variables.variable_type", "env_var", "file"),
		}
	case "cancel":
		options.Usage = "Cancel a running pipeline's jobs by project_id and pipeline_id. Use when a build must be stopped early. Completed pipelines cannot be cancelled."
		options.Aliases = []string{"cancel pipeline", "stop pipeline", "abort pipeline"}
		options.RelatedActions = []string{actionPipelineGet, actionPipelineRetry, actionJobListProject}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "Cancel a running pipeline. Returns: updated pipeline metadata reflecting the canceled status. See also: gitlab_pipeline_get, gitlab_pipeline_retry, gitlab_job_list_project."
	case "retry":
		options.Usage = "Retry the failed and canceled jobs of a pipeline by project_id and pipeline_id. Use after a pipeline fails to re-run only the non-successful jobs."
		options.Aliases = []string{"retry pipeline", "rerun pipeline", "retry failed jobs"}
		options.RelatedActions = []string{actionPipelineGet, actionPipelineCancel, actionJobListProject}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "Retry failed and canceled jobs in a pipeline. Returns: updated pipeline metadata after re-queuing jobs. See also: gitlab_pipeline_get, gitlab_pipeline_cancel, gitlab_job_list_project."
	case "delete":
		options.Usage = "Permanently delete a pipeline and all its jobs by project_id and pipeline_id. Use only when the pipeline record must be removed. This cannot be undone and requires Owner role."
		options.Aliases = []string{"delete pipeline", "remove pipeline", "destroy pipeline"}
		options.RelatedActions = []string{actionPipelineGet, actionPipelineList, actionPipelineCancel}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "Delete a pipeline and all its jobs. Returns: a success status and confirmation message. See also: gitlab_pipeline_get, gitlab_pipeline_list, gitlab_pipeline_cancel."
	case "variables":
		options.Usage = "List the variables used to run a pipeline by project_id and pipeline_id. Use to inspect the runtime variable overrides a pipeline executed with. Requires Maintainer+ role."
		options.Aliases = []string{"pipeline variables", "show pipeline variables", "get pipeline variables"}
		options.RelatedActions = []string{actionPipelineGet, "pipeline.create", actionJobListProject}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "List a pipeline's runtime variables. Returns: variable keys, values, and types. See also: gitlab_pipeline_get, gitlab_pipeline_create, gitlab_job_list_project."
	case "test_report":
		options.Usage = "Get the full test report for a pipeline by project_id and pipeline_id. Use when the task needs per-suite test counts and timing. Requires jobs that upload JUnit artifacts."
		options.Aliases = []string{"pipeline test report", "show test report", "get test results"}
		options.RelatedActions = []string{"pipeline.test_report_summary", actionPipelineGet, actionJobListProject}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "Get a pipeline's full test report. Returns: total/passed/failed/skipped counts and per-suite breakdown. See also: gitlab_pipeline_test_report_summary, gitlab_pipeline_get, gitlab_job_list_project."
	case "test_report_summary":
		options.Usage = "Get the summarized test report for a pipeline by project_id and pipeline_id. Use for aggregate pass/fail totals without the full per-test detail."
		options.Aliases = []string{"pipeline test report summary", "test summary", "get test report summary"}
		options.RelatedActions = []string{"pipeline.test_report", actionPipelineGet, actionJobListProject}
		options.ParameterGuidance = pipelineIDGuidance()
		options.IndividualTool.Description = "Get a pipeline's test report summary. Returns: aggregate test totals and per-suite summaries with build IDs. See also: gitlab_pipeline_test_report, gitlab_pipeline_get, gitlab_job_list_project."
	case "latest":
		options.Usage = "Get the most recent pipeline for a project, optionally filtered by ref. Use when the exact pipeline_id is unknown and you need the latest run for a branch or tag."
		options.Aliases = []string{"latest pipeline", "most recent pipeline", "get latest pipeline"}
		options.RelatedActions = []string{actionPipelineGet, actionPipelineList, actionJobListProject}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project whose latest pipeline is requested.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"ref": {
				SemanticRole:   "git_ref",
				ValueSource:    "Branch or tag to scope the latest pipeline to. Defaults to the project default branch.",
				ExampleBinding: `params.ref:"main"`,
			},
		}
		options.IndividualTool.Description = "Get the latest pipeline for a ref. Returns: full pipeline metadata for the most recent run. See also: gitlab_pipeline_get, gitlab_pipeline_list, gitlab_job_list_project."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("status", "created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed", "canceled", "skipped", "manual", "scheduled"),
			toolutil.SchemaEnumOverride("scope", "running", "pending", "finished", "branches", "tags"),
			toolutil.SchemaEnumOverride("order_by", "id", "status", "ref", "updated_at", "user_id"),
			toolutil.SchemaEnumOverride("source", pipelineSourceValues...),
		}
	case "update_metadata":
		options.Usage = "Update a pipeline's metadata (its display name) by project_id and pipeline_id. Use to rename a pipeline. Requires Developer+ role and a non-archived pipeline."
		options.Aliases = []string{"update pipeline metadata", "rename pipeline", "set pipeline name"}
		options.RelatedActions = []string{actionPipelineGet, actionPipelineList, "pipeline.create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramPipelineID: pipelineIDGuidanceEntry(),
			"name": {
				SemanticRole:   "display_name",
				ValueSource:    "New human-readable pipeline name to set.",
				ExampleBinding: `params.name:"nightly-release"`,
			},
		}
		options.IndividualTool.Description = "Update a pipeline's name. Returns: updated pipeline metadata reflecting the new name. See also: gitlab_pipeline_get, gitlab_pipeline_list, gitlab_pipeline_create."
	}
	if actionName == "wait" {
		options.Usage = "Use only to poll an existing pipeline_id until a terminal status. For merge when pipeline succeeds, use merge_request.merge with auto_merge=true instead."
		options.Aliases = []string{"wait for pipeline", "poll pipeline status", "wait pipeline completion"}
		options.RelatedActions = []string{"pipeline.get", actionPipelineList, "merge_request.pipelines", "merge_request.merge"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"pipeline_id": {
				SemanticRole:     "pipeline_identifier",
				ValueSource:      "Pipeline ID returned by pipeline.list, pipeline.get, pipeline.latest, or merge_request.pipelines.",
				CommonConfusions: []string{"Do not use merge_request_iid. Pipeline.wait requires pipeline_id."},
				ExampleBinding:   "MR !7 pipeline #123 => pipeline_id=123.",
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			{PropertyPath: "pipeline_id", Values: map[string]any{"description": "Pipeline ID returned by pipeline.list, pipeline.get, pipeline.latest, or merge_request.pipelines. Do not use merge_request_iid."}},
		}
	}
	return options
}

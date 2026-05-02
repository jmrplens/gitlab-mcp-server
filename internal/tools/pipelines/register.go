package pipelines

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all pipeline MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_list",
		Title:       toolutil.TitleFromName("gitlab_pipeline_list"),
		Description: "List pipelines for a GitLab project. Supports filtering by status (success, failed, running, pending, canceled), scope (running, pending, finished, branches, tags), source (push, web, schedule, merge_request_event), ref (branch/tag), SHA, and username. Returns pipeline ID, status, source, ref, web URL, and timestamps with pagination.\n\nReturns: paginated list of pipelines with id, status, source, ref, sha, web_url, and timestamps. See also: gitlab_pipeline_get, gitlab_job_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_get",
		Title:       toolutil.TitleFromName("gitlab_pipeline_get"),
		Description: "Retrieve detailed information about a specific pipeline in a GitLab project. Returns pipeline ID, status, source, ref, SHA, duration, coverage, user, timestamps, and YAML errors. See also: gitlab_job_list, gitlab_pipeline_test_report.\n\nReturns: id, iid, status, source, ref, sha, duration, coverage, user, timestamps, and yaml_errors.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_get", start, nil)
			return toolutil.NotFoundResult("Pipeline", fmt.Sprintf("ID %d in project %s", input.PipelineID, input.ProjectID),
				"Use gitlab_pipeline_list with project_id to list pipelines",
				"Verify the pipeline_id is correct for this project",
			), DetailOutput{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_get", start, err)
		result := toolutil.ToolResultAnnotated(FormatDetailMarkdown(out), toolutil.ContentDetail)
		if err == nil && out.ProjectID > 0 && out.ID > 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%d/pipeline/%d", out.ProjectID, out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_cancel",
		Title:       toolutil.TitleFromName("gitlab_pipeline_cancel"),
		Description: "Cancel a running pipeline in a GitLab project. Returns the updated pipeline details with canceled status.\n\nReturns: pipeline id, iid, status, ref, sha, web_url, and timestamps. See also: gitlab_pipeline_retry, gitlab_pipeline_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := Cancel(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_cancel", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatDetailMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_retry",
		Title:       toolutil.TitleFromName("gitlab_pipeline_retry"),
		Description: "Retry only the failed jobs in a pipeline (successful jobs are not re-run). Returns the updated pipeline details. To retry a specific job, use gitlab_job_retry instead.\n\nReturns: pipeline id, iid, status, ref, sha, web_url, and timestamps. See also: gitlab_pipeline_get, gitlab_job_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := Retry(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_retry", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatDetailMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_delete",
		Title:       toolutil.TitleFromName("gitlab_pipeline_delete"),
		Description: "Permanently delete a pipeline and all its jobs. This action cannot be undone. Requires at least Maintainer access level.\n\nReturns: confirmation message. See also: gitlab_pipeline_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Permanently delete pipeline %d in project %q? This action cannot be undone.", input.PipelineID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("pipeline %d from project %s", input.PipelineID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_variables",
		Title:       toolutil.TitleFromName("gitlab_pipeline_variables"),
		Description: "Get the variables for a specific pipeline. Returns variable keys, values, and types.\n\nReturns: list of variables with key, value, and variable_type. See also: gitlab_pipeline_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, VariablesOutput, error) {
		start := time.Now()
		out, err := GetVariables(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_variables", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatVariablesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_test_report",
		Title:       toolutil.TitleFromName("gitlab_pipeline_test_report"),
		Description: "Get the full test report for a pipeline. Returns total/passed/failed/skipped/error counts and per-suite breakdowns.\n\nReturns: total, success, failed, skipped, error counts and per-suite test case details. See also: gitlab_pipeline_get, gitlab_job_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TestReportOutput, error) {
		start := time.Now()
		out, err := GetTestReport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_test_report", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTestReportMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_test_report_summary",
		Title:       toolutil.TitleFromName("gitlab_pipeline_test_report_summary"),
		Description: "Get a summary of the test report for a pipeline. Returns aggregated counts and per-suite summaries with build IDs.\n\nReturns: aggregated test counts and per-suite summaries with build IDs. See also: gitlab_pipeline_test_report.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TestReportSummaryOutput, error) {
		start := time.Now()
		out, err := GetTestReportSummary(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_test_report_summary", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTestReportSummaryMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_latest",
		Title:       toolutil.TitleFromName("gitlab_pipeline_latest"),
		Description: "Get the latest pipeline for a project, optionally filtered by branch/tag ref. Returns full pipeline details.\n\nReturns: id, iid, status, source, ref, sha, duration, web_url, and timestamps. See also: gitlab_pipeline_get, gitlab_job_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetLatestInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := GetLatest(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_latest", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatDetailMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_create",
		Title:       toolutil.TitleFromName("gitlab_pipeline_create"),
		Description: "Create a new pipeline for a branch or tag. Optionally pass variables (key/value pairs with type env_var or file). Returns: id, iid, status, source, ref, sha, web_url, duration, coverage, timestamps. See also: gitlab_pipeline_get, gitlab_job_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_create", start, err)
		result := toolutil.ToolResultAnnotated(FormatDetailMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_update_metadata",
		Title:       toolutil.TitleFromName("gitlab_pipeline_update_metadata"),
		Description: "Update the display name of an existing pipeline. This is the only field that can be changed \u2014 status, ref, variables, and other fields cannot be updated. Returns: id, iid, status, source, ref, sha, web_url, duration. See also: gitlab_pipeline_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateMetadataInput) (*mcp.CallToolResult, DetailOutput, error) {
		start := time.Now()
		out, err := UpdateMetadata(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_update_metadata", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatDetailMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_wait",
		Title:       toolutil.TitleFromName("gitlab_pipeline_wait"),
		Description: "Wait for a pipeline to reach a terminal state (success, failed, canceled, skipped, manual). Polls the pipeline status at a configurable interval and sends progress notifications. Returns the final pipeline details when done or when the timeout is reached.\n\nReturns: pipeline details, waited_for duration, poll_count, final_status, timed_out flag. See also: gitlab_pipeline_get, gitlab_pipeline_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WaitInput) (*mcp.CallToolResult, WaitOutput, error) {
		start := time.Now()
		out, err := Wait(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_wait", start, err)
		if out.TimedOut {
			result := toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail)
			result.IsError = true
			return result, out, nil
		}
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail), out, err)
	})
}

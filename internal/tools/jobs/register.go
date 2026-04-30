// register.go wires GitLab CI/CD job tools into the MCP server.
package jobs

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all CI/CD job MCP tools with the server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_list",
		Title:       toolutil.TitleFromName("gitlab_job_list"),
		Description: "List jobs for a specific pipeline in a GitLab project. Supports filtering by scope (created, pending, running, failed, success, canceled, skipped, manual). Returns job ID, name, status, stage, runner, duration, and web URL with pagination.\n\nReturns: JSON array of jobs with pagination. See also: gitlab_job_get, gitlab_pipeline_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_get",
		Title:       toolutil.TitleFromName("gitlab_job_get"),
		Description: "Retrieve detailed information about a specific CI/CD job in a GitLab project. Returns job ID, name, status, stage, pipeline, runner, duration, coverage, timestamps, and web URL.\n\nReturns: JSON with job details. See also: gitlab_job_cancel, gitlab_job_trace.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		if err == nil && out.ID != 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/job/%d", url.PathEscape(string(input.ProjectID)), out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_trace",
		Title:       toolutil.TitleFromName("gitlab_job_trace"),
		Description: "Retrieve the log (trace) output of a CI/CD job. Returns the raw log text, truncated to the last 100KB if larger. There is no pagination \u2014 if the log is truncated, the beginning is lost. Useful for debugging failed jobs by examining the most recent output.\n\nReturns: job trace log content. See also: gitlab_job_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TraceInput) (*mcp.CallToolResult, TraceOutput, error) {
		start := time.Now()
		out, err := Trace(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_trace", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTraceMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_cancel",
		Title:       toolutil.TitleFromName("gitlab_job_cancel"),
		Description: "Cancel a running or pending CI/CD job in a GitLab project. Returns the updated job details.\n\nReturns: JSON with the updated job details. See also: gitlab_job_retry, gitlab_job_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Cancel(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_cancel", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_retry",
		Title:       toolutil.TitleFromName("gitlab_job_retry"),
		Description: "Retry a failed or canceled CI/CD job in a GitLab project. Returns the new job details.\n\nReturns: JSON with the updated job details. See also: gitlab_job_get, gitlab_pipeline_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Retry(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_retry", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_list_project",
		Title:       toolutil.TitleFromName("gitlab_job_list_project"),
		Description: "List all jobs across a GitLab project (not limited to a single pipeline). Supports filtering by scope and pagination. Returns job ID, name, status, stage, duration.\n\nReturns: JSON array of jobs with pagination.\n\nSee also: gitlab_job_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_list_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_list_bridges",
		Title:       toolutil.TitleFromName("gitlab_job_list_bridges"),
		Description: "List pipeline bridge (trigger) jobs for a pipeline. Bridge jobs connect upstream and downstream pipelines. Returns bridge ID, name, stage, status, duration, and downstream pipeline ID.\n\nReturns: JSON array of bridge jobs with pagination. See also: gitlab_job_list, gitlab_pipeline_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BridgeListInput) (*mcp.CallToolResult, BridgeListOutput, error) {
		start := time.Now()
		out, err := ListBridges(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_list_bridges", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBridgeListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_artifacts",
		Title:       toolutil.TitleFromName("gitlab_job_artifacts"),
		Description: "Download the artifacts archive (zip) for a specific job. Returns base64-encoded content (limited to 1MB). Use for retrieving build outputs.\n\nReturns: base64-encoded artifact archive content.\n\nSee also: gitlab_job_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, ArtifactsOutput, error) {
		start := time.Now()
		out, err := GetArtifacts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_artifacts", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatArtifactsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_download_artifacts",
		Title:       toolutil.TitleFromName("gitlab_job_download_artifacts"),
		Description: "Download the artifacts archive for a specific ref and optional job name. Returns base64-encoded content (limited to 1MB).\n\nReturns: base64-encoded artifact archive content. See also: gitlab_job_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadArtifactsInput) (*mcp.CallToolResult, ArtifactsOutput, error) {
		start := time.Now()
		out, err := DownloadArtifacts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_download_artifacts", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatArtifactsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_download_single_artifact",
		Title:       toolutil.TitleFromName("gitlab_job_download_single_artifact"),
		Description: "Download a single artifact file from a job by its path within the archive. Returns raw file content. Useful for reading specific build outputs like test results or coverage reports.\n\nReturns: raw artifact file content. See also: gitlab_job_download_artifacts.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SingleArtifactInput) (*mcp.CallToolResult, SingleArtifactOutput, error) {
		start := time.Now()
		out, err := DownloadSingleArtifact(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_download_single_artifact", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSingleArtifactMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_download_single_artifact_by_ref",
		Title:       toolutil.TitleFromName("gitlab_job_download_single_artifact_by_ref"),
		Description: "Download a single artifact file by branch/tag name and artifact path. Returns raw file content from the latest successful pipeline for that ref.\n\nReturns: raw artifact file content.\n\nSee also: gitlab_job_artifacts.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SingleArtifactRefInput) (*mcp.CallToolResult, SingleArtifactOutput, error) {
		start := time.Now()
		out, err := DownloadSingleArtifactByRef(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_download_single_artifact_by_ref", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSingleArtifactMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_erase",
		Title:       toolutil.TitleFromName("gitlab_job_erase"),
		Description: "Erase a job's trace log and artifacts. Returns the updated job details with erased_at timestamp.\n\nReturns: JSON with the updated job details. See also: gitlab_job_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, Output, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Erase job %d trace and artifacts in project %q?", input.JobID, input.ProjectID)); r != nil {
			return r, Output{}, nil
		}
		start := time.Now()
		out, err := Erase(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_erase", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_keep_artifacts",
		Title:       toolutil.TitleFromName("gitlab_job_keep_artifacts"),
		Description: "Prevent a job's artifacts from being deleted when expiration is set. Returns updated job details.\n\nReturns: JSON with the updated job details.\n\nSee also: gitlab_job_artifacts.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ActionInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := KeepArtifacts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_keep_artifacts", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_play",
		Title:       toolutil.TitleFromName("gitlab_job_play"),
		Description: "Trigger (play) a manual CI/CD job. Supports passing job variables. Returns updated job details.\n\nReturns: JSON with the updated job details. See also: gitlab_job_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PlayInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Play(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_play", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_delete_artifacts",
		Title:       toolutil.TitleFromName("gitlab_job_delete_artifacts"),
		Description: "Delete the artifacts for a specific job.\n\nReturns: confirmation message.\n\nSee also: gitlab_job_artifacts.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteArtifactsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete artifacts for job %d in project %q?", input.JobID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteArtifacts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_delete_artifacts", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("job artifacts")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_delete_project_artifacts",
		Title:       toolutil.TitleFromName("gitlab_job_delete_project_artifacts"),
		Description: "Delete all artifacts across an entire project. This is a destructive operation.\n\nReturns: confirmation message.\n\nSee also: gitlab_job_list_project.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectArtifactsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete ALL artifacts in project %q? This affects the entire project.", input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteProjectArtifacts(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_delete_project_artifacts", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project artifacts")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_job_wait",
		Title:       toolutil.TitleFromName("gitlab_job_wait"),
		Description: "Wait for a CI/CD job to reach a terminal state (success, failed, canceled, skipped, manual). Polls the job status at a configurable interval and sends progress notifications. Returns the final job details when done or when the timeout is reached.\n\nReturns: job details, waited_for duration, poll_count, final_status, timed_out flag. See also: gitlab_job_get, gitlab_job_trace.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WaitInput) (*mcp.CallToolResult, WaitOutput, error) {
		start := time.Now()
		out, err := Wait(ctx, req, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_job_wait", start, err)
		if out.TimedOut {
			result := toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail)
			result.IsError = true
			return result, out, nil
		}
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatWaitMarkdown(out), toolutil.ContentDetail), out, err)
	})
}

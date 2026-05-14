package pipelineschedules

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the six pipeline schedule management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	scheduleTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSchedule})
	}

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_list", "List pipeline schedules for a GitLab project. Supports filtering by scope (active, inactive). Returns paginated results with schedule details.\n\nReturns: paginated list of schedules with id, description, ref, cron, active state, and next_run_at. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_get", "Get details of a specific pipeline schedule in a GitLab project by its ID. Returns description, ref, cron expression, timezone, active state, owner, and timestamps.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, owner, and timestamps. See also: gitlab_pipeline_schedule_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_create", "Create a new pipeline schedule in a GitLab project. Requires description, ref (branch/tag), and cron expression. Optionally set timezone and active state. Returns: id, description, ref, cron, cron_timezone, next_run_at, active, owner_name. See also: gitlab_pipeline_schedule_get."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_update", "Update an existing pipeline schedule in a GitLab project. All fields are optional: description, ref, cron, timezone, active state. Returns: id, description, ref, cron, cron_timezone, next_run_at, active, owner_name. See also: gitlab_pipeline_schedule_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_delete", "Permanently delete a pipeline schedule from a GitLab project. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_pipeline_schedule_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete pipeline schedule %d from project %q?", input.ScheduleID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("pipeline schedule")
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_run", "Trigger an immediate run of a pipeline schedule. Executes the schedule now regardless of its cron timing. Returns the updated schedule details.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, and owner. See also: gitlab_pipeline_list."), func(ctx context.Context, req *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Run(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_run", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_take_ownership", "Take ownership of a pipeline schedule, making the current user the owner. Returns the updated schedule details.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, and owner. See also: gitlab_pipeline_schedule_list."), func(ctx context.Context, req *mcp.CallToolRequest, input TakeOwnershipInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := TakeOwnership(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_take_ownership", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_create_variable", "Create a new variable for a pipeline schedule. Variables are passed to pipelines triggered by the schedule. Supports env_var (default) and file types. Returns: key, value, variable_type. See also: gitlab_pipeline_schedule_get."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateVariableInput) (*mcp.CallToolResult, VariableOutput, error) {
		start := time.Now()
		out, err := CreateVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_create_variable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatVariableMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_edit_variable", "Edit an existing pipeline schedule variable by key. Updates the value and optionally the variable type. Returns: key, value, variable_type. See also: gitlab_pipeline_schedule_get."), func(ctx context.Context, req *mcp.CallToolRequest, input EditVariableInput) (*mcp.CallToolResult, VariableOutput, error) {
		start := time.Now()
		out, err := EditVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_edit_variable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatVariableMarkdown(out)), out, err)
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_delete_variable", "Delete a pipeline schedule variable by key. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_pipeline_schedule_get."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteVariableInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete variable %q from schedule %d in project %q?", input.Key, input.ScheduleID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_delete_variable", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("pipeline schedule variable %q", input.Key))
	})

	mcp.AddTool(server, scheduleTool("gitlab_pipeline_schedule_list_triggered_pipelines", "List all pipelines that were triggered by a specific pipeline schedule. Returns paginated results with pipeline ID, ref, status, and source.\n\nReturns: paginated list of pipelines with id, ref, status, and source. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListTriggeredPipelinesInput) (*mcp.CallToolResult, TriggeredPipelinesListOutput, error) {
		start := time.Now()
		out, err := ListTriggeredPipelines(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_list_triggered_pipelines", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggeredPipelinesMarkdown(out)), out, err)
	})
}

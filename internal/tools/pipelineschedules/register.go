// register.go wires pipelineschedules MCP tools to the MCP server.

package pipelineschedules

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the six pipeline schedule management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_list",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_list"),
		Description: "List pipeline schedules for a GitLab project. Supports filtering by scope (active, inactive). Returns paginated results with schedule details.\n\nReturns: paginated list of schedules with id, description, ref, cron, active state, and next_run_at. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_get",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_get"),
		Description: "Get details of a specific pipeline schedule in a GitLab project by its ID. Returns description, ref, cron expression, timezone, active state, owner, and timestamps.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, owner, and timestamps. See also: gitlab_pipeline_schedule_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_create",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_create"),
		Description: "Create a new pipeline schedule in a GitLab project. Requires description, ref (branch/tag), and cron expression. Optionally set timezone and active state. Returns: id, description, ref, cron, cron_timezone, next_run_at, active, owner_name. See also: gitlab_pipeline_schedule_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_update",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_update"),
		Description: "Update an existing pipeline schedule in a GitLab project. All fields are optional: description, ref, cron, timezone, active state. Returns: id, description, ref, cron, cron_timezone, next_run_at, active, owner_name. See also: gitlab_pipeline_schedule_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_delete",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_delete"),
		Description: "Permanently delete a pipeline schedule from a GitLab project. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_pipeline_schedule_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_run",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_run"),
		Description: "Trigger an immediate run of a pipeline schedule. Executes the schedule now regardless of its cron timing. Returns the updated schedule details.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, and owner. See also: gitlab_pipeline_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Run(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_run", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_take_ownership",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_take_ownership"),
		Description: "Take ownership of a pipeline schedule, making the current user the owner. Returns the updated schedule details.\n\nReturns: id, description, ref, cron, cron_timezone, active, next_run_at, and owner. See also: gitlab_pipeline_schedule_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TakeOwnershipInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := TakeOwnership(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_take_ownership", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_create_variable",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_create_variable"),
		Description: "Create a new variable for a pipeline schedule. Variables are passed to pipelines triggered by the schedule. Supports env_var (default) and file types. Returns: key, value, variable_type. See also: gitlab_pipeline_schedule_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateVariableInput) (*mcp.CallToolResult, VariableOutput, error) {
		start := time.Now()
		out, err := CreateVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_create_variable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatVariableMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_edit_variable",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_edit_variable"),
		Description: "Edit an existing pipeline schedule variable by key. Updates the value and optionally the variable type. Returns: key, value, variable_type. See also: gitlab_pipeline_schedule_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditVariableInput) (*mcp.CallToolResult, VariableOutput, error) {
		start := time.Now()
		out, err := EditVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_edit_variable", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatVariableMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_delete_variable",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_delete_variable"),
		Description: "Delete a pipeline schedule variable by key. This action cannot be undone.\n\nReturns: confirmation message. See also: gitlab_pipeline_schedule_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteVariableInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_schedule_list_triggered_pipelines",
		Title:       toolutil.TitleFromName("gitlab_pipeline_schedule_list_triggered_pipelines"),
		Description: "List all pipelines that were triggered by a specific pipeline schedule. Returns paginated results with pipeline ID, ref, status, and source.\n\nReturns: paginated list of pipelines with id, ref, status, and source. See also: gitlab_pipeline_schedule_get, gitlab_pipeline_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSchedule,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListTriggeredPipelinesInput) (*mcp.CallToolResult, TriggeredPipelinesListOutput, error) {
		start := time.Now()
		out, err := ListTriggeredPipelines(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_schedule_list_triggered_pipelines", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggeredPipelinesMarkdown(out)), out, err)
	})
}

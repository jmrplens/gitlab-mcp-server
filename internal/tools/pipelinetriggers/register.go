package pipelinetriggers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all pipeline trigger individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	triggerTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconPipeline})
	}

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_list", "List pipeline trigger tokens for a project\n\nReturns: JSON array of pipeline triggers with pagination.\n\nSee also: gitlab_pipeline_trigger_create, gitlab_pipeline_trigger_run"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListTriggers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListTriggersMarkdown(out)), out, err)
	})

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_get", "Get a single pipeline trigger token\n\nReturns: JSON with trigger token details.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_update"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_create", "Create a new pipeline trigger token\n\nReturns: JSON with the created trigger token.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_run"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_update", "Update a pipeline trigger token description\n\nReturns: JSON with the updated trigger token.\n\nSee also: gitlab_pipeline_trigger_get, gitlab_pipeline_trigger_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_delete", "Delete a pipeline trigger token\n\nReturns: JSON confirming trigger deletion.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_create"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete pipeline trigger %d in project %q?", input.TriggerID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("pipeline trigger")
	})

	mcp.AddTool(server, triggerTool("gitlab_pipeline_trigger_run", "Trigger a pipeline using a trigger token\n\nReturns: JSON with the triggered pipeline details.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_create"), func(ctx context.Context, req *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, RunOutput, error) {
		start := time.Now()
		out, err := RunTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_run", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRunOutputMarkdown(out)), out, err)
	})
}

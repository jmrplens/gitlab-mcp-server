// register.go wires pipelinetriggers MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_list",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_list"),
		Description: "List pipeline trigger tokens for a project\n\nReturns: JSON array of pipeline triggers with pagination.\n\nSee also: gitlab_pipeline_trigger_create, gitlab_pipeline_trigger_run",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListTriggers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListTriggersMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_get",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_get"),
		Description: "Get a single pipeline trigger token\n\nReturns: JSON with trigger token details.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_update",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_create",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_create"),
		Description: "Create a new pipeline trigger token\n\nReturns: JSON with the created trigger token.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_run",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_update",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_update"),
		Description: "Update a pipeline trigger token description\n\nReturns: JSON with the updated trigger token.\n\nSee also: gitlab_pipeline_trigger_get, gitlab_pipeline_trigger_delete",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTriggerMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_delete",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_delete"),
		Description: "Delete a pipeline trigger token\n\nReturns: JSON confirming trigger deletion.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_pipeline_trigger_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_pipeline_trigger_run",
		Title:       toolutil.TitleFromName("gitlab_pipeline_trigger_run"),
		Description: "Trigger a pipeline using a trigger token\n\nReturns: JSON with the triggered pipeline details.\n\nSee also: gitlab_pipeline_trigger_list, gitlab_create_pipeline",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, RunOutput, error) {
		start := time.Now()
		out, err := RunTrigger(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_pipeline_trigger_run", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRunOutputMarkdown(out)), out, err)
	})
}

// RegisterMeta registers the pipeline trigger meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":   toolutil.RouteAction(client, ListTriggers),
		"get":    toolutil.RouteAction(client, GetTrigger),
		"create": toolutil.RouteAction(client, CreateTrigger),
		"update": toolutil.RouteAction(client, UpdateTrigger),
		"delete": toolutil.DestructiveVoidAction(client, DeleteTrigger),
		"run":    toolutil.RouteAction(client, RunTrigger),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_pipeline_trigger",
		Title: toolutil.TitleFromName("gitlab_pipeline_trigger"),
		Description: `Pipeline trigger token operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List trigger tokens (project_id, page, per_page)
- get: Get a trigger token (project_id, trigger_id)
- create: Create a trigger token (project_id, description)
- update: Update a trigger token (project_id, trigger_id, description)
- delete: Delete a trigger token (project_id, trigger_id)
- run: Trigger a pipeline (project_id, ref, token, variables)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconPipeline,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_pipeline_trigger", routes, nil))
}

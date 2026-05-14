package instancevariables

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five instance CI/CD variable management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	instanceVariableTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconVariable})
	}

	mcp.AddTool(server, instanceVariableTool("gitlab_instance_variable_list", "List CI/CD variables at the GitLab instance level. Returns paginated results with variable key, type, protection, and masking.\n\nReturns: JSON with array of instance-level CI/CD variables and pagination info. See also: gitlab_instance_variable_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, instanceVariableTool("gitlab_instance_variable_get", "Get a specific CI/CD variable by key from the GitLab instance level.\n\nReturns: JSON with variable details (key, value, type, protection, masking). See also: gitlab_instance_variable_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, instanceVariableTool("gitlab_instance_variable_create", "Create a new CI/CD variable at the GitLab instance level. Requires key and value. Optionally set type (env_var/file), protection, and masking.\n\nReturns: JSON with the created variable details. See also: gitlab_instance_variable_list."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, instanceVariableTool("gitlab_instance_variable_update", "Update an existing CI/CD variable at the GitLab instance level. Specify the key to update and any fields to change.\n\nReturns: JSON with the updated variable details. See also: gitlab_instance_variable_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, instanceVariableTool("gitlab_instance_variable_delete", "Delete a CI/CD variable from the GitLab instance level by key. This action cannot be undone.\n\nReturns: JSON confirmation of deletion. See also: gitlab_instance_variable_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete instance CI/CD variable %q?", input.Key)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_instance_variable_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("instance CI/CD variable")
	})
}

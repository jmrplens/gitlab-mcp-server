package terraformstates

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Terraform state tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	terraformStateTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconInfra})
	}

	mcp.AddTool(server, terraformStateTool("gitlab_list_terraform_states", "List Terraform states for a GitLab project\n\nReturns: JSON array of Terraform states with pagination.\n\nSee also: gitlab_get_terraform_state, gitlab_list_secure_files"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_terraform_states", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, terraformStateTool("gitlab_get_terraform_state", "Get details of a Terraform state\n\nReturns: JSON with Terraform state details.\n\nSee also: gitlab_list_terraform_states, gitlab_lock_terraform_state"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, StateItem, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, terraformStateTool("gitlab_delete_terraform_state", "Delete a Terraform state\n\nReturns: JSON confirming state deletion.\n\nSee also: gitlab_list_terraform_states, gitlab_get_terraform_state"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete Terraform state %q from project %s?", input.Name, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_terraform_state", start, err)
		r, o, _ := toolutil.DeleteResult("terraform state")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, terraformStateTool("gitlab_delete_terraform_state_version", "Delete a specific version of a Terraform state\n\nReturns: JSON confirming state version deletion.\n\nSee also: gitlab_get_terraform_state, gitlab_delete_terraform_state"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteVersionInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete Terraform state %q version %d from project %s?", input.Name, input.Serial, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteVersion(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_terraform_state_version", start, err)
		r, o, _ := toolutil.DeleteResult("terraform state version")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, terraformStateTool("gitlab_lock_terraform_state", "Lock a Terraform state\n\nReturns: JSON confirming the state was locked.\n\nSee also: gitlab_unlock_terraform_state, gitlab_get_terraform_state"), func(ctx context.Context, req *mcp.CallToolRequest, input LockInput) (*mcp.CallToolResult, LockOutput, error) {
		start := time.Now()
		out, err := Lock(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_lock_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLockMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, terraformStateTool("gitlab_unlock_terraform_state", "Unlock a Terraform state\n\nReturns: JSON confirming the state was unlocked.\n\nSee also: gitlab_lock_terraform_state, gitlab_get_terraform_state"), func(ctx context.Context, req *mcp.CallToolRequest, input LockInput) (*mcp.CallToolResult, LockOutput, error) {
		start := time.Now()
		out, err := Unlock(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_unlock_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLockMarkdown(out)), out, nil)
	})
}

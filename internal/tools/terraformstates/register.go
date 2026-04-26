// register.go wires terraformstates MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_terraform_states",
		Title:       toolutil.TitleFromName("gitlab_list_terraform_states"),
		Description: "List Terraform states for a GitLab project\n\nReturns: JSON array of Terraform states with pagination.\n\nSee also: gitlab_get_terraform_state, gitlab_list_secure_files",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_terraform_states", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_terraform_state",
		Title:       toolutil.TitleFromName("gitlab_get_terraform_state"),
		Description: "Get details of a Terraform state\n\nReturns: JSON with Terraform state details.\n\nSee also: gitlab_list_terraform_states, gitlab_lock_terraform_state",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, StateItem, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStateMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_terraform_state",
		Title:       toolutil.TitleFromName("gitlab_delete_terraform_state"),
		Description: "Delete a Terraform state\n\nReturns: JSON confirming state deletion.\n\nSee also: gitlab_list_terraform_states, gitlab_get_terraform_state",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_terraform_state_version",
		Title:       toolutil.TitleFromName("gitlab_delete_terraform_state_version"),
		Description: "Delete a specific version of a Terraform state\n\nReturns: JSON confirming state version deletion.\n\nSee also: gitlab_get_terraform_state, gitlab_delete_terraform_state",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteVersionInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_lock_terraform_state",
		Title:       toolutil.TitleFromName("gitlab_lock_terraform_state"),
		Description: "Lock a Terraform state\n\nReturns: JSON confirming the state was locked.\n\nSee also: gitlab_unlock_terraform_state, gitlab_get_terraform_state",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input LockInput) (*mcp.CallToolResult, LockOutput, error) {
		start := time.Now()
		out, err := Lock(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_lock_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLockMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_unlock_terraform_state",
		Title:       toolutil.TitleFromName("gitlab_unlock_terraform_state"),
		Description: "Unlock a Terraform state\n\nReturns: JSON confirming the state was unlocked.\n\nSee also: gitlab_lock_terraform_state, gitlab_get_terraform_state",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconInfra,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input LockInput) (*mcp.CallToolResult, LockOutput, error) {
		start := time.Now()
		out, err := Unlock(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_unlock_terraform_state", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLockMarkdown(out)), out, nil)
	})
}

// RegisterMeta registers the gitlab_terraform_state meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"list":           toolutil.RouteAction(client, List),
		"get":            toolutil.RouteAction(client, Get),
		"delete":         toolutil.DestructiveVoidAction(client, Delete),
		"delete_version": toolutil.DestructiveVoidAction(client, DeleteVersion),
		"lock":           toolutil.RouteAction(client, Lock),
		"unlock":         toolutil.RouteAction(client, Unlock),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_terraform_state",
		Title: toolutil.TitleFromName("gitlab_terraform_state"),
		Description: `Manage Terraform states in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List Terraform states. Params: project_path (required, full path e.g. group/project)
- get: Get a Terraform state by name. Params: project_path (required), name (required)
- delete: Delete a Terraform state. Params: project_id (required), name (required)
- delete_version: Delete a specific Terraform state version. Params: project_id (required), name (required), serial (required, int)
- lock: Lock a Terraform state. Params: project_id (required), name (required)
- unlock: Unlock a Terraform state. Params: project_id (required), name (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconInfra,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_terraform_state", routes, nil))
}

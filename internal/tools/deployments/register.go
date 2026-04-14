// register.go wires deployments MCP tools to the MCP server.

package deployments

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five deployment management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_list",
		Title:       toolutil.TitleFromName("gitlab_deployment_list"),
		Description: "List deployments for a GitLab project. Supports filtering by environment name and status (created, running, success, failed, canceled). Returns paginated results with deployment details.\n\nReturns: paginated list of deployments with id, iid, ref, sha, status, environment, and timestamps. See also: gitlab_deployment_get, gitlab_environment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_get",
		Title:       toolutil.TitleFromName("gitlab_deployment_get"),
		Description: "Get details of a specific deployment in a GitLab project by its ID. Returns deployment ref, SHA, status, user, environment, and timestamps.\n\nReturns: id, iid, ref, sha, status, user_name, environment_name, and timestamps. See also: gitlab_deployment_list, gitlab_environment_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_get", start, nil)
			return toolutil.NotFoundResult("Deployment", fmt.Sprintf("ID %d in project %s", input.DeploymentID, input.ProjectID),
				"Use gitlab_deployment_list with project_id to list deployments",
				"Verify the deployment_id is correct for this project",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_create",
		Title:       toolutil.TitleFromName("gitlab_deployment_create"),
		Description: "Create a new deployment in a GitLab project. Requires environment name, git ref, and SHA. Optionally specify tag flag and initial status. Returns: id, iid, ref, sha, status, user_name, environment_name, created_at. See also: gitlab_deployment_get, gitlab_environment_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_update",
		Title:       toolutil.TitleFromName("gitlab_deployment_update"),
		Description: "Update the status of an existing deployment in a GitLab project. Use to transition a deployment between states: created, running, success, failed, canceled. Returns: id, iid, ref, sha, status, user_name, environment_name, updated_at. See also: gitlab_deployment_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_delete",
		Title:       toolutil.TitleFromName("gitlab_deployment_delete"),
		Description: "Permanently delete a deployment from a GitLab project. This action cannot be undone.\n\nReturns: confirmation message.\n\nSee also: gitlab_deployment_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete deployment %d from project %q?", input.DeploymentID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("deployment")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_deployment_approve_or_reject",
		Title:       toolutil.TitleFromName("gitlab_deployment_approve_or_reject"),
		Description: "Approve or reject a deployment that is blocked waiting for manual approval (environments with required approvals). Set status to 'approved' or 'rejected'. Only works when the deployment is in 'blocked' state — use gitlab_deployment_get to check status first. Optionally include a comment.\n\nReturns: deployment approval status with user and comment. See also: gitlab_deployment_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApproveOrRejectInput) (*mcp.CallToolResult, ApproveOrRejectOutput, error) {
		start := time.Now()
		out, err := ApproveOrReject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_deployment_approve_or_reject", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatApproveOrRejectMarkdown(out)), out, err)
	})
}

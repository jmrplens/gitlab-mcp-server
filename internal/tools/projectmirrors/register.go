package projectmirrors

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab project mirror operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	mirrorTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconInfra})
	}

	mcp.AddTool(server, mirrorTool("gitlab_list_project_mirrors", "List all remote push mirrors configured for a GitLab project. Returns mirror URL, status, enabled state, and configuration.\n\nReturns: JSON with mirrors array and pagination. See also: gitlab_get_project_mirror."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_mirrors", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, mirrorTool("gitlab_get_project_mirror", "Get a single remote push mirror for a GitLab project by its mirror ID, including URL, status, timestamps, and configuration.\n\nReturns: JSON with mirror details. See also: gitlab_list_project_mirrors."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_mirror", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, mirrorTool("gitlab_get_project_mirror_public_key", "Get the SSH public key for a remote push mirror, used for SSH-based mirror authentication.\n\nReturns: JSON with public_key field. See also: gitlab_get_project_mirror."), func(ctx context.Context, req *mcp.CallToolRequest, input GetPublicKeyInput) (*mcp.CallToolResult, PublicKeyOutput, error) {
		start := time.Now()
		out, err := GetPublicKey(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_mirror_public_key", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPublicKeyMarkdown(out)), out, err)
	})

	mcp.AddTool(server, mirrorTool("gitlab_add_project_mirror", "Create a new remote push mirror for a GitLab project. This push-mirror endpoint may use URL-embedded credentials when GitLab requires inline authentication; treat embedded credentials as secrets and redact them from logs, telemetry, and errors. Pull mirroring uses separate auth_user/auth_password fields and must not put credentials in the URL.\n\nReturns: JSON with created mirror details. See also: gitlab_edit_project_mirror."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_project_mirror", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, mirrorTool("gitlab_edit_project_mirror", "Update an existing remote push mirror configuration for a GitLab project.\n\nReturns: JSON with updated mirror details. See also: gitlab_get_project_mirror."), func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_project_mirror", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, mirrorTool("gitlab_delete_project_mirror", "Delete a remote push mirror from a GitLab project.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_project_mirrors."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete mirror %d from project %q?", input.MirrorID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_project_mirror", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("mirror %d from project %s", input.MirrorID, input.ProjectID))
	})

	mcp.AddTool(server, mirrorTool("gitlab_force_push_mirror_update", "Trigger an immediate update for a remote push mirror, bypassing the normal schedule.\n\nReturns: JSON confirming the update was triggered. See also: gitlab_get_project_mirror."), func(ctx context.Context, req *mcp.CallToolRequest, input ForcePushInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := ForcePushUpdate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_force_push_mirror_update", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		out := toolutil.DeleteOutput{Message: fmt.Sprintf("Force push update triggered for mirror %d in project %s", input.MirrorID, input.ProjectID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(fmt.Sprintf("Force push update triggered for mirror %d in project %s.", input.MirrorID, input.ProjectID)), out, nil)
	})
}

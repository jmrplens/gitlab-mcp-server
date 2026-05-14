package groupserviceaccounts

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group service account tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	serviceAccountTool := func(name, description string, icons []mcp.Icon) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: icons})
	}

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_list", "List all service accounts for a GitLab group.\n\nReturns: paginated list of service accounts with ID, name, username, and email. See also: gitlab_group_service_account_create, gitlab_group_service_account_pat_list.", toolutil.IconBot), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_create", "Create a service account in a GitLab group (top-level only).\n\nReturns: created service account details. See also: gitlab_group_service_account_list, gitlab_group_service_account_pat_create.", toolutil.IconBot), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_update", "Update a service account in a GitLab group (top-level only).\n\nReturns: updated service account details. See also: gitlab_group_service_account_list, gitlab_group_service_account_delete.", toolutil.IconBot), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_delete", "Delete a service account from a GitLab group (top-level only).\n\nReturns: confirmation of deletion. See also: gitlab_group_service_account_list, gitlab_group_service_account_create.", toolutil.IconBot), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group service account")
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_pat_list", "List personal access tokens for a group service account.\n\nReturns: paginated list of PATs with ID, name, scopes, and status. See also: gitlab_group_service_account_pat_create, gitlab_group_service_account_list.", toolutil.IconKey), func(ctx context.Context, req *mcp.CallToolRequest, input ListPATInput) (*mcp.CallToolResult, ListPATOutput, error) {
		start := time.Now()
		out, err := ListPATs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListPATMarkdown(out)), out, err)
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_pat_create", "Create a personal access token for a group service account.\n\nReturns: created PAT details including the token value (shown only once). See also: gitlab_group_service_account_pat_list, gitlab_group_service_account_pat_revoke.", toolutil.IconKey), func(ctx context.Context, req *mcp.CallToolRequest, input CreatePATInput) (*mcp.CallToolResult, PATOutput, error) {
		start := time.Now()
		out, err := CreatePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPATOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, serviceAccountTool("gitlab_group_service_account_pat_revoke", "Revoke a personal access token for a group service account.\n\nReturns: confirmation of revocation. See also: gitlab_group_service_account_pat_list, gitlab_group_service_account_pat_create.", toolutil.IconKey), func(ctx context.Context, req *mcp.CallToolRequest, input RevokePATInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := RevokePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_revoke", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("service account PAT")
	})
}

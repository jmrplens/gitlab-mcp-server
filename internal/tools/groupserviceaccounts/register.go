// register.go wires group service account MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_list",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_list"),
		Description: "List all service accounts for a GitLab group.\n\nReturns: paginated list of service accounts with ID, name, username, and email. See also: gitlab_group_service_account_create, gitlab_group_service_account_pat_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBot,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_create",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_create"),
		Description: "Create a service account in a GitLab group (top-level only).\n\nReturns: created service account details. See also: gitlab_group_service_account_list, gitlab_group_service_account_pat_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBot,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_update",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_update"),
		Description: "Update a service account in a GitLab group (top-level only).\n\nReturns: updated service account details. See also: gitlab_group_service_account_list, gitlab_group_service_account_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBot,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_delete",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_delete"),
		Description: "Delete a service account from a GitLab group (top-level only).\n\nReturns: confirmation of deletion. See also: gitlab_group_service_account_list, gitlab_group_service_account_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBot,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group service account")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_pat_list",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_pat_list"),
		Description: "List personal access tokens for a group service account.\n\nReturns: paginated list of PATs with ID, name, scopes, and status. See also: gitlab_group_service_account_pat_create, gitlab_group_service_account_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListPATInput) (*mcp.CallToolResult, ListPATOutput, error) {
		start := time.Now()
		out, err := ListPATs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListPATMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_pat_create",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_pat_create"),
		Description: "Create a personal access token for a group service account.\n\nReturns: created PAT details including the token value (shown only once). See also: gitlab_group_service_account_pat_list, gitlab_group_service_account_pat_revoke.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreatePATInput) (*mcp.CallToolResult, PATOutput, error) {
		start := time.Now()
		out, err := CreatePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPATOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_service_account_pat_revoke",
		Title:       toolutil.TitleFromName("gitlab_group_service_account_pat_revoke"),
		Description: "Revoke a personal access token for a group service account.\n\nReturns: confirmation of revocation. See also: gitlab_group_service_account_pat_list, gitlab_group_service_account_pat_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RevokePATInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := RevokePAT(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_service_account_pat_revoke", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("service account PAT")
	})
}

package accesstokens

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all access token management MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// -----------------------------------------------------------------------
	// Project Access Tokens
	// -----------------------------------------------------------------------

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_list",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_list"),
		Description: "List all access tokens for a GitLab project. Filter by state (active, inactive).\n\nSee also: gitlab_group_access_token_list, gitlab_personal_access_token_list\n\nReturns: JSON array of access tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ProjectList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_get",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_get"),
		Description: "Get a specific project access token by its ID.\n\nSee also: gitlab_project_access_token_list, gitlab_project_access_token_rotate\n\nReturns: JSON with access token details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectGet(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_create",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_create"),
		Description: "Create a new project access token with specified name, scopes, access level, and optional expiry date.\n\nSee also: gitlab_project_access_token_list, gitlab_group_access_token_create\n\nReturns: JSON with the created access token including the token value.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectCreate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_rotate",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_rotate"),
		Description: "Rotate a project access token, generating a new token value. Optionally set a new expiry date.\n\nSee also: gitlab_project_access_token_list, gitlab_project_access_token_revoke\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectRotateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectRotate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_rotate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_revoke",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_revoke"),
		Description: "Revoke a project access token. This action cannot be undone.\n\nSee also: gitlab_project_access_token_list, gitlab_project_access_token_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectRevokeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke access token %d from project %s? This cannot be undone.", input.TokenID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := ProjectRevoke(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_revoke", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("project access token")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_access_token_rotate_self",
		Title:       toolutil.TitleFromName("gitlab_project_access_token_rotate_self"),
		Description: "Rotate the project access token used for the current request. Returns the new token value.\n\nSee also: gitlab_project_access_token_rotate, gitlab_personal_access_token_rotate_self\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectRotateSelfInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := ProjectRotateSelf(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_access_token_rotate_self", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	// -----------------------------------------------------------------------
	// Group Access Tokens
	// -----------------------------------------------------------------------

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_list",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_list"),
		Description: "List all access tokens for a GitLab group. Filter by state (active, inactive).\n\nSee also: gitlab_project_access_token_list, gitlab_personal_access_token_list\n\nReturns: JSON array of access tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := GroupList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_get",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_get"),
		Description: "Get a specific group access token by its ID.\n\nSee also: gitlab_group_access_token_list, gitlab_group_access_token_rotate\n\nReturns: JSON with access token details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GroupGet(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_create",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_create"),
		Description: "Create a new group access token with specified name, scopes, access level, and optional expiry date.\n\nSee also: gitlab_group_access_token_list, gitlab_project_access_token_create\n\nReturns: JSON with the created access token including the token value.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupCreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GroupCreate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_rotate",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_rotate"),
		Description: "Rotate a group access token, generating a new token value. Optionally set a new expiry date.\n\nSee also: gitlab_group_access_token_list, gitlab_group_access_token_revoke\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupRotateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GroupRotate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_rotate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_revoke",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_revoke"),
		Description: "Revoke a group access token. This action cannot be undone.\n\nSee also: gitlab_group_access_token_list, gitlab_group_access_token_create\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupRevokeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke access token %d from group %s? This cannot be undone.", input.TokenID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := GroupRevoke(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_revoke", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group access token")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_access_token_rotate_self",
		Title:       toolutil.TitleFromName("gitlab_group_access_token_rotate_self"),
		Description: "Rotate the group access token used for the current request. Returns the new token value.\n\nSee also: gitlab_group_access_token_rotate, gitlab_personal_access_token_rotate_self\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupRotateSelfInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GroupRotateSelf(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_access_token_rotate_self", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	// -----------------------------------------------------------------------
	// Personal Access Tokens
	// -----------------------------------------------------------------------

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_list",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_list"),
		Description: "List personal access tokens. Filter by state, search by name, or filter by user ID (admin only).\n\nSee also: gitlab_project_access_token_list, gitlab_group_access_token_list\n\nReturns: JSON array of access tokens with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := PersonalList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_get",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_get"),
		Description: "Get a personal access token by ID. Use token_id=0 to retrieve the current token used for authentication.\n\nSee also: gitlab_personal_access_token_list, gitlab_personal_access_token_rotate\n\nReturns: JSON with access token details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalGetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := PersonalGet(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_rotate",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_rotate"),
		Description: "Rotate a personal access token, generating a new token value. Optionally set a new expiry date.\n\nSee also: gitlab_personal_access_token_list, gitlab_personal_access_token_revoke\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalRotateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := PersonalRotate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_rotate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_revoke",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_revoke"),
		Description: "Revoke a personal access token by ID. This action cannot be undone.\n\nSee also: gitlab_personal_access_token_list, gitlab_personal_access_token_rotate\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalRevokeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Revoke personal access token %d? This cannot be undone.", input.TokenID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := PersonalRevoke(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_revoke", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("personal access token")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_rotate_self",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_rotate_self"),
		Description: "Rotate the personal access token used for the current request. Returns the new token value.\n\nSee also: gitlab_personal_access_token_rotate, gitlab_project_access_token_rotate_self\n\nReturns: JSON with the rotated access token including the new token value.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalRotateSelfInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := PersonalRotateSelf(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_rotate_self", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_personal_access_token_revoke_self",
		Title:       toolutil.TitleFromName("gitlab_personal_access_token_revoke_self"),
		Description: "Revoke the personal access token used for the current request. This action cannot be undone.\n\nSee also: gitlab_personal_access_token_revoke, gitlab_personal_access_token_list\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PersonalRevokeSelfInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, "Revoke the current personal access token? This cannot be undone."); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := PersonalRevokeSelf(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_personal_access_token_revoke_self", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("personal access token")
	})
}

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

// RegisterMeta registers the gitlab_access_token meta-tool with all access token actions.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"project_list":         toolutil.RouteAction(client, ProjectList),
		"project_get":          toolutil.RouteAction(client, ProjectGet),
		"project_create":       toolutil.RouteAction(client, ProjectCreate),
		"project_rotate":       toolutil.RouteAction(client, ProjectRotate),
		"project_rotate_self":  toolutil.RouteAction(client, ProjectRotateSelf),
		"project_revoke":       toolutil.DestructiveVoidAction(client, ProjectRevoke),
		"group_list":           toolutil.RouteAction(client, GroupList),
		"group_get":            toolutil.RouteAction(client, GroupGet),
		"group_create":         toolutil.RouteAction(client, GroupCreate),
		"group_rotate":         toolutil.RouteAction(client, GroupRotate),
		"group_rotate_self":    toolutil.RouteAction(client, GroupRotateSelf),
		"group_revoke":         toolutil.DestructiveVoidAction(client, GroupRevoke),
		"personal_list":        toolutil.RouteAction(client, PersonalList),
		"personal_get":         toolutil.RouteAction(client, PersonalGet),
		"personal_rotate":      toolutil.RouteAction(client, PersonalRotate),
		"personal_rotate_self": toolutil.RouteAction(client, PersonalRotateSelf),
		"personal_revoke":      toolutil.DestructiveVoidAction(client, PersonalRevoke),
		"personal_revoke_self": toolutil.DestructiveVoidAction(client, PersonalRevokeSelf),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_access_token",
		Title: toolutil.TitleFromName("gitlab_access_token"),
		Description: `Manage access tokens in GitLab (project, group, and personal). Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- project_list: List project access tokens. Params: project_id (required), state (active/inactive), page, per_page
- project_get: Get a project access token. Params: project_id (required), token_id (required, int)
- project_create: Create project access token. Params: project_id (required), name (required), scopes (required, array), access_level (int), description, expires_at (YYYY-MM-DD)
- project_rotate: Rotate project access token. Params: project_id (required), token_id (required, int), expires_at (YYYY-MM-DD)
- project_rotate_self: Rotate the project access token used for auth. Params: project_id (required), expires_at (YYYY-MM-DD)
- project_revoke: Revoke project access token. Params: project_id (required), token_id (required, int)
- group_list: List group access tokens. Params: group_id (required), state (active/inactive), page, per_page
- group_get: Get a group access token. Params: group_id (required), token_id (required, int)
- group_create: Create group access token. Params: group_id (required), name (required), scopes (required, array), access_level (int), description, expires_at (YYYY-MM-DD)
- group_rotate: Rotate group access token. Params: group_id (required), token_id (required, int), expires_at (YYYY-MM-DD)
- group_rotate_self: Rotate the group access token used for auth. Params: group_id (required), expires_at (YYYY-MM-DD)
- group_revoke: Revoke group access token. Params: group_id (required), token_id (required, int)
- personal_list: List personal access tokens. Params: state (active/inactive), search, user_id (int, admin only), page, per_page
- personal_get: Get a personal access token. Params: token_id (int, use 0 for current token)
- personal_rotate: Rotate personal access token. Params: token_id (required, int), expires_at (YYYY-MM-DD)
- personal_rotate_self: Rotate the personal access token used for auth. Params: expires_at (YYYY-MM-DD)
- personal_revoke: Revoke personal access token. Params: token_id (required, int)
- personal_revoke_self: Revoke the personal access token used for auth. No params`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconToken,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_access_token", routes, nil))
}

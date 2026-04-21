// register.go wires featureflags MCP tools to the MCP server.

package featureflags

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project feature flag individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_feature_flag_list",
		Title:       toolutil.TitleFromName("gitlab_feature_flag_list"),
		Description: "List feature flags for a project.\n\nReturns: JSON with feature flags array including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_get, gitlab_ff_user_list_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListFeatureFlags(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListFeatureFlagsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_feature_flag_get",
		Title:       toolutil.TitleFromName("gitlab_feature_flag_get"),
		Description: "Get a single feature flag by name.\n\nReturns: JSON with feature flag details including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_list, gitlab_feature_flag_update",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_feature_flag_create",
		Title:       toolutil.TitleFromName("gitlab_feature_flag_create"),
		Description: "Create a new feature flag for a project.\n\nReturns: JSON with created feature flag including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_list, gitlab_ff_user_list_create",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_feature_flag_update",
		Title:       toolutil.TitleFromName("gitlab_feature_flag_update"),
		Description: "Update an existing feature flag.\n\nReturns: JSON with updated feature flag including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_get, gitlab_feature_flag_delete",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_feature_flag_delete",
		Title:       toolutil.TitleFromName("gitlab_feature_flag_delete"),
		Description: "Delete a feature flag.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_feature_flag_list, gitlab_feature_flag_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete feature flag %q from project %s?", input.Name, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("feature flag")
	})
}

// RegisterMeta registers the feature flag meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"list":   toolutil.WrapAction(client, ListFeatureFlags),
		"get":    toolutil.WrapAction(client, GetFeatureFlag),
		"create": toolutil.WrapAction(client, CreateFeatureFlag),
		"update": toolutil.WrapAction(client, UpdateFeatureFlag),
		"delete": toolutil.WrapVoidAction(client, DeleteFeatureFlag),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_feature_flag",
		Title: toolutil.TitleFromName("gitlab_feature_flag"),
		Description: `Project feature flag operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List feature flags (project_id, scope, page, per_page)
- get: Get a feature flag by name (project_id, name)
- create: Create a feature flag (project_id, name, description, version, active, strategies)
- update: Update a feature flag (project_id, name, new_name, description, active, strategies)
- delete: Delete a feature flag (project_id, name)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconConfig,
	}, toolutil.MakeMetaHandler("gitlab_feature_flag", routes, nil))
}

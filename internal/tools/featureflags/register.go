package featureflags

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all project feature flag individual tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	featureFlagTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, featureFlagTool("gitlab_feature_flag_list", "List feature flags for a project.\n\nReturns: JSON with feature flags array including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_get, gitlab_ff_user_list_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListFeatureFlags(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListFeatureFlagsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, featureFlagTool("gitlab_feature_flag_get", "Get a single feature flag by name.\n\nReturns: JSON with feature flag details including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_list, gitlab_feature_flag_update"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out))
		if err == nil && out.Name != "" && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/feature_flag/%s", url.PathEscape(string(input.ProjectID)), url.PathEscape(out.Name)),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, featureFlagTool("gitlab_feature_flag_create", "Create a new feature flag for a project.\n\nReturns: JSON with created feature flag including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_list, gitlab_ff_user_list_create"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, featureFlagTool("gitlab_feature_flag_update", "Update an existing feature flag.\n\nReturns: JSON with updated feature flag including name, active status, and strategies.\n\nSee also: gitlab_feature_flag_get, gitlab_feature_flag_delete"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UpdateFeatureFlag(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_feature_flag_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatFeatureFlagMarkdown(out)), out, err)
	})

	mcp.AddTool(server, featureFlagTool("gitlab_feature_flag_delete", "Delete a feature flag.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_feature_flag_list, gitlab_feature_flag_create"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

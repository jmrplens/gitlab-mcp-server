// register.go wires features MCP tools to the MCP server.

package features

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all feature flag tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_features",
		Title:       toolutil.TitleFromName("gitlab_list_features"),
		Description: "List all feature flags (admin). Returns name, state and gates for each flag.\n\nReturns: JSON array of feature flags.\n\nSee also: gitlab_set_feature_flag.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_features", start, err)
		if err != nil {
			return nil, ListOutput{}, err
		}
		return toolutil.WithHints(FormatListMarkdown(out), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_feature_definitions",
		Title:       toolutil.TitleFromName("gitlab_list_feature_definitions"),
		Description: "List all feature definitions (admin). Returns name, type, group, milestone and default_enabled for each definition.\n\nReturns: JSON array of feature definitions.\n\nSee also: gitlab_list_features.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListDefinitionsInput) (*mcp.CallToolResult, ListDefinitionsOutput, error) {
		start := time.Now()
		out, err := ListDefinitions(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_feature_definitions", start, err)
		if err != nil {
			return nil, ListDefinitionsOutput{}, err
		}
		return toolutil.WithHints(FormatListDefinitionsMarkdown(out), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_set_feature_flag",
		Title:       toolutil.TitleFromName("gitlab_set_feature_flag"),
		Description: "Set or create a feature flag (admin). Requires name and value. Supports scoping to user, group, project, namespace, or repository.\n\nReturns: JSON with the feature flag details.\n\nSee also: gitlab_list_features.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetInput) (*mcp.CallToolResult, SetOutput, error) {
		start := time.Now()
		out, err := Set(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_feature_flag", start, err)
		if err != nil {
			return nil, SetOutput{}, err
		}
		return toolutil.WithHints(FormatFeatureMarkdown(out), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_feature_flag",
		Title:       toolutil.TitleFromName("gitlab_delete_feature_flag"),
		Description: "Delete a feature flag (admin). Requires the flag name.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_feature_flags.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete feature flag %q?", input.Name)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_feature_flag", start, err)
		r, o, _ := toolutil.DeleteResult("feature flag")
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return r, o, nil
	})
}

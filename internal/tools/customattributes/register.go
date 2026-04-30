// register.go wires customattributes MCP tools to the MCP server.
package customattributes

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Custom Attributes MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_custom_attributes",
		Title:       toolutil.TitleFromName("gitlab_list_custom_attributes"),
		Description: "List custom attributes for a user, group, or project (admin). Params: resource_type (required: user|group|project), resource_id (required).\n\nReturns: JSON array of custom attributes.\n\nSee also: gitlab_get_custom_attribute, gitlab_set_custom_attribute",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_custom_attributes", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_custom_attribute",
		Title:       toolutil.TitleFromName("gitlab_get_custom_attribute"),
		Description: "Get a custom attribute by key for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required).\n\nReturns: JSON with the custom attribute details.\n\nSee also: gitlab_list_custom_attributes, gitlab_set_custom_attribute",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_custom_attribute", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_set_custom_attribute",
		Title:       toolutil.TitleFromName("gitlab_set_custom_attribute"),
		Description: "Set (create/update) a custom attribute for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required), value (required).\n\nReturns: JSON with the custom attribute details.\n\nSee also: gitlab_list_custom_attributes, gitlab_delete_custom_attribute",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetInput) (*mcp.CallToolResult, SetOutput, error) {
		start := time.Now()
		out, err := Set(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_custom_attribute", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatSetMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_custom_attribute",
		Title:       toolutil.TitleFromName("gitlab_delete_custom_attribute"),
		Description: "Delete a custom attribute for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required).\n\nReturns: confirmation message.\n\nSee also: gitlab_list_custom_attributes, gitlab_set_custom_attribute",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete custom attribute %q from %s %d?", input.Key, input.ResourceType, input.ResourceID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_custom_attribute", start, err)
		r, o, _ := toolutil.DeleteResult("custom_attribute")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

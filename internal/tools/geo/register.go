// Package geo register.go wires Geo site MCP tools to the MCP server.
package geo

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab Geo site operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_geo_site",
		Title:       toolutil.TitleFromName("gitlab_create_geo_site"),
		Description: "Create a new Geo replication site.\n\nReturns: JSON with the created Geo site configuration.\n\nSee also: gitlab_list_geo_sites, gitlab_edit_geo_site",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_geo_site", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_geo_sites",
		Title:       toolutil.TitleFromName("gitlab_list_geo_sites"),
		Description: "List all Geo replication sites.\n\nReturns: JSON with array of Geo sites and pagination.\n\nSee also: gitlab_get_geo_site, gitlab_list_status_all_geo_sites",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_geo_sites", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_geo_site",
		Title:       toolutil.TitleFromName("gitlab_get_geo_site"),
		Description: "Get configuration of a specific Geo site by ID.\n\nReturns: JSON with Geo site configuration.\n\nSee also: gitlab_list_geo_sites, gitlab_get_status_geo_site",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_geo_site", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_edit_geo_site",
		Title:       toolutil.TitleFromName("gitlab_edit_geo_site"),
		Description: "Update configuration of an existing Geo site.\n\nReturns: JSON with updated Geo site configuration.\n\nSee also: gitlab_get_geo_site, gitlab_create_geo_site",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_geo_site", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_geo_site",
		Title:       toolutil.TitleFromName("gitlab_delete_geo_site"),
		Description: "Delete a Geo replication site by ID.\n\nReturns: JSON with deletion confirmation.\n\nSee also: gitlab_list_geo_sites",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete Geo site %d?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_geo_site", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("Geo site %d", input.ID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repair_geo_site",
		Title:       toolutil.TitleFromName("gitlab_repair_geo_site"),
		Description: "Repair the OAuth authentication of a Geo site.\n\nReturns: JSON with the repaired Geo site configuration.\n\nSee also: gitlab_get_geo_site",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Repair(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repair_geo_site", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_status_all_geo_sites",
		Title:       toolutil.TitleFromName("gitlab_list_status_all_geo_sites"),
		Description: "Retrieve replication status of all Geo sites.\n\nReturns: JSON with array of Geo site statuses and pagination.\n\nSee also: gitlab_get_status_geo_site, gitlab_list_geo_sites",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListStatusInput) (*mcp.CallToolResult, ListStatusOutput, error) {
		start := time.Now()
		out, err := ListStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_status_all_geo_sites", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListStatusMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_status_geo_site",
		Title:       toolutil.TitleFromName("gitlab_get_status_geo_site"),
		Description: "Retrieve replication status of a specific Geo site by ID.\n\nReturns: JSON with the Geo site's replication status.\n\nSee also: gitlab_list_status_all_geo_sites, gitlab_get_geo_site",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := GetStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_status_geo_site", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatStatusMarkdown(out)), out, err)
	})
}

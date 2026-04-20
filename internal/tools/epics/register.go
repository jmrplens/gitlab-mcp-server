// register.go wires epic MCP tools to the MCP server.
// Five of six tools use the Work Items GraphQL API; GetLinks remains on REST.

package epics

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group epic operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_list",
		Title:       toolutil.TitleFromName("gitlab_epic_list"),
		Description: "List epics for a GitLab group (via Work Items GraphQL API with type=Epic). Supports filtering by state, labels, author, search text, and cursor-based pagination.\n\nReturns: JSON with epics array. See also: gitlab_epic_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_get",
		Title:       toolutil.TitleFromName("gitlab_epic_get"),
		Description: "Get a single group epic by its IID (via Work Items GraphQL API). Returns title, description, state, labels, dates, author, assignees, linked items, and health status.\n\nReturns: JSON with epic details. See also: gitlab_epic_list, gitlab_epic_get_links.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_get_links",
		Title:       toolutil.TitleFromName("gitlab_epic_get_links"),
		Description: "Get all child epics of a parent epic (via REST API). Returns the list of sub-epics linked to the specified epic.\n\nNote: This tool uses the REST API because the Work Items GraphQL API does not yet support listing children.\n\nReturns: JSON with child_epics array. See also: gitlab_epic_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetLinksInput) (*mcp.CallToolResult, LinksOutput, error) {
		start := time.Now()
		out, err := GetLinks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_get_links", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLinksMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_create",
		Title:       toolutil.TitleFromName("gitlab_epic_create"),
		Description: "Create a new epic in a GitLab group (via Work Items GraphQL API). Supports title, description, confidentiality, color, dates, assignees, labels, weight, and health status.\n\nReturns: JSON with created epic details. See also: gitlab_epic_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_update",
		Title:       toolutil.TitleFromName("gitlab_epic_update"),
		Description: "Update an existing group epic (via Work Items GraphQL API). Can modify title, description, labels, state (CLOSE/REOPEN), confidentiality, parent, color, dates, assignees, weight, health status, and status.\n\nReturns: JSON with updated epic details. See also: gitlab_epic_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_delete",
		Title:       toolutil.TitleFromName("gitlab_epic_delete"),
		Description: "Permanently delete an epic from a GitLab group (via Work Items GraphQL API).\n\nReturns: JSON with deletion confirmation. See also: gitlab_epic_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete epic &%d from group %q?", input.IID, input.FullPath)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("epic &%d from group %s", input.IID, input.FullPath))
	})
}

// register.go wires epic MCP tools to the MCP server.

package groupscim

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group SCIM identity operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_scim_identities",
		Title:       toolutil.TitleFromName("gitlab_list_group_scim_identities"),
		Description: "List all SCIM identities for a GitLab group. Returns external UIDs, user IDs, and active status.\n\nReturns: JSON with identities array. See also: gitlab_get_group_scim_identity.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_scim_identities", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_scim_identity",
		Title:       toolutil.TitleFromName("gitlab_get_group_scim_identity"),
		Description: "Get a single SCIM identity for a GitLab group by UID.\n\nReturns: JSON with identity details. See also: gitlab_list_group_scim_identities.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_scim_identity", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_group_scim_identity",
		Title:       toolutil.TitleFromName("gitlab_update_group_scim_identity"),
		Description: "Update a SCIM identity for a GitLab group. Changes the external UID.\n\nReturns: JSON with confirmation. See also: gitlab_get_group_scim_identity.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_group_scim_identity", start, err)
		if err != nil {
			return nil, UpdateOutput{}, err
		}
		out := UpdateOutput{Updated: true, Message: fmt.Sprintf("SCIM identity %s updated in group %s", input.UID, input.GroupID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(fmt.Sprintf("SCIM identity `%s` updated successfully.", input.UID)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_scim_identity",
		Title:       toolutil.TitleFromName("gitlab_delete_group_scim_identity"),
		Description: "Delete a SCIM identity from a GitLab group.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_group_scim_identities.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete SCIM identity %q from group %q?", input.UID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_scim_identity", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("SCIM identity %s from group %s", input.UID, input.GroupID))
	})
}

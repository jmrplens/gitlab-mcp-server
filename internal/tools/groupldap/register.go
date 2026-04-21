package groupldap

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group LDAP link tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_ldap_link_list",
		Title:       toolutil.TitleFromName("gitlab_group_ldap_link_list"),
		Description: "List all LDAP group links for a GitLab group.\n\nReturns: list of LDAP links with CN, filter, access level, and provider.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_ldap_link_add",
		Title:       toolutil.TitleFromName("gitlab_group_ldap_link_add"),
		Description: "Add an LDAP group link to a GitLab group (by CN or filter).\n\nReturns: created LDAP link details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_ldap_link_delete",
		Title:       toolutil.TitleFromName("gitlab_group_ldap_link_delete"),
		Description: "Delete a group LDAP link by CN or filter.\n\nReturns: confirmation of deletion.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteWithCNOrFilterInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := DeleteWithCNOrFilter(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group LDAP link")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_ldap_link_delete_for_provider",
		Title:       toolutil.TitleFromName("gitlab_group_ldap_link_delete_for_provider"),
		Description: "Delete a group LDAP link for a specific provider.\n\nReturns: confirmation of deletion.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForProviderInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := DeleteForProvider(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_delete_for_provider", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group LDAP link")
	})
}

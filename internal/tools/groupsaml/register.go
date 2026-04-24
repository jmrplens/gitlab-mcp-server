// register.go wires group SAML MCP tools to the MCP server.

package groupsaml

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// DeleteOutput confirms the deletion of a SAML link.
type DeleteOutput = toolutil.DeleteOutput

// RegisterTools registers group SAML link tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_saml_link_list",
		Title:       toolutil.TitleFromName("gitlab_group_saml_link_list"),
		Description: "List all SAML group links for a GitLab group.\n\nReturns: list of SAML links with name, access level, and provider. See also: gitlab_group_saml_link_get, gitlab_group_saml_link_add, gitlab_group_ldap_link_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_saml_link_get",
		Title:       toolutil.TitleFromName("gitlab_group_saml_link_get"),
		Description: "Get a single SAML group link by name.\n\nReturns: SAML link details with access level. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_delete.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_saml_link_add",
		Title:       toolutil.TitleFromName("gitlab_group_saml_link_add"),
		Description: "Add a SAML group link to a GitLab group.\n\nReturns: created SAML link details. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_delete.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_saml_link_delete",
		Title:       toolutil.TitleFromName("gitlab_group_saml_link_delete"),
		Description: "Delete a SAML group link from a GitLab group.\n\nReturns: confirmation of deletion. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group SAML link")
	})
}

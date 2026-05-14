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
	specs := ActionSpecs(client)
	groupSAMLTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconGroup})
	}

	mcp.AddTool(server, groupSAMLTool("gitlab_group_saml_link_list", "List all SAML group links for a GitLab group.\n\nReturns: list of SAML links with name, access level, and provider. See also: gitlab_group_saml_link_get, gitlab_group_saml_link_add, gitlab_group_ldap_link_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupSAMLTool("gitlab_group_saml_link_get", "Get a single SAML group link by name.\n\nReturns: SAML link details with access level. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_delete."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupSAMLTool("gitlab_group_saml_link_add", "Add a SAML group link to a GitLab group.\n\nReturns: created SAML link details. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_delete."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupSAMLTool("gitlab_group_saml_link_delete", "Delete a SAML group link from a GitLab group.\n\nReturns: confirmation of deletion. See also: gitlab_group_saml_link_list, gitlab_group_saml_link_get."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_saml_link_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group SAML link")
	})
}

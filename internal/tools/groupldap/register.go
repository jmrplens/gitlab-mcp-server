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
	specs := ActionSpecs(client)
	groupLDAPTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconGroup})
	}

	mcp.AddTool(server, groupLDAPTool("gitlab_group_ldap_link_list", "List all LDAP group links for a GitLab group.\n\nReturns: list of LDAP links with CN, filter, access level, and provider. See also: gitlab_group_ldap_link_add, gitlab_group_saml_link_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupLDAPTool("gitlab_group_ldap_link_add", "Add an LDAP group link to a GitLab group (by CN or filter).\n\nReturns: created LDAP link details. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_delete."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, groupLDAPTool("gitlab_group_ldap_link_delete", "Delete a group LDAP link by CN or filter.\n\nReturns: confirmation of deletion. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_delete_for_provider."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteWithCNOrFilterInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := DeleteWithCNOrFilter(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group LDAP link")
	})

	mcp.AddTool(server, groupLDAPTool("gitlab_group_ldap_link_delete_for_provider", "Delete a group LDAP link for a specific provider.\n\nReturns: confirmation of deletion. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_delete."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForProviderInput) (*mcp.CallToolResult, DeleteOutput, error) {
		start := time.Now()
		err := DeleteForProvider(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_ldap_link_delete_for_provider", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group LDAP link")
	})
}

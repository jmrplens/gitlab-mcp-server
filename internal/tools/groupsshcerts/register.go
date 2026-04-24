// register.go wires group SSH certificate MCP tools to the MCP server.
package groupsshcerts

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group SSH certificate operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_ssh_certificates",
		Title:       toolutil.TitleFromName("gitlab_list_group_ssh_certificates"),
		Description: "List all SSH certificates for a GitLab group.\n\nReturns: JSON with certificates array. See also: gitlab_create_group_ssh_certificate.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_ssh_certificates", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_group_ssh_certificate",
		Title:       toolutil.TitleFromName("gitlab_create_group_ssh_certificate"),
		Description: "Create an SSH certificate for a GitLab group. Provide a public key and title.\n\nReturns: JSON with created certificate details. See also: gitlab_list_group_ssh_certificates.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_group_ssh_certificate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_ssh_certificate",
		Title:       toolutil.TitleFromName("gitlab_delete_group_ssh_certificate"),
		Description: "Delete an SSH certificate from a GitLab group.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_group_ssh_certificates.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete SSH certificate %d from group %q?", input.CertificateID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_ssh_certificate", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("SSH certificate %d from group %s", input.CertificateID, input.GroupID))
	})
}

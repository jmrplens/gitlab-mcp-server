package license

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all license tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_license",
		Title:       toolutil.TitleFromName("gitlab_get_license"),
		Description: "Get current GitLab license information (admin). Returns plan, expiry, user counts and licensee.\n\nReturns: JSON with license details.\n\nSee also: gitlab_add_license.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_license", start, err)
		if err != nil {
			return nil, GetOutput{}, err
		}
		return toolutil.WithHints(FormatGetMarkdown(out), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_license",
		Title:       toolutil.TitleFromName("gitlab_add_license"),
		Description: "Add a new GitLab license (admin). Requires the Base64-encoded license string.\n\nReturns: JSON with the added license details.\n\nSee also: gitlab_get_license.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_license", start, err)
		if err != nil {
			return nil, AddOutput{}, err
		}
		return toolutil.WithHints(FormatAddMarkdown(out), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_license",
		Title:       toolutil.TitleFromName("gitlab_delete_license"),
		Description: "Delete a GitLab license by ID (admin).\n\nReturns: confirmation message.\n\nSee also: gitlab_get_license.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete license %d?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_license", start, err)
		r, o, _ := toolutil.DeleteResult("license")
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return r, o, nil
	})
}

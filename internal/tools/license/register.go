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
	specs := ActionSpecs(client)
	licenseTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSecurity})
	}

	mcp.AddTool(server, licenseTool("gitlab_get_license", "Get current GitLab license information (admin). Returns plan, expiry, user counts and licensee.\n\nReturns: JSON with license details.\n\nSee also: gitlab_add_license."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_license", start, err)
		if err != nil {
			return nil, GetOutput{}, err
		}
		return toolutil.WithHints(FormatGetMarkdown(out), out, nil)
	})

	mcp.AddTool(server, licenseTool("gitlab_add_license", "Add a new GitLab license (admin). Requires the Base64-encoded license string.\n\nReturns: JSON with the added license details.\n\nSee also: gitlab_get_license."), func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
		start := time.Now()
		out, err := Add(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_license", start, err)
		if err != nil {
			return nil, AddOutput{}, err
		}
		return toolutil.WithHints(FormatAddMarkdown(out), out, nil)
	})

	mcp.AddTool(server, licenseTool("gitlab_delete_license", "Delete a GitLab license by ID (admin).\n\nReturns: confirmation message.\n\nSee also: gitlab_get_license."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

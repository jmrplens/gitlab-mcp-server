package dependencyproxy

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all dependency proxy tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	dependencyProxyTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconPackage})
	}

	mcp.AddTool(server, dependencyProxyTool("gitlab_purge_dependency_proxy", "Purge the dependency proxy cache for a GitLab group.\n\nReturns: confirmation message.\n\nSee also: gitlab_group_get."), func(ctx context.Context, req *mcp.CallToolRequest, input PurgeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Purge dependency proxy cache for group %s?", input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Purge(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_purge_dependency_proxy", start, err)
		r, o, _ := toolutil.DeleteResult("dependency proxy cache")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}

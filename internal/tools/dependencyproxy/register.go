// register.go wires dependencyproxy MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_purge_dependency_proxy",
		Title:       toolutil.TitleFromName("gitlab_purge_dependency_proxy"),
		Description: "Purge the dependency proxy cache for a GitLab group.\n\nReturns: confirmation message.\n\nSee also: gitlab_group_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PurgeInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

// RegisterMeta registers the gitlab_dependency_proxy meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"purge": toolutil.DestructiveVoidAction(client, Purge),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_dependency_proxy",
		Title: toolutil.TitleFromName("gitlab_dependency_proxy"),
		Description: `Manage dependency proxy in GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- purge: Purge the dependency proxy cache for a group. Params: group_id (required)`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconPackage,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_dependency_proxy", routes, nil))
}

// register.go wires project discovery MCP tools to the MCP server.

package projectdiscovery

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers project discovery tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_resolve_project_from_remote",
		Title: toolutil.TitleFromName("gitlab_resolve_project_from_remote"),
		Description: "Resolve a git remote URL to a GitLab project. " +
			"Extract the FULL remote URL from the workspace .git/config file (look for [remote \"origin\"] url = ...) " +
			"or from 'git remote -v' output, and pass it here to discover the project_id needed for all other GitLab operations. " +
			"IMPORTANT: Pass the complete URL exactly as it appears — do NOT strip the git@ prefix from SSH URLs. " +
			"Supported formats: HTTPS (https://gitlab.example.com/group/project.git), " +
			"SSH shorthand (git@gitlab.example.com:group/project.git), " +
			"SSH protocol (ssh://git@gitlab.example.com/group/project.git)." +
			"\n\nReturns: JSON with the resolved project ID and details.\n\nSee also: gitlab_project_get, gitlab_project_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveInput) (*mcp.CallToolResult, ResolveOutput, error) {
		start := time.Now()
		out, err := Resolve(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_resolve_project_from_remote", start, err)
		if err != nil {
			return nil, out, err
		}
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: FormatMarkdown(out)},
			},
		}
		return toolutil.WithHints(result, out, nil)
	})
}

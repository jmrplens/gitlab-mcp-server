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
		Name:  "gitlab_discover_project",
		Title: toolutil.TitleFromName("gitlab_discover_project"),
		Description: "Resolve a git remote URL to a GitLab project and return its project_id and metadata. " +
			"Read-only; performs at most one authenticated GET /projects/:path lookup; no side effects.\n\n" +
			"When to use: at the start of a workspace session, to obtain the project_id required by most other gitlab_* tools. " +
			"Extract the FULL remote URL from .git/config ([remote \"origin\"] url = ...) or from 'git remote -v'.\n" +
			"NOT for: searching projects by name (use gitlab_search action=projects), listing a user's projects (use gitlab_user action=list_projects), " +
			"verifying GitLab connectivity or authentication (use gitlab_server action=health_check).\n\n" +
			"IMPORTANT: pass the complete URL exactly as it appears — do NOT strip the git@ prefix from SSH URLs. " +
			"Supported formats:\n" +
			"- HTTPS: https://gitlab.example.com/group/project.git\n" +
			"- SSH shorthand: git@gitlab.example.com:group/project.git\n" +
			"- SSH protocol: ssh://git@gitlab.example.com/group/project.git\n" +
			"- Bare path: gitlab.example.com/group/project\n\n" +
			"Returns: {id, name, path_with_namespace, web_url, description, default_branch, visibility, archived}. " +
			"Errors: 404 not found (hint: project may be private — verify token permissions), 403 forbidden (hint: token lacks read_api scope).\n\n" +
			"See also: gitlab_project (full project CRUD/settings once id is known), gitlab_server (connectivity and version checks), gitlab_search (find projects by query).",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveInput) (*mcp.CallToolResult, ResolveOutput, error) {
		start := time.Now()
		out, err := Resolve(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_discover_project", start, err)
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

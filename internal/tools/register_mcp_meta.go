// register_mcp_meta.go registers the gitlab_server meta-tool that exposes
// MCP server health, version, and update operations.

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterMCPMeta registers the gitlab_server meta-tool consolidating MCP server
// health/status and update operations. If updater is nil, only the status
// action is available.
func RegisterMCPMeta(server *mcp.Server, client *gitlabclient.Client, updater *autoupdate.Updater) {
	routes := actionMap{
		"status":       routeAction(client, health.Check),
		"health_check": routeAction(client, health.Check),
	}

	desc := `MCP server operations: health check, version info, and update management.
Use this tool to verify GitLab connectivity, check server version, or manage updates.
Do NOT use for GitLab resource operations — use the domain-specific tools instead.
Use 'action' to specify the operation.

Actions:
- status: Check MCP server health and GitLab connectivity. Returns server version, author, department, repository, GitLab version, authentication status, current user, and response time. Params: (none required)
- health_check: Alias for status — check server health. Params: (none required)

See also: gitlab_discover_project (resolve git remote URL to project ID)`

	if updater != nil {
		routes["check_update"] = route(wrapUpdaterAction(updater, serverupdate.Check))
		routes["apply_update"] = destructiveRoute(wrapUpdaterAction(updater, serverupdate.Apply))

		desc = `MCP server operations: health check, version info, and update management.
Use this tool to verify GitLab connectivity, check server version, or manage updates.
Do NOT use for GitLab resource operations — use the domain-specific tools instead.
Use 'action' to specify the operation.

Actions:
- status: Check MCP server health and GitLab connectivity. Returns server version, author, department, repository, GitLab version, authentication status, current user, and response time. Params: (none required)
- health_check: Alias for status — check server health. Params: (none required)
- check_update: Check if a newer version of the MCP server is available. Returns current version, latest version, release URL, and release notes. Params: (none required)
- apply_update: Download and apply the latest MCP server update. On Linux/macOS the binary is replaced atomically. On Windows the update is downloaded to a staging path with an update script. Params: (none required)

See also: gitlab_discover_project (resolve git remote URL to project ID)`
	}
	addMetaTool(server, "gitlab_server", desc, routes, toolutil.IconHealth)
}

// wrapUpdaterAction wraps a function that takes an *autoupdate.Updater (instead
// of *gitlabclient.Client) into an actionFunc for meta-tool dispatch.
func wrapUpdaterAction[T any, R any](updater *autoupdate.Updater, fn func(ctx context.Context, updater *autoupdate.Updater, input T) (R, error)) actionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return fn(ctx, updater, input)
	}
}

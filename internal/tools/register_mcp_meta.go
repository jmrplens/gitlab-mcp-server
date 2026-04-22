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

	desc := `MCP server health, version, and GitLab connectivity check. Read-only, no params required.
Valid actions: ` + validActionsString(routes) + `

When to use: verify GitLab connectivity, check server/GitLab version, diagnose auth issues. NOT for: GitLab resource operations (use domain-specific tools).

- status: Returns server version, GitLab version, auth status, current user, response time. No params.
- health_check: Alias for status. No params.

See also: gitlab_discover_project (resolve git remote URL to project ID)`

	if updater != nil {
		routes["check_update"] = route(wrapUpdaterAction(updater, serverupdate.Check))
		routes["apply_update"] = destructiveRoute(wrapUpdaterAction(updater, serverupdate.Apply))

		desc = `MCP server health, version, update management, and GitLab connectivity check. apply_update replaces the server binary (destructive on Linux/macOS, staged on Windows).
Valid actions: ` + validActionsString(routes) + `

When to use: verify GitLab connectivity, check server/GitLab version, manage server updates. NOT for: GitLab resource operations (use domain-specific tools).

- status: Returns server version, GitLab version, auth status, current user, response time. No params.
- health_check: Alias for status. No params.
- check_update: Check if newer server version is available. Returns current/latest version, release URL, notes. No params.
- apply_update: Download and apply latest server update. Linux/macOS: atomic binary replace. Windows: staged download with update script. No params.

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

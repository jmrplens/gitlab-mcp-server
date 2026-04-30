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

	desc := `MCP server self-diagnostics: GitLab connectivity probe, server/GitLab version, and authenticated user identity. Read-only; no required params.
Valid actions: ` + validActionsString(routes) + `

When to use: at session start to confirm the GitLab token works, when diagnosing 401/403 errors from other tools, or to record server/GitLab versions for support tickets.
NOT for: resolving a git remote URL to a project (use gitlab_discover_project), GitLab instance admin (use gitlab_admin), per-project membership/permissions (use gitlab_project / gitlab_user), CI runner health (use gitlab_runner).

Returns: {status, mcp_server_version, gitlab_url, gitlab_version, gitlab_revision, authenticated (bool), username, user_id, response_time_ms, error}. Authentication and connectivity failures are surfaced inside this diagnostics object (status / error fields), not as a tool-level JSON-RPC error.
Errors: tool-level errors are rare — inspect the returned status / error fields. Network errors include the GitLab URL verbatim.

- status: (no params) — returns the diagnostics object above.
- health_check: alias for status. (no params)

See also: gitlab_discover_project (resolve git remote URL → project_id), gitlab_admin (instance admin), gitlab_user (current user details and impersonation tokens).`

	if updater != nil {
		routes["check_update"] = route(wrapUpdaterAction(updater, serverupdate.Check))
		routes["apply_update"] = destructiveRoute(wrapUpdaterAction(updater, serverupdate.Apply))

		desc = `MCP server self-diagnostics, version, GitLab connectivity probe, and self-update. apply_update REPLACES the running binary on disk; on Linux/macOS the replacement is atomic, on Windows it is staged via a script. Read-only except apply_update (destructive).
Valid actions: ` + validActionsString(routes) + `

When to use: confirm the GitLab token works, check whether a newer server release is available, apply that update without leaving the editor.
NOT for: GitLab instance admin (use gitlab_admin), git remote resolution (use gitlab_discover_project), CI runner health (use gitlab_runner).

Returns:
- status / health_check: {status, mcp_server_version, gitlab_url, gitlab_version, gitlab_revision, authenticated, username, user_id, response_time_ms, error}.
- check_update: {update_available (bool), current_version, latest_version, release_url, release_notes, mode}.
- apply_update: {applied (bool), previous_version, new_version, message}.
Errors: connectivity / auth failures appear inside the diagnostics object (status / error). Update channel errors include the release fetch URL.

- status / health_check: (no params)
- check_update: (no params) — compares current binary version against the configured release feed.
- apply_update: (no params) — downloads and applies the latest server release.

See also: gitlab_discover_project (resolve git remote URL → project_id), gitlab_admin (GitLab instance admin), gitlab_user (current user identity).`
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

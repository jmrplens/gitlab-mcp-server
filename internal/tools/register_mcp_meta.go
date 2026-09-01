package tools

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// RegisterMCPMeta registers the gitlab_server meta-tool carrying MCP server
// health and status. Catalog construction failures are logged and the
// function returns without registering.
func RegisterMCPMeta(server *mcp.Server, client *gitlabclient.Client) {
	catalog := actioncatalog.NewCatalog()
	if err := catalog.AddGroup(BuildMCPActionGroup(client)); err != nil {
		slog.Error("failed to add MCP meta action group", "error", err)
		return
	}
	RegisterMetaCatalog(server, catalog)
}

// BuildMCPActionGroup builds the registry group backing the gitlab_server
// meta-tool. The custom description documents the available actions and their
// semantics for the schema resource.
//
// The group is read-only. It used to gain check_update and apply_update when a
// self-updater was configured, and apply_update was the only destructive action
// this server had that acted on the machine rather than on GitLab. Self-update
// is gone: every distribution channel already owns the binary, and the update
// path was the one place this server fetched and executed code at runtime.
func BuildMCPActionGroup(client *gitlabclient.Client) actioncatalog.Group {
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

	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{
		ToolName:    "gitlab_server",
		Description: desc,
		Icons:       toolutil.IconHealth,
		ReadOnly:    true,
	})
	specActions, specErr := actioncatalog.ActionsFromSpecs(
		actioncompat.ApplyToActionSpecs("gitlab_server", "server", health.ActionSpecs(client)),
	)
	if specErr != nil {
		slog.Error("failed to build MCP health action specs", "error", specErr)
	}
	specActionByName := make(map[string]actioncatalog.Action, len(specActions))
	for _, action := range specActions {
		specActionByName[action.Name] = action
	}
	for name, route := range routes {
		if action, ok := specActionByName[name]; ok {
			group.SetAction(action)
			continue
		}
		group.SetAction(actioncatalog.Action{Name: name, Route: route})
	}
	return group
}

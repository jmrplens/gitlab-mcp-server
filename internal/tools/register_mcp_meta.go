package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterMCPMeta registers the gitlab_server meta-tool consolidating MCP server
// health/status, schema discovery, and update operations. If updater is nil,
// self-update actions are omitted.
func RegisterMCPMeta(server *mcp.Server, client *gitlabclient.Client, updater *autoupdate.Updater, schemaRegistries ...*toolutil.MetaSchemaRegistry) {
	schemaRegistry := toolutil.NewMetaSchemaRegistry(nil)
	if len(schemaRegistries) > 0 && schemaRegistries[0] != nil {
		schemaRegistry = schemaRegistries[0]
	}

	routes := actionMap{
		"status":       routeAction(client, health.Check),
		"health_check": routeAction(client, health.Check),
		"schema_get": routeAction(client, func(_ context.Context, _ *gitlabclient.Client, input schemaGetInput) (any, error) {
			return getMetaSchema(schemaRegistry, input)
		}),
		"schema_index": routeAction(client, func(_ context.Context, _ *gitlabclient.Client, input schemaIndexInput) (any, error) {
			return getMetaSchemaIndex(schemaRegistry, input)
		}),
	}

	desc := `MCP server self-diagnostics, GitLab connectivity probe, server/GitLab version, authenticated user identity, and model-controlled meta-tool schema discovery. Read-only; schema actions expose only currently visible meta-tool routes.
Valid actions: ` + validActionsString(routes) + `

When to use: at session start to confirm the GitLab token works, when diagnosing 401/403 errors from other tools, to record server/GitLab versions for support tickets, or before calling an unfamiliar meta-tool action whose params schema is not in the current context.
NOT for: resolving a git remote URL to a project (use gitlab_discover_project), GitLab instance admin (use gitlab_admin), per-project membership/permissions (use gitlab_project / gitlab_user), CI runner health (use gitlab_runner).

Returns:
- status / health_check: {status, mcp_server_version, gitlab_url, gitlab_version, gitlab_revision, authenticated (bool), username, user_id, response_time_ms, error}. Authentication and connectivity failures are surfaced inside this diagnostics object (status / error fields), not as a tool-level JSON-RPC error.
- schema_index: {uri_template, tool_count, action_count, tools[{tool, action_count, actions[{action, schema_uri, destructive}]}]}.
- schema_get: JSON Schema for one action's params when action is provided; otherwise the schema_index shape filtered to one tool.
Errors: tool-level errors are rare — inspect the returned status / error fields. Network errors include the GitLab URL verbatim.

- status: (no params) — returns the diagnostics object above.
- health_check: alias for status. (no params)
- schema_index: tool — lists visible meta-tools/actions with schema URIs and destructive flags; provide tool to filter to one meta-tool.
- schema_get: tool*, action — returns the params JSON Schema for an action, or one-tool schema index when action is omitted.

See also: gitlab_discover_project (resolve git remote URL → project_id), gitlab_admin (instance admin), gitlab_user (current user details and impersonation tokens).`

	if updater != nil {
		routes["check_update"] = route(wrapUpdaterAction(updater, serverupdate.Check))
		routes["apply_update"] = destructiveRoute(wrapUpdaterAction(updater, serverupdate.Apply))

		desc = `MCP server self-diagnostics, version, GitLab connectivity probe, model-controlled meta-tool schema discovery, and self-update. apply_update REPLACES the running binary on disk; on Linux/macOS the replacement is atomic, on Windows it is staged via a script. Read-only except apply_update (destructive).
Valid actions: ` + validActionsString(routes) + `

When to use: confirm the GitLab token works, fetch exact params schemas for unfamiliar meta-tool actions, check whether a newer server release is available, apply that update without leaving the editor.
NOT for: GitLab instance admin (use gitlab_admin), git remote resolution (use gitlab_discover_project), CI runner health (use gitlab_runner).

Returns:
- status / health_check: {status, mcp_server_version, gitlab_url, gitlab_version, gitlab_revision, authenticated, username, user_id, response_time_ms, error}.
- schema_index: {uri_template, tool_count, action_count, tools[{tool, action_count, actions[{action, schema_uri, destructive}]}]}.
- schema_get: JSON Schema for one action's params when action is provided; otherwise the schema_index shape filtered to one tool.
- check_update: {update_available (bool), current_version, latest_version, release_url, release_notes, mode}.
- apply_update: {applied (bool), previous_version, new_version, message}.
Errors: connectivity / auth failures appear inside the diagnostics object (status / error). Update channel errors include the release fetch URL.

- status / health_check: (no params)
- schema_index: tool — lists visible meta-tools/actions with schema URIs and destructive flags; provide tool to filter to one meta-tool.
- schema_get: tool*, action — returns the params JSON Schema for an action, or one-tool schema index when action is omitted.
- check_update: (no params) — compares current binary version against the configured release feed.
- apply_update: (no params) — downloads and applies the latest server release.

See also: gitlab_discover_project (resolve git remote URL → project_id), gitlab_admin (GitLab instance admin), gitlab_user (current user identity).`
	}
	if updater == nil {
		addReadOnlyMetaTool(server, "gitlab_server", desc, routes, toolutil.IconHealth)
		return
	}
	addMetaTool(server, "gitlab_server", desc, routes, toolutil.IconHealth)
}

type schemaGetInput struct {
	Tool   string `json:"tool" jsonschema:"Meta-tool name, for example gitlab_project or gitlab_merge_request."`
	Action string `json:"action,omitempty" jsonschema:"Optional action name. When provided, returns the JSON Schema for that action's params. When omitted, returns the schema index filtered to the selected tool."`
}

type schemaIndexInput struct {
	Tool string `json:"tool,omitempty" jsonschema:"Optional meta-tool name to filter the schema index, for example gitlab_project or gitlab_merge_request."`
}

func getMetaSchema(registry *toolutil.MetaSchemaRegistry, input schemaGetInput) (any, error) {
	if input.Tool == "" {
		return nil, errors.New("schema_get: tool is required; call schema_index to list visible meta-tools")
	}
	routes := registry.Routes()
	if input.Action == "" {
		index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(routes, input.Tool)
		if !ok {
			return nil, fmt.Errorf("schema_get: unknown tool %q; call schema_index to list visible meta-tools", input.Tool)
		}
		return index, nil
	}
	schema, ok := toolutil.LookupMetaActionSchema(routes, input.Tool, input.Action)
	if !ok {
		return nil, fmt.Errorf("schema_get: unknown action %q for tool %q; call schema_index with {\"tool\":%q} to list valid actions", input.Action, input.Tool, input.Tool)
	}
	return schema, nil
}

func getMetaSchemaIndex(registry *toolutil.MetaSchemaRegistry, input schemaIndexInput) (any, error) {
	routes := registry.Routes()
	if input.Tool != "" {
		index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(routes, input.Tool)
		if !ok {
			return nil, fmt.Errorf("schema_index: unknown tool %q; omit tool to list visible meta-tools", input.Tool)
		}
		return index, nil
	}
	return toolutil.BuildMetaSchemaDiscoveryIndex(routes), nil
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

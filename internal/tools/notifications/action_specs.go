package notifications

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical notification action names. These are the spec names projected
// into every surface; RelatedActions cross-links reference them directly.
const (
	actionGlobalGet     = "notification_global_get"
	actionGlobalUpdate  = "notification_global_update"
	actionProjectGet    = "notification_project_get"
	actionProjectUpdate = "notification_project_update"
	actionGroupGet      = "notification_group_get"
	actionGroupUpdate   = "notification_group_update"
)

// ActionSpecs returns canonical specs for notification settings actions
// exposed as MCP tools. The global, project, and group read/update
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_notification_global_get — read the caller's global notification settings.
		notificationReadSpec(actionGlobalGet, toolutil.RouteAction(client, GetGlobalSettings), "gitlab_notification_global_get"),
		// gitlab_notification_project_get — read project notification settings.
		notificationReadSpec(actionProjectGet, toolutil.RouteAction(client, GetSettingsForProject), "gitlab_notification_project_get"),
		// gitlab_notification_group_get — read group notification settings.
		notificationReadSpec(actionGroupGet, toolutil.RouteAction(client, GetSettingsForGroup), "gitlab_notification_group_get"),
		// gitlab_notification_global_update — update global notification settings.
		notificationUpdateSpec(actionGlobalUpdate, toolutil.RouteAction(client, UpdateGlobalSettings), "gitlab_notification_global_update"),
		// gitlab_notification_project_update — update project notification settings.
		notificationUpdateSpec(actionProjectUpdate, toolutil.RouteAction(client, UpdateSettingsForProject), "gitlab_notification_project_update"),
		// gitlab_notification_group_update — update group notification settings.
		notificationUpdateSpec(actionGroupUpdate, toolutil.RouteAction(client, UpdateSettingsForGroup), "gitlab_notification_group_update"),
	}
}

// notificationReadSpec builds a read-only [toolutil.ActionSpec] for a
// notification action using the package's default [notificationOptions]
// decorated with the action's non-generic discovery metadata.
func notificationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, notificationOptions(name, individualTool))
}

// notificationUpdateSpec builds an update-style [toolutil.ActionSpec]
// for a notification action using the package's default
// [notificationOptions] decorated with the action's non-generic
// discovery metadata.
func notificationUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, notificationOptions(name, individualTool))
}

// notificationOptions returns the base [toolutil.ActionSpecOptions]
// shared by every notification action (tags, owner, individual tool
// metadata) and applies the action-specific Usage, natural-language
// Aliases, RelatedActions cross-links, and "Returns: … See also: …"
// individual-tool description from [notificationActionMeta].
func notificationOptions(name, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"user", "notification"},
		OpenWorld:      true,
		OwnerPackage:   "notifications",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := notificationActionMeta[name]; ok {
		options.Usage = meta.usage
		options.Aliases = append([]string(nil), meta.aliases...)
		options.RelatedActions = append([]string(nil), meta.related...)
		options.IndividualTool.Description = meta.description
	}
	switch name {
	case actionGlobalUpdate, actionProjectUpdate, actionGroupUpdate:
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("level", map[string]any{"enum": []any{"disabled", "participating", "watch", "global", "mention", "custom"}}),
		}
	}
	return options
}

// notificationActionMetaEntry is the discovery metadata for one
// notification settings action.
type notificationActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// notificationActionMeta maps each notification action to its non-generic
// discovery metadata: an action-specific Usage sentence, natural-language
// Aliases, RelatedActions cross-links to the sibling read/update routes,
// and the "Returns: … See also: …" individual-tool Description.
var notificationActionMeta = map[string]notificationActionMetaEntry{
	actionGlobalGet: {
		usage:       "Read the authenticated user's account-wide (global) notification settings. Use when the prompt asks what notifications a user receives by default, or before changing them with the global update action. Requires read_user token scope.",
		aliases:     []string{"get global notification settings", "show my notification settings", "read default notification level"},
		related:     []string{actionGlobalUpdate, actionProjectGet, actionGroupGet},
		description: "Get the authenticated user's global notification settings. Returns: the global notification level, notification email, and the per-event flags (issue, merge request, pipeline, note, and epic events) when the level is custom. See also: gitlab_notification_global_update, gitlab_notification_project_get, gitlab_notification_group_get.",
	},
	actionGlobalUpdate: {
		usage:       "Update the authenticated user's account-wide (global) notification settings. Set level (disabled, participating, watch, global, mention, custom), notification_email, and individual event flags when level is custom. Only the fields you pass are changed.",
		aliases:     []string{"update global notification settings", "set my notification level", "change default notification email"},
		related:     []string{actionGlobalGet, actionProjectUpdate, actionGroupUpdate},
		description: "Update the authenticated user's global notification settings. Returns: the updated global notification level, notification email, and per-event flags. See also: gitlab_notification_global_get, gitlab_notification_project_update, gitlab_notification_group_update.",
	},
	actionProjectGet: {
		usage:       "Read the authenticated user's notification settings for one project. Use when the prompt asks how a user is notified for a specific project, or before overriding them with the project update action. Requires project membership.",
		aliases:     []string{"get project notification settings", "show notification settings for project", "read project notification level"},
		related:     []string{actionProjectUpdate, actionGlobalGet, actionGroupGet},
		description: "Get the authenticated user's notification settings for a project. Returns: the project notification level, notification email, and per-event flags when the level is custom. See also: gitlab_notification_project_update, gitlab_notification_global_get, gitlab_notification_group_get.",
	},
	actionProjectUpdate: {
		usage:       "Update the authenticated user's notification settings for one project. Pass project_id plus level (disabled, participating, watch, global, mention, custom), notification_email, and event flags when level is custom. Only the fields you pass are changed.",
		aliases:     []string{"update project notification settings", "set notification level for project", "override notifications for project"},
		related:     []string{actionProjectGet, actionGlobalUpdate, actionGroupUpdate},
		description: "Update the authenticated user's notification settings for a project. Returns: the updated project notification level, notification email, and per-event flags. See also: gitlab_notification_project_get, gitlab_notification_global_update, gitlab_notification_group_update.",
	},
	actionGroupGet: {
		usage:       "Read the authenticated user's notification settings for one group. Use when the prompt asks how a user is notified for a specific group, or before overriding them with the group update action. Requires group membership.",
		aliases:     []string{"get group notification settings", "show notification settings for group", "read group notification level"},
		related:     []string{actionGroupUpdate, actionGlobalGet, actionProjectGet},
		description: "Get the authenticated user's notification settings for a group. Returns: the group notification level, notification email, and per-event flags when the level is custom. See also: gitlab_notification_group_update, gitlab_notification_global_get, gitlab_notification_project_get.",
	},
	actionGroupUpdate: {
		usage:       "Update the authenticated user's notification settings for one group. Pass group_id plus level (disabled, participating, watch, global, mention, custom), notification_email, and event flags when level is custom. Only the fields you pass are changed.",
		aliases:     []string{"update group notification settings", "set notification level for group", "override notifications for group"},
		related:     []string{actionGroupGet, actionGlobalUpdate, actionProjectUpdate},
		description: "Update the authenticated user's notification settings for a group. Returns: the updated group notification level, notification email, and per-event flags. See also: gitlab_notification_group_get, gitlab_notification_global_update, gitlab_notification_project_update.",
	},
}

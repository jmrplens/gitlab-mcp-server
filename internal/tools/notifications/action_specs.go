package notifications

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for notification settings actions
// exposed as MCP tools. The global, project, and group read/update
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_notification_global_get — read the caller's global notification settings.
		notificationReadSpec("notification_global_get", toolutil.RouteAction(client, GetGlobalSettings), "gitlab_notification_global_get"),
		// gitlab_notification_project_get — read project notification settings.
		notificationReadSpec("notification_project_get", toolutil.RouteAction(client, GetSettingsForProject), "gitlab_notification_project_get"),
		// gitlab_notification_group_get — read group notification settings.
		notificationReadSpec("notification_group_get", toolutil.RouteAction(client, GetSettingsForGroup), "gitlab_notification_group_get"),
		// gitlab_notification_global_update — update global notification settings.
		notificationUpdateSpec("notification_global_update", toolutil.RouteAction(client, UpdateGlobalSettings), "gitlab_notification_global_update"),
		// gitlab_notification_project_update — update project notification settings.
		notificationUpdateSpec("notification_project_update", toolutil.RouteAction(client, UpdateSettingsForProject), "gitlab_notification_project_update"),
		// gitlab_notification_group_update — update group notification settings.
		notificationUpdateSpec("notification_group_update", toolutil.RouteAction(client, UpdateSettingsForGroup), "gitlab_notification_group_update"),
	}
}

// notificationReadSpec builds a read-only [toolutil.ActionSpec] for a
// notification action using the package's default
// [notificationOptions].
func notificationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, notificationOptions(individualTool))
}

// notificationUpdateSpec builds an update-style [toolutil.ActionSpec]
// for a notification action using the package's default
// [notificationOptions].
func notificationUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, notificationOptions(individualTool))
}

// notificationOptions returns the base [toolutil.ActionSpecOptions]
// shared by every notification action (tags, owner, individual tool
// metadata).
func notificationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute notifications domain action.", Tags: []string{"user", "notification"},
		OpenWorld:      true,
		OwnerPackage:   "notifications",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

package notifications

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for notification settings actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		notificationReadSpec("notification_global_get", toolutil.RouteAction(client, GetGlobalSettings), "gitlab_notification_global_get"),
		notificationReadSpec("notification_project_get", toolutil.RouteAction(client, GetSettingsForProject), "gitlab_notification_project_get"),
		notificationReadSpec("notification_group_get", toolutil.RouteAction(client, GetSettingsForGroup), "gitlab_notification_group_get"),
		notificationUpdateSpec("notification_global_update", toolutil.RouteAction(client, UpdateGlobalSettings), "gitlab_notification_global_update"),
		notificationUpdateSpec("notification_project_update", toolutil.RouteAction(client, UpdateSettingsForProject), "gitlab_notification_project_update"),
		notificationUpdateSpec("notification_group_update", toolutil.RouteAction(client, UpdateSettingsForGroup), "gitlab_notification_group_update"),
	}
}

func notificationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, notificationOptions(individualTool))
}

func notificationUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, notificationOptions(individualTool))
}

func notificationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute notifications domain action.", Tags: []string{"user", "notification"},
		OpenWorld:      true,
		OwnerPackage:   "notifications",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

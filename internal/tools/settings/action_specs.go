package settings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for application settings tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		settingsReadSpec("settings_get", toolutil.RouteAction(client, Get), "gitlab_get_settings"),
		settingsUpdateSpec("settings_update", toolutil.RouteAction(client, Update), "gitlab_update_settings"),
	}
}

func settingsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := settingsOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func settingsUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := settingsOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func settingsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "settings"},
		OpenWorld:      true,
		OwnerPackage:   "settings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

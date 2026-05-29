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
	return toolutil.NewReadActionSpec(name, route, settingsOptions(name, individualTool))
}

func settingsUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, settingsOptions(name, individualTool))
}

func settingsOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Get current GitLab application settings."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "settings_update" {
		usage = "Update mutable GitLab application settings through a key-value settings map."
		guidance["settings"] = toolutil.ParameterGuidance{
			SemanticRole:   "settings_patch",
			ValueSource:    "Map of setting keys to desired values (snake_case keys expected by GitLab API).",
			ExampleBinding: `params.settings:{"signup_enabled":false}`,
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"admin", "settings"},
		Usage:             usage,
		RelatedActions:    []string{"admin.metadata_get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "settings",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

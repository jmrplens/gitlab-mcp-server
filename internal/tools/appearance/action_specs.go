package appearance

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for appearance tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		appearanceReadSpec("appearance_get", toolutil.RouteAction(client, Get), "gitlab_get_appearance"),
		appearanceUpdateSpec("appearance_update", toolutil.RouteAction(client, Update), "gitlab_update_appearance"),
	}
}

func appearanceReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := appearanceOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func appearanceUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := appearanceOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func appearanceOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "appearance"},
		OpenWorld:      true,
		OwnerPackage:   "appearance",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

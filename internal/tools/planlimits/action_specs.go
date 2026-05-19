package planlimits

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for plan limit tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		planLimitReadSpec("plan_limits_get", toolutil.RouteAction(client, Get), "gitlab_get_plan_limits"),
		planLimitUpdateSpec("plan_limits_change", toolutil.RouteAction(client, Change), "gitlab_change_plan_limits"),
	}
}

func planLimitReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := planLimitOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func planLimitUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := planLimitOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func planLimitOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "plan-limit"},
		OpenWorld:      true,
		OwnerPackage:   "planlimits",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

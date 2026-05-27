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
	return toolutil.NewReadActionSpec(name, route, planLimitOptions(name, individualTool))
}

func planLimitUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, planLimitOptions(name, individualTool))
}

func planLimitOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Get plan limits for the default or requested plan."
	guidance := map[string]toolutil.ParameterGuidance{
		"plan_name": {
			SemanticRole:   "plan_name",
			ValueSource:    "Optional plan name (for example default) when querying/updating plan limits.",
			ExampleBinding: `params.plan_name:"default"`,
		},
	}
	if actionName == "plan_limits_change" {
		usage = "Update plan limit values for a specific plan."
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"admin", "plan-limit"},
		Usage:             usage,
		RelatedActions:    []string{"admin.settings_get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "planlimits",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

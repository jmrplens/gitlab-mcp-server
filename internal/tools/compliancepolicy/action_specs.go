package compliancepolicy

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for compliance policy setting actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		compliancePolicyReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_compliance_policy_settings"),
		compliancePolicyUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_update_compliance_policy_settings"),
	}
}

func compliancePolicyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, compliancePolicyOptions(individualTool))
}

func compliancePolicyUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, compliancePolicyOptions(individualTool))
}

func compliancePolicyOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"compliance", "policy"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "compliancepolicy",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

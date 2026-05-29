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
	return toolutil.NewReadActionSpec(name, route, compliancePolicyOptions(name, individualTool))
}

func compliancePolicyUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, compliancePolicyOptions(name, individualTool))
}

func compliancePolicyOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Get current compliance policy settings."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "update" {
		usage = "Update compliance policy settings for the instance."
		guidance["csp_namespace_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "compliance_namespace_id",
			ValueSource:    "Namespace ID that should host compliance policy project(s).",
			ExampleBinding: "params.csp_namespace_id:200",
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"compliance", "policy"},
		Usage:             usage,
		RelatedActions:    []string{"group.get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		Edition:           "premium",
		OwnerPackage:      "compliancepolicy",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

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
	usage := "Read the instance-level compliance policy settings to discover which namespace hosts the centralized compliance security policy (CSP) project. Use this before updating to confirm the current CSP namespace binding."
	aliases := []string{
		individualTool,
		"get compliance policy settings",
		"show compliance framework settings",
		"view csp namespace",
		"read security policy settings",
		"compliance policy configuration",
	}
	guidance := map[string]toolutil.ParameterGuidance{}
	description := "Get the instance compliance policy settings. Returns: the centralized compliance security policy (CSP) namespace ID, or unset when no namespace hosts the compliance policy project. See also: gitlab_update_compliance_policy_settings, gitlab_group_get."
	related := []string{"compliance_policy.update", "group.get"}

	if actionName == "update" {
		usage = "Bind the centralized compliance security policy (CSP) project to a top-level group namespace so its compliance frameworks and security policies apply instance-wide. Requires an Ultimate license and the Owner role; GitLab may lock CSP namespace changes for several minutes after each update."
		aliases = []string{
			individualTool,
			"set csp namespace",
			"update compliance framework settings",
			"bind compliance security policy project",
			"change compliance policy namespace",
			"configure security policy namespace",
		}
		guidance["csp_namespace_id"] = toolutil.ParameterGuidance{
			SemanticRole:     "compliance_namespace_id",
			ValueSource:      "ID of an existing top-level group namespace that should host the compliance security policy (CSP) project.",
			ExampleBinding:   "params.csp_namespace_id:200",
			CommonConfusions: []string{"Use a top-level group namespace ID, not a project ID or a subgroup; GitLab rejects empty update requests."},
		}
		description = "Update the instance compliance policy settings. Returns: the compliance policy settings with the newly bound centralized compliance security policy (CSP) namespace ID. See also: gitlab_get_compliance_policy_settings, gitlab_group_get."
		related = []string{"compliance_policy.get", "group.get"}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"compliance", "policy"},
		Usage:             usage,
		RelatedActions:    related,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		Edition:           "premium",
		OwnerPackage:      "compliancepolicy",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}

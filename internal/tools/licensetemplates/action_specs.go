package licensetemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for license template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		licenseTemplateSpec("license_list", toolutil.RouteAction(client, List), "gitlab_list_license_templates"),
		licenseTemplateSpec("license_get", toolutil.RouteAction(client, Get), "gitlab_get_license_template"),
	}
}

func licenseTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, licenseTemplateOptions(name, individualTool))
}

func licenseTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "List available license templates."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "license_get" {
		usage = "Get one license template by key for project README/LICENSE scaffolding."
		guidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "template_key",
			ValueSource:    "License key returned by license template list output.",
			ExampleBinding: `params.key:"mit"`,
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"template", "license"},
		Usage:             usage,
		RelatedActions:    []string{"repository.file_create", "project.create"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "licensetemplates",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

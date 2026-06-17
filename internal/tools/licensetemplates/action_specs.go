package licensetemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for license template actions
// exposed as MCP tools. The list and get routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_license_templates — list available license templates.
		licenseTemplateSpec("license_list", toolutil.RouteAction(client, List), "gitlab_list_license_templates"),
		// gitlab_get_license_template — fetch a single license template by key.
		licenseTemplateSpec("license_get", toolutil.RouteAction(client, Get), "gitlab_get_license_template"),
	}
}

// licenseTemplateSpec builds a read-only [toolutil.ActionSpec] for a
// license template action using the package's default
// [licenseTemplateOptions].
func licenseTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, licenseTemplateOptions(name, individualTool))
}

// licenseTemplateOptions returns the base [toolutil.ActionSpecOptions]
// for a license template action and customizes the Usage/ParameterGuidance
// for the get individual tool.
func licenseTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
		OpenWorld:      true,
		OwnerPackage:   "licensetemplates",
		Tags:           []string{"template", "license"},
		Aliases:        []string{individualTool},
		RelatedActions: []string{"repository.file_create", "project.create"},
		Usage:          "List available license templates.",
	}
	if actionName == "license_get" {
		opts.Usage = "Get one license template by key for project README/LICENSE scaffolding."
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {SemanticRole: "template_key", ValueSource: "License key returned by license template list output.", ExampleBinding: `params.key:"mit"`},
		}
	}
	return opts
}

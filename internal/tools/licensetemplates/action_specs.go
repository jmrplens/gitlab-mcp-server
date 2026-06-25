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
		RelatedActions: []string{"licensetemplates.license_get", "repository.file_create", "project.create"},
		Usage:          "List available license templates with optional popular filter, ordering, and keyset pagination.",
	}
	if actionName == "license_get" {
		opts.Usage = "Get one license template by key for project README/LICENSE scaffolding."
		opts.RelatedActions = []string{"licensetemplates.license_list", "repository.file_create", "project.create"}
		opts.IndividualTool.Description = "Get a single license template by key, optionally substituting project and fullname placeholders. Returns: the license's key, name, nickname, featured flag, source/HTML URLs, description, conditions, permissions, limitations, and rendered content. See also: gitlab_list_license_templates, gitlab_create_file."
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key":      {SemanticRole: "template_key", ValueSource: "License key returned by license template list output.", ExampleBinding: `params.key:"mit"`},
			"project":  {SemanticRole: "placeholder_project", ValueSource: "Project name substituted into the rendered license content placeholder.", ExampleBinding: `params.project:"my-project"`},
			"fullname": {SemanticRole: "placeholder_fullname", ValueSource: "Full name substituted into the rendered license copyright placeholder.", ExampleBinding: `params.fullname:"Jane Doe"`},
		}
		return opts
	}
	opts.IndividualTool.Description = "List available license templates with an optional popular filter, order_by/sort, and offset or keyset pagination. Returns: each template's key, name, nickname, featured flag, source/HTML URLs, description, conditions, permissions, limitations, and content, with pagination metadata. See also: gitlab_get_license_template, gitlab_create_file."
	opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"popular":  {SemanticRole: "popular_filter", ValueSource: "Set true to restrict the listing to popular/featured license templates.", ExampleBinding: `params.popular:true`},
		"order_by": {SemanticRole: "order_by", ValueSource: "Column by which to order keyset-paginated results.", ExampleBinding: `params.order_by:"name"`},
		"sort":     {SemanticRole: "sort", ValueSource: "Sort direction for ordered results.", ExampleBinding: `params.sort:"asc"`},
	}
	return opts
}

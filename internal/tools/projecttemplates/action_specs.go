package projecttemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectTemplateSpec("project_template_list", toolutil.RouteAction(client, List), "gitlab_list_project_templates"),
		projectTemplateSpec("project_template_get", toolutil.RouteAction(client, Get), "gitlab_get_project_template"),
	}
}

func projectTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectTemplateOptions(name, individualTool))
}

// templateTypeGuidance documents the canonical template-type enum once so both
// the list and get actions surface the same authoritative value list.
const templateTypeGuidance = "Template family, one of: dockerfiles, gitignores, gitlab_ci_ymls, licenses, issues, merge_requests."

func projectTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
		OpenWorld:      true,
		OwnerPackage:   "projecttemplates",
		Tags:           []string{"template", "project"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path whose template namespace is queried.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"template_type": {
				SemanticRole:   "template_type",
				ValueSource:    templateTypeGuidance,
				ExampleBinding: `params.template_type:"licenses"`,
			},
		},
	}
	if actionName == "project_template_get" {
		opts.Usage = "Get one project-scoped template (license, gitignore, Dockerfile, .gitlab-ci.yml) by template_type and key, returning its full rendered content."
		opts.Aliases = []string{
			individualTool,
			"show project template",
			"view license template",
			"get gitignore template",
			"read dockerfile template",
		}
		opts.RelatedActions = []string{"projecttemplates.project_template_list", "project.create", "project.get"}
		opts.IndividualTool.Description = "Returns one project template's full content by template_type and key. See also: gitlab_list_project_templates, gitlab_create_project."
		opts.ParameterGuidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "template_key",
			ValueSource:    "Template key from project template list output (e.g. mit, apache-2.0, Go, Python).",
			ExampleBinding: `params.key:"mit"`,
		}
		return opts
	}

	opts.Usage = "List the project-scoped templates of a given template_type (licenses, gitignores, dockerfiles, gitlab_ci_ymls), with optional id/type filters, ordering, and keyset pagination."
	opts.Aliases = []string{
		individualTool,
		"list project templates",
		"available license templates",
		"available gitignore templates",
		"list dockerfile templates",
		"ci yaml templates",
	}
	opts.RelatedActions = []string{"projecttemplates.project_template_get", "project.create", "project.list"}
	opts.IndividualTool.Description = "Lists project templates of a given template_type with optional id/type filters, order_by/sort, and offset or keyset pagination. See also: gitlab_get_project_template, gitlab_create_project."
	opts.ParameterGuidance["type"] = toolutil.ParameterGuidance{
		SemanticRole:   "template_type_filter",
		ValueSource:    "Optional secondary filter passed as the 'type' query parameter; " + templateTypeGuidance,
		ExampleBinding: `params.type:"licenses"`,
	}
	opts.ParameterGuidance["id"] = toolutil.ParameterGuidance{
		SemanticRole:   "template_id_filter",
		ValueSource:    "Optional numeric template id to fetch a single template entry.",
		ExampleBinding: `params.id:42`,
	}
	opts.ParameterGuidance["order_by"] = toolutil.ParameterGuidance{
		SemanticRole:   "order_by",
		ValueSource:    "Column by which to order keyset-paginated results.",
		ExampleBinding: `params.order_by:"name"`,
	}
	opts.ParameterGuidance["sort"] = toolutil.ParameterGuidance{
		SemanticRole:   "sort",
		ValueSource:    "Sort direction for ordered results.",
		ExampleBinding: `params.sort:"asc"`,
	}
	return opts
}

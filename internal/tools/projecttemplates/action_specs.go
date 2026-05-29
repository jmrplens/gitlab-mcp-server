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

func projectTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
		OpenWorld:      true,
		OwnerPackage:   "projecttemplates",
		Tags:           []string{"template", "project"},
		Aliases:        []string{individualTool},
		RelatedActions: []string{"project.create", "project.list"},
		Usage:          "List project-scoped templates by template_type.",
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path whose template namespace is queried.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"template_type": {
				SemanticRole:   "template_type",
				ValueSource:    "Template family, such as licenses, gitignores, or dockerfiles.",
				ExampleBinding: `params.template_type:"licenses"`,
			},
		},
	}
	if actionName == "project_template_get" {
		opts.Usage = "Get one project-scoped template by template_type and key."
		opts.ParameterGuidance["key"] = toolutil.ParameterGuidance{
			SemanticRole:   "template_key",
			ValueSource:    "Template key from project template list output.",
			ExampleBinding: `params.key:"mit"`,
		}
	}
	return opts
}

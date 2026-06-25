package dockerfiletemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Dockerfile template actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		dockerfileTemplateSpec("dockerfile_list", toolutil.RouteAction(client, List), "gitlab_list_dockerfile_templates"),
		dockerfileTemplateSpec("dockerfile_get", toolutil.RouteAction(client, Get), "gitlab_get_dockerfile_template"),
	}
}

func dockerfileTemplateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, dockerfileTemplateOptions(name, individualTool))
}

func dockerfileTemplateOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	if actionName == "dockerfile_get" {
		return toolutil.ActionSpecOptions{
			Aliases: []string{
				individualTool,
				"fetch Dockerfile template content by key",
				"show GitLab-provided Dockerfile boilerplate",
				"scaffold a container image build file from a template",
			},
			Tags:  []string{"template", "dockerfile"},
			Usage: "Fetch the full Dockerfile boilerplate for one template key (e.g. \"Go\", \"Python\") to scaffold a project's container image build file.",
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"key": {SemanticRole: "template_key", ValueSource: "Template key returned by dockerfile template list output.", ExampleBinding: `params.key:"Go"`},
			},
			RelatedActions: []string{"dockerfiletemplates.dockerfile_list", "repository.file_create"},
			OpenWorld:      true,
			OwnerPackage:   "dockerfiletemplates",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        individualTool,
				Title:       toolutil.TitleFromName(individualTool),
				Description: "Get a single Dockerfile template by key. Returns: the template name and full Dockerfile content ready to commit as a project's Dockerfile. See also: gitlab_list_dockerfile_templates, gitlab_get_gitignore_template.",
			},
		}
	}
	return toolutil.ActionSpecOptions{
		Aliases: []string{
			individualTool,
			"browse available Dockerfile boilerplate templates",
			"discover Dockerfile template keys for scaffolding",
			"list GitLab-provided container image build templates",
		},
		Tags:           []string{"template", "dockerfile"},
		Usage:          "Browse the catalog of GitLab-provided Dockerfile templates to discover template keys before fetching one for container image scaffolding.",
		RelatedActions: []string{"dockerfiletemplates.dockerfile_get", "repository.file_create"},
		OpenWorld:      true,
		OwnerPackage:   "dockerfiletemplates",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "List available Dockerfile templates with pagination and sorting. Returns: matching templates with key and name plus pagination metadata. See also: gitlab_get_dockerfile_template, gitlab_list_gitignore_templates.",
		},
	}
}

package ciyamltemplates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for CI/CD YAML pipeline template actions.
// Each action carries action-specific discovery metadata (usage, natural-language
// aliases, related actions, and an individual-tool description) so the dynamic,
// meta, and individual surfaces expose distinctive guidance for the GitLab-provided
// .gitlab-ci.yml starter templates, distinct from gitignore/dockerfile/license
// templates and the CI lint / CI catalog domains.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		ciYMLTemplateListSpec(toolutil.RouteAction(client, List)),
		ciYMLTemplateGetSpec(toolutil.RouteAction(client, Get)),
	}
}

// ciYMLTemplateListSpec builds the read-only spec for listing the GitLab-provided
// CI/CD pipeline YAML templates.
func ciYMLTemplateListSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := ciYMLTemplateOptions("gitlab_list_ci_yml_templates",
		"List the GitLab-provided CI/CD pipeline YAML templates. Returns: available templates with key and name, plus pagination metadata. See also: gitlab_get_ci_yml_template, gitlab_ci_lint, gitlab_file_create.")
	options.Usage = "List the GitLab-provided starter CI/CD pipeline YAML templates (the presets offered when creating a new .gitlab-ci.yml). Use when the prompt asks which built-in CI pipeline templates exist, or to discover a template key before fetching its contents with template.ci_yml_get."
	options.Aliases = []string{"list gitlab ci pipeline templates", "show built-in .gitlab-ci.yml starter templates", "browse ci/cd yaml pipeline presets"}
	options.RelatedActions = []string{"template.ci_yml_get", "template.lint", "repository.file_create"}
	return toolutil.NewReadActionSpec("ci_yml_list", route, options)
}

// ciYMLTemplateGetSpec builds the read-only spec for fetching one GitLab-provided
// CI/CD pipeline YAML template by key.
func ciYMLTemplateGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := ciYMLTemplateOptions("gitlab_get_ci_yml_template",
		"Get a single GitLab CI/CD pipeline YAML template by key. Returns: the template name and full YAML content to copy into a .gitlab-ci.yml file. See also: gitlab_list_ci_yml_templates, gitlab_ci_lint, gitlab_file_create.")
	options.Usage = "Fetch one GitLab-provided CI/CD pipeline YAML template by its key (for example Android, Go, or Docker) and return the full .gitlab-ci.yml content. Use after template.ci_yml_list reveals the key, or when the prompt already names the pipeline template to scaffold."
	options.Aliases = []string{"get gitlab ci pipeline template by key", "fetch a built-in .gitlab-ci.yml template", "scaffold ci/cd yaml from a preset"}
	options.RelatedActions = []string{"template.ci_yml_list", "template.lint", "repository.file_create"}
	return toolutil.NewReadActionSpec("ci_yml_get", route, options)
}

// ciYMLTemplateOptions seeds the shared ActionSpecOptions fields (tags, ownership,
// and individual-tool projection) common to both CI/CD YAML template actions.
func ciYMLTemplateOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:         []string{"template", "ci"},
		OpenWorld:    true,
		OwnerPackage: "ciyamltemplates",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}

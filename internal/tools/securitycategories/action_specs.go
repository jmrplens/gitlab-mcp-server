package securitycategories

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	descriptionCreateSecurityCategory = "Create a GitLab security category in a namespace via GraphQL. Requires Premium or Ultimate. Returns: created security category. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate"
	descriptionUpdateSecurityCategory = "Update a GitLab security category name or description via GraphQL. Requires Premium or Ultimate. Returns: updated security category. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate"
	descriptionDeleteSecurityCategory = "Delete a GitLab security category and its associated security attributes via GraphQL. Requires Premium or Ultimate. Returns: deletion confirmation. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy"
)

// ActionSpecs returns canonical specs for security category actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		securityCategoryCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_security_category", descriptionCreateSecurityCategory),
		securityCategoryUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_update_security_category", descriptionUpdateSecurityCategory),
		securityCategoryDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_security_category", descriptionDeleteSecurityCategory),
	}
}

func securityCategoryCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.Usage = "Create a security category before creating security attributes under it."
	return toolutil.NewActionSpec(name, route, options)
}

func securityCategoryUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.Idempotent = true
	options.Usage = "Update editable custom security category metadata."
	return toolutil.NewActionSpec(name, route, options)
}

func securityCategoryDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.Destructive = true
	options.Idempotent = true
	options.Usage = "Delete a custom security category and its associated attributes."
	return toolutil.NewActionSpec(name, route, options)
}

func securityCategoryOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"security", "category", "graphql", "namespace"},
		RelatedActions: []string{"security_attribute.create", "security_attribute.update", "group.get", "project.get"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "securitycategories",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

package securitycategories

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	descriptionCreateSecurityCategory = "Create a GitLab security category in a namespace via GraphQL. Requires Premium or Ultimate. Returns: created security category. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate"
	descriptionUpdateSecurityCategory = "Update a GitLab security category name or description via GraphQL. Requires Premium or Ultimate. Returns: updated security category. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate"
	descriptionDeleteSecurityCategory = "Delete a GitLab security category and its associated security attributes via GraphQL. Requires Premium or Ultimate. Returns: deletion confirmation. See also: gitlab_security_attribute, gitlab_group, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy"
)

// ActionSpecs returns canonical specs for security category actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_create_security_category — create a new security category in a namespace.
		securityCategoryCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_security_category", descriptionCreateSecurityCategory),
		// gitlab_update_security_category — rename or update the description of an existing security category.
		securityCategoryUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_update_security_category", descriptionUpdateSecurityCategory),
		// gitlab_delete_security_category — delete a security category and its associated attributes.
		securityCategoryDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_security_category", descriptionDeleteSecurityCategory),
	}
}

// securityCategoryCreateSpec builds the canonical create spec for a security category tool.
func securityCategoryCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("name", map[string]any{"minLength": 1}),
		toolutil.SchemaPropertyOverride("description", map[string]any{"type": "string"}),
		toolutil.SchemaPropertyOverride("multiple_selection", map[string]any{"type": "boolean"}),
	}
	options.Usage = "Create a security category before creating security attributes under it."
	return toolutil.NewCreateActionSpec(name, route, options)
}

// securityCategoryUpdateSpec builds the canonical update spec for a security category tool.
func securityCategoryUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("name", "description"),
		toolutil.SchemaPropertyOverride("name", map[string]any{"type": "string", "minLength": 1}),
		toolutil.SchemaPropertyOverride("description", map[string]any{"type": "string"}),
	}
	options.Usage = "Update editable custom security category metadata."
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// securityCategoryDeleteSpec builds the canonical destructive delete spec for a security category tool.
func securityCategoryDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityCategoryOptions(individualTool, description)
	options.Usage = "Delete a custom security category and its associated attributes."
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func securityCategoryOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute securitycategories domain action.", Tags: []string{"security", "category", "graphql", "namespace"},
		RelatedActions: []string{"security_attribute.create", "security_attribute.update", "group.get", "project.get"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "securitycategories",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

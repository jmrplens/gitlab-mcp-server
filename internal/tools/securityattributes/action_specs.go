package securityattributes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	descriptionCreateSecurityAttribute        = "Create one or more GitLab security attributes under a security category via GraphQL. Requires Premium or Ultimate. Returns: created security attributes and their categories. See also: gitlab_security_category, gitlab_project, gitlab_group. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate"
	descriptionUpdateSecurityAttribute        = "Update a GitLab security attribute name, description, or color via GraphQL. Requires Premium or Ultimate. Returns: updated security attribute metadata. See also: gitlab_security_category, gitlab_project, gitlab_group. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate"
	descriptionDeleteSecurityAttribute        = "Delete a GitLab security attribute via GraphQL. Requires Premium or Ultimate. Returns: deletion confirmation. See also: gitlab_security_category, gitlab_project, gitlab_group. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributedestroy"
	descriptionUpdateProjectSecurityAttribute = "Add or remove GitLab security attributes on a project via GraphQL. Requires Premium or Ultimate. Returns: project security attribute assignments. See also: gitlab_security_attribute, gitlab_project. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate"
	descriptionBulkUpdateSecurityAttributes   = "Add, remove, or replace GitLab security attributes on multiple groups and projects via GraphQL. Requires Premium or Ultimate. Returns: bulk update status, execution mode, and selected target/attribute IDs. See also: gitlab_security_attribute, gitlab_project, gitlab_group. API docs: https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes"
)

// ActionSpecs returns canonical specs for security attribute actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_create_security_attribute — create one or more security attributes under a category.
		securityAttributeCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_security_attribute", descriptionCreateSecurityAttribute),
		// gitlab_update_security_attribute — rename, re-describe, or recolor an editable security attribute.
		securityAttributeUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_update_security_attribute", descriptionUpdateSecurityAttribute),
		// gitlab_delete_security_attribute — delete an editable custom security attribute.
		securityAttributeDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_security_attribute", descriptionDeleteSecurityAttribute),
		// gitlab_update_project_security_attributes — add or remove security attributes on a project.
		securityAttributeProjectUpdateSpec("project_update", toolutil.RouteAction(client, ProjectUpdate), "gitlab_update_project_security_attributes", descriptionUpdateProjectSecurityAttribute),
		// gitlab_bulk_update_security_attributes — add, remove, or replace security attributes across many groups and projects.
		securityAttributeBulkUpdateSpec("bulk_update", toolutil.DestructiveAction(client, BulkUpdate), "gitlab_bulk_update_security_attributes", descriptionBulkUpdateSecurityAttributes),
	}
}

// securityAttributeCreateSpec builds the canonical create spec for a security attribute tool.
func securityAttributeCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("attributes", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("attributes.name", map[string]any{"minLength": 1}),
		toolutil.SchemaPropertyOverride("attributes.description", map[string]any{"minLength": 1}),
		toolutil.SchemaPropertyOverride("attributes.color", map[string]any{"pattern": hexColorSchemaPattern}),
	}
	options.Usage = "Create one or more security attribute values under an existing security category, supplying namespace_id, category_id, and each attribute's name, description, and hex color. Use this to define new classification labels (for example business-impact tiers) before assigning them to projects."
	options.Aliases = []string{"create security attribute", "add security attribute value", "define security classification label", "new security attribute under category"}
	options.RelatedActions = []string{"security_category.create", "security_attribute.project_update", "security_attribute.bulk_update", "project.get"}
	return toolutil.NewCreateActionSpec(name, route, options)
}

// securityAttributeUpdateSpec builds the canonical update spec for a security attribute tool.
func securityAttributeUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("name", "description", "color"),
		toolutil.SchemaPropertyOverride("name", map[string]any{"type": "string", "minLength": 1}),
		toolutil.SchemaPropertyOverride("description", map[string]any{"type": "string"}),
		toolutil.SchemaPropertyOverride("color", map[string]any{"type": "string", "pattern": hexColorSchemaPattern}),
	}
	options.Usage = "Rename, re-describe, or recolor an editable custom security attribute by attribute_id. Provide at least one of name, description, or color; template-provided attributes are not editable."
	options.Aliases = []string{"update security attribute", "rename security attribute", "recolor security attribute", "edit security classification label"}
	options.RelatedActions = []string{"security_attribute.create", "security_attribute.delete", "security_category.update", "project.get"}
	return toolutil.NewUpdateActionSpec(name, route, options)
}

// securityAttributeDeleteSpec builds the canonical destructive delete spec for a security attribute tool.
func securityAttributeDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Usage = "Permanently delete an editable custom security attribute by attribute_id. This destructive action removes the attribute and unassigns it from every project that uses it; template-provided attributes cannot be deleted."
	options.Aliases = []string{"delete security attribute", "remove security attribute value", "destroy security classification label"}
	options.RelatedActions = []string{"security_attribute.create", "security_attribute.update", "security_category.delete", "security_attribute.project_update"}
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// securityAttributeProjectUpdateSpec builds the canonical update spec for adding/removing attributes on a project.
func securityAttributeProjectUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("add_attribute_ids", "remove_attribute_ids"),
		toolutil.SchemaPropertyOverride("add_attribute_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("remove_attribute_ids", map[string]any{"type": "array", "minItems": 1}),
	}
	options.Usage = "Assign or unassign existing security attributes on a single project by project_id, supplying add_attribute_ids, remove_attribute_ids, or both. Use this to classify one project; for many targets at once use security_attribute.bulk_update instead."
	options.Aliases = []string{"assign security attributes to project", "tag project with security attribute", "remove security attribute from project", "classify project security attributes"}
	options.RelatedActions = []string{"security_attribute.bulk_update", "security_attribute.create", "project.get", "security_category.create"}
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// securityAttributeBulkUpdateSpec builds the canonical destructive spec for bulk security attribute updates.
func securityAttributeBulkUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("group_ids", "project_ids"),
		toolutil.SchemaPropertyOverride("group_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("project_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("attribute_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("mode", map[string]any{"enum": []string{string(BulkUpdateModeAdd), string(BulkUpdateModeRemove), string(BulkUpdateModeReplace)}}),
	}
	options.Usage = "Add, remove, or replace security attributes across many groups and projects in one request. Provide attribute_ids, at least one of group_ids or project_ids, and a mode of ADD, REMOVE, or REPLACE (REPLACE swaps the full set on each target). Use this for fleet-wide classification rather than the single-project security_attribute.project_update."
	options.Aliases = []string{"bulk assign security attributes", "mass tag projects with security attributes", "apply security attributes across groups and projects", "fleet-wide security classification"}
	options.RelatedActions = []string{"security_attribute.project_update", "security_attribute.create", "group.get", "project.get"}
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func securityAttributeOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute securityattributes domain action.", Tags: []string{"security", "attribute", "graphql", "namespace"},
		RelatedActions: []string{"security_category.create", "security_category.update", "project.get", "group.get"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "securityattributes",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

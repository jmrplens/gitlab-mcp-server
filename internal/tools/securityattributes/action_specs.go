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
		securityAttributeCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_security_attribute", descriptionCreateSecurityAttribute),
		securityAttributeUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_update_security_attribute", descriptionUpdateSecurityAttribute),
		securityAttributeDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_security_attribute", descriptionDeleteSecurityAttribute),
		securityAttributeProjectUpdateSpec("project_update", toolutil.RouteAction(client, ProjectUpdate), "gitlab_update_project_security_attributes", descriptionUpdateProjectSecurityAttribute),
		securityAttributeBulkUpdateSpec("bulk_update", toolutil.DestructiveAction(client, BulkUpdate), "gitlab_bulk_update_security_attributes", descriptionBulkUpdateSecurityAttributes),
	}
}

func securityAttributeCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("attributes", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("attributes.name", map[string]any{"minLength": 1}),
		toolutil.SchemaPropertyOverride("attributes.description", map[string]any{"minLength": 1}),
		toolutil.SchemaPropertyOverride("attributes.color", map[string]any{"pattern": hexColorSchemaPattern}),
	}
	options.Usage = "Create security attributes under an existing security category."
	return toolutil.NewCreateActionSpec(name, route, options)
}

func securityAttributeUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("name", "description", "color"),
		toolutil.SchemaPropertyOverride("name", map[string]any{"type": "string", "minLength": 1}),
		toolutil.SchemaPropertyOverride("description", map[string]any{"type": "string"}),
		toolutil.SchemaPropertyOverride("color", map[string]any{"type": "string", "pattern": hexColorSchemaPattern}),
	}
	options.Usage = "Update security attribute metadata."
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func securityAttributeDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Usage = "Delete an editable custom security attribute."
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func securityAttributeProjectUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("add_attribute_ids", "remove_attribute_ids"),
		toolutil.SchemaPropertyOverride("add_attribute_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("remove_attribute_ids", map[string]any{"type": "array", "minItems": 1}),
	}
	options.Usage = "Add or remove security attributes on a project."
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func securityAttributeBulkUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaAnyOfRequired("group_ids", "project_ids"),
		toolutil.SchemaPropertyOverride("group_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("project_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("attribute_ids", map[string]any{"type": "array", "minItems": 1}),
		toolutil.SchemaPropertyOverride("mode", map[string]any{"enum": []string{string(BulkUpdateModeAdd), string(BulkUpdateModeRemove), string(BulkUpdateModeReplace)}}),
	}
	options.Usage = "Apply security attributes to many groups and projects in one request."
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

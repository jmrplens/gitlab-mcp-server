package securityattributes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options.Usage = "Create security attributes under an existing security category."
	return toolutil.NewActionSpec(name, route, options)
}

func securityAttributeUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Idempotent = true
	options.Usage = "Update security attribute metadata or assignments."
	return toolutil.NewActionSpec(name, route, options)
}

func securityAttributeDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Destructive = true
	options.Idempotent = true
	options.Usage = "Delete an editable custom security attribute."
	return toolutil.NewActionSpec(name, route, options)
}

func securityAttributeProjectUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Destructive = true
	options.Idempotent = true
	options.Usage = "Add or remove security attributes on a project."
	return toolutil.NewActionSpec(name, route, options)
}

func securityAttributeBulkUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	options := securityAttributeOptions(individualTool, description)
	options.Destructive = true
	options.Idempotent = true
	options.Usage = "Apply security attributes to many groups and projects in one request."
	return toolutil.NewActionSpec(name, route, options)
}

func securityAttributeOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"security", "attribute", "graphql", "namespace"},
		RelatedActions: []string{"security_category.create", "security_category.update", "project.get", "group.get"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "securityattributes",
		ContentKind:    toolutil.ActionSpecContentMutate,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

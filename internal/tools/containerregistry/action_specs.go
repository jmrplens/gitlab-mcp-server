package containerregistry

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for container registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		registryReadSpec("registry_list_project", toolutil.RouteAction(client, ListProject), "gitlab_registry_list_project"),
		registryReadSpec("registry_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_registry_list_group"),
		registryReadSpec("registry_get", toolutil.RouteAction(client, GetRepository), "gitlab_registry_get_repository"),
		registryDeleteSpec("registry_delete", toolutil.DestructiveVoidAction(client, DeleteRepository), "gitlab_registry_delete_repository"),
		registryReadSpec("registry_tag_list", toolutil.RouteAction(client, ListTags), "gitlab_registry_list_tags"),
		registryReadSpec("registry_tag_get", toolutil.RouteAction(client, GetTag), "gitlab_registry_get_tag"),
		registryDeleteSpec("registry_tag_delete", toolutil.DestructiveVoidAction(client, DeleteTag), "gitlab_registry_delete_tag"),
		registryDeleteSpec("registry_tag_delete_bulk", toolutil.DestructiveVoidAction(client, DeleteTagsBulk), "gitlab_registry_delete_tags_bulk"),
		registryReadSpec("registry_rule_list", toolutil.RouteAction(client, ListProtectionRules), "gitlab_registry_protection_list"),
		registryCreateSpec("registry_rule_create", toolutil.RouteAction(client, CreateProtectionRule), "gitlab_registry_protection_create"),
		registryUpdateSpec("registry_rule_update", toolutil.RouteAction(client, UpdateProtectionRule), "gitlab_registry_protection_update"),
		registryDeleteSpec("registry_rule_delete", toolutil.DestructiveVoidAction(client, DeleteProtectionRule), "gitlab_registry_protection_delete"),
	}
}

func registryReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := registryOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func registryCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, registryOptions(individualTool))
}

func registryUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := registryOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func registryDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := registryOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func registryOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"container", "package", "registry"},
		OpenWorld:      true,
		OwnerPackage:   "containerregistry",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

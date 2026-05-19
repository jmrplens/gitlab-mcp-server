package containerregistry

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for container registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		registryReadSpec("registry_list_project", toolutil.RouteAction(client, ListProject), "gitlab_registry_list_project"),
		registryReadSpec("registry_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_registry_list_group"),
		registryReadSpec("registry_get", toolutil.RouteAction(client, GetRepository), "gitlab_registry_get_repository"),
		registryDeleteSpec("registry_delete", toolutil.DestructiveAction(client, DeleteRepositoryOutput), "gitlab_registry_delete_repository"),
		registryReadSpec("registry_tag_list", toolutil.RouteAction(client, ListTags), "gitlab_registry_list_tags"),
		registryReadSpec("registry_tag_get", toolutil.RouteAction(client, GetTag), "gitlab_registry_get_tag"),
		registryDeleteSpec("registry_tag_delete", toolutil.DestructiveAction(client, DeleteTagOutput), "gitlab_registry_delete_tag"),
		registryDeleteSpec("registry_tag_delete_bulk", toolutil.DestructiveAction(client, DeleteTagsBulkOutput), "gitlab_registry_delete_tags_bulk"),
		registryReadSpec("registry_rule_list", toolutil.RouteAction(client, ListProtectionRules), "gitlab_registry_protection_list"),
		registryCreateSpec("registry_rule_create", toolutil.RouteAction(client, CreateProtectionRule), "gitlab_registry_protection_create"),
		registryUpdateSpec("registry_rule_update", toolutil.RouteAction(client, UpdateProtectionRule), "gitlab_registry_protection_update"),
		registryDeleteSpec("registry_rule_delete", toolutil.DestructiveAction(client, DeleteProtectionRuleOutput), "gitlab_registry_protection_delete"),
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

// DeleteRepositoryOutput deletes a registry repository and returns the canonical success message shape.
func DeleteRepositoryOutput(ctx context.Context, client *gitlabclient.Client, input DeleteRepositoryInput) (toolutil.DeleteOutput, error) {
	if err := DeleteRepository(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted registry repository."}, nil
}

// DeleteTagOutput deletes a registry tag and returns the canonical success message shape.
func DeleteTagOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTag(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted registry tag."}, nil
}

// DeleteTagsBulkOutput deletes registry tags in bulk and returns the canonical success message shape.
func DeleteTagsBulkOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagsBulkInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTagsBulk(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted registry tags (bulk)."}, nil
}

// DeleteProtectionRuleOutput deletes a registry protection rule and returns the canonical success message shape.
func DeleteProtectionRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteProtectionRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteProtectionRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted registry protection rule."}, nil
}

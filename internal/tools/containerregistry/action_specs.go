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
	return toolutil.NewReadActionSpec(name, route, registryOptions(individualTool))
}

func registryCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, registryOptions(individualTool))
}

func registryUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, registryOptions(individualTool))
}

func registryDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, registryOptions(individualTool))
}

func registryOptions(individualTool string) toolutil.ActionSpecOptions {
	usage := "Manage container registry repositories, tags, and protection rules for projects or groups."
	switch individualTool {
	case "gitlab_registry_list_project":
		usage = "Lists container registry image repositories for a project."
	case "gitlab_registry_list_group":
		usage = "List container registry repositories across a group."
	case "gitlab_registry_get_repository":
		usage = "Get details for one container registry repository."
	case "gitlab_registry_delete_repository":
		usage = "Delete one container registry repository."
	case "gitlab_registry_list_tags":
		usage = "List tags in one container registry repository."
	case "gitlab_registry_get_tag":
		usage = "Get metadata for one container registry tag."
	case "gitlab_registry_delete_tag":
		usage = "Delete one container registry tag."
	case "gitlab_registry_delete_tags_bulk":
		usage = "Delete container registry tags in bulk by name patterns."
	case "gitlab_registry_protection_list":
		usage = "List container registry protection rules for a project."
	case "gitlab_registry_protection_create":
		usage = "Create a container registry protection rule for a project."
	case "gitlab_registry_protection_update":
		usage = "Update a container registry protection rule in a project."
	case "gitlab_registry_protection_delete":
		usage = "Delete a container registry protection rule from a project."
	}

	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"container", "package", "registry"},
		Usage:          usage,
		RelatedActions: []string{"project.get", "package.list"},
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

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
		registryReadSpec("registry_tag_rule_list", toolutil.RouteAction(client, ListTagProtectionRules), "gitlab_registry_tag_protection_list"),
		registryCreateSpec("registry_tag_rule_create", toolutil.RouteAction(client, CreateTagProtectionRule), "gitlab_registry_tag_protection_create"),
		registryUpdateSpec("registry_tag_rule_update", toolutil.RouteAction(client, UpdateTagProtectionRule), "gitlab_registry_tag_protection_update"),
		registryDeleteSpec("registry_tag_rule_delete", toolutil.DestructiveAction(client, DeleteTagProtectionRuleOutput), "gitlab_registry_tag_protection_delete"),
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
	case "gitlab_registry_tag_protection_list":
		usage = "List container registry tag protection rules for a project."
	case "gitlab_registry_tag_protection_create":
		usage = "Create a container registry tag protection rule for a project."
	case "gitlab_registry_tag_protection_update":
		usage = "Update a container registry tag protection rule in a project."
	case "gitlab_registry_tag_protection_delete":
		usage = "Delete a container registry tag protection rule from a project."
	}

	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"container", "package", "registry"},
		Usage:          usage,
		RelatedActions: []string{"project.get", "package.list"},
		OpenWorld:      true,
		OwnerPackage:   "containerregistry",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	applyRegistryDiscovery(&options, individualTool)
	return options
}

// applyRegistryDiscovery layers per-tool discovery metadata (natural-language
// aliases, related canonical actions, parameter guidance, and the rich
// individual-tool description) on top of the shared registry options. It keeps
// the two protection-rule families cross-referenced so models can distinguish
// repository-path protection from tag protection and chain to the matching
// list/create actions.
func applyRegistryDiscovery(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_registry_list_project":
		options.Aliases = []string{"list container images", "project registry repositories", "docker images in project"}
		options.RelatedActions = []string{"package.registry_get", "package.registry_tag_list", "package.registry_list_group"}
		options.IndividualTool.Description = "List container registry image repositories in a project. Returns: each repository's id, name, path, location, status, tag count, optional tags, and pagination metadata. See also: gitlab_registry_get_repository, gitlab_registry_list_tags, gitlab_registry_list_group."
	case "gitlab_registry_list_group":
		options.Aliases = []string{"list group container images", "group registry repositories", "docker images across group"}
		options.RelatedActions = []string{"package.registry_list_project", "package.registry_get"}
		options.IndividualTool.Description = "List container registry image repositories across a group. Returns: each repository's id, name, path, location, status, tag count, and pagination metadata. See also: gitlab_registry_list_project, gitlab_registry_get_repository."
	case "gitlab_registry_get_repository":
		options.Aliases = []string{"get container repository", "registry image details", "show docker repository"}
		options.RelatedActions = []string{"package.registry_list_project", "package.registry_tag_list", "package.registry_delete"}
		options.IndividualTool.Description = "Get details of one container registry repository by its id. Returns: the repository's name, path, location, status, tag count, and optional tags. See also: gitlab_registry_list_project, gitlab_registry_list_tags, gitlab_registry_delete_repository."
	case "gitlab_registry_delete_repository":
		options.Aliases = []string{"delete container repository", "remove registry image", "purge docker repository"}
		options.RelatedActions = []string{"package.registry_list_project", "package.registry_get", "package.registry_tag_delete"}
		options.IndividualTool.Description = "Delete one container registry repository (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_list_project, gitlab_registry_get_repository, gitlab_registry_delete_tag."
	case "gitlab_registry_get_tag":
		options.Aliases = []string{"get image tag", "container tag details", "show registry tag"}
		options.RelatedActions = []string{"package.registry_tag_list", "package.registry_tag_delete"}
		options.IndividualTool.Description = "Get metadata of one container registry tag. Returns: the tag's name, path, location, revision, digest, total size, and creation time. See also: gitlab_registry_list_tags, gitlab_registry_delete_tag."
	case "gitlab_registry_delete_tag":
		options.Aliases = []string{"delete image tag", "remove container tag", "purge registry tag"}
		options.RelatedActions = []string{"package.registry_tag_list", "package.registry_tag_delete_bulk", "package.registry_tag_get"}
		options.IndividualTool.Description = "Delete one container registry tag (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_list_tags, gitlab_registry_delete_tags_bulk, gitlab_registry_get_tag."
	case "gitlab_registry_delete_tags_bulk":
		options.Aliases = []string{"bulk delete image tags", "clean up old container tags", "prune registry tags"}
		options.RelatedActions = []string{"package.registry_tag_list", "package.registry_tag_delete"}
		options.IndividualTool.Description = "Delete container registry tags in bulk by name patterns and age (cannot be undone). Returns: a success confirmation; deletion runs asynchronously. See also: gitlab_registry_list_tags, gitlab_registry_delete_tag."
	case "gitlab_registry_protection_update":
		options.Aliases = []string{"update registry protection rule", "change repository path access levels", "edit image push rule"}
		options.RelatedActions = []string{"package.registry_rule_list", "package.registry_rule_delete", "package.registry_rule_create"}
		options.IndividualTool.Description = "Update a container registry repository-path protection rule. Returns: the updated rule with its path pattern and minimum push/delete access levels. See also: gitlab_registry_protection_list, gitlab_registry_protection_delete, gitlab_registry_protection_create."
	case "gitlab_registry_protection_delete":
		options.Aliases = []string{"delete registry protection rule", "remove repository path protection", "unprotect image path"}
		options.RelatedActions = []string{"package.registry_rule_list", "package.registry_rule_create"}
		options.IndividualTool.Description = "Delete a container registry repository-path protection rule (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_protection_list, gitlab_registry_protection_create."
	}
	switch individualTool {
	case "gitlab_registry_protection_list":
		options.Aliases = []string{"list registry protection rules", "container image push rules", "repository path protection"}
		options.RelatedActions = []string{"package.registry_rule_create", "package.registry_tag_rule_list", "project.get"}
		options.IndividualTool.Description = "List container registry repository-path protection rules for a project. Returns: each rule's path pattern and minimum push/delete access levels. For tag-level protection use gitlab_registry_tag_protection_list. See also: gitlab_registry_protection_create, gitlab_registry_tag_protection_list."
	case "gitlab_registry_protection_create":
		options.Aliases = []string{"protect registry repository path", "restrict image push by path", "create registry protection rule"}
		options.RelatedActions = []string{"package.registry_rule_list", "package.registry_tag_rule_create"}
	case "gitlab_registry_list_tags":
		options.Aliases = []string{"list image tags", "container registry tags"}
		options.RelatedActions = []string{"package.registry_tag_get", "package.registry_tag_rule_list", "package.registry_get"}
	case "gitlab_registry_tag_protection_list":
		options.Aliases = []string{"list tag protection rules", "immutable tag rules", "protected image tags", "container tag protection"}
		options.RelatedActions = []string{"package.registry_tag_rule_create", "package.registry_rule_list", "package.registry_tag_list", "project.get"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or full path that owns the container registry.",
				ExampleBinding: `params.project_id:"my-org/app"`,
			},
		}
		options.IndividualTool.Description = "List container registry tag protection rules for a project. Returns: each rule's tag name pattern and minimum push/delete access levels (empty = immutable). For repository-path protection use gitlab_registry_protection_list. See also: gitlab_registry_tag_protection_create, gitlab_registry_list_tags."
	case "gitlab_registry_tag_protection_create":
		options.Aliases = []string{"protect image tags", "make tags immutable", "create tag protection rule", "restrict tag push or delete"}
		options.RelatedActions = []string{"package.registry_tag_rule_list", "package.registry_tag_rule_update", "package.registry_rule_create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"tag_name_pattern": {
				SemanticRole:     "match_pattern",
				ValueSource:      "An RE2 regular expression matching the tags to protect.",
				ExampleBinding:   `params.tag_name_pattern:"v.+"`,
				CommonConfusions: []string{"tag_name_pattern is an RE2 regex, not a glob; omit both minimum access levels to make matching tags fully immutable."},
			},
		}
		options.IndividualTool.Description = "Create a container registry tag protection rule. Returns: the created rule. Omit both minimum access levels to make matching tags immutable. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_update, gitlab_registry_protection_create."
	case "gitlab_registry_tag_protection_update":
		options.Aliases = []string{"update tag protection rule", "change tag protection access levels"}
		options.RelatedActions = []string{"package.registry_tag_rule_list", "package.registry_tag_rule_delete"}
		options.IndividualTool.Description = "Update a container registry tag protection rule. Returns: the updated rule. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_delete."
	case "gitlab_registry_tag_protection_delete":
		options.Aliases = []string{"delete tag protection rule", "remove immutable tag rule", "unprotect image tags"}
		options.RelatedActions = []string{"package.registry_tag_rule_list", "package.registry_tag_rule_create"}
		options.IndividualTool.Description = "Delete a container registry tag protection rule (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_create."
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

// DeleteTagProtectionRuleOutput deletes a registry tag protection rule and returns the canonical success message shape.
func DeleteTagProtectionRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagProtectionRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTagProtectionRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted registry tag protection rule."}, nil
}

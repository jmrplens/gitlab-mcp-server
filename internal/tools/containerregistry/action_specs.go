package containerregistry

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionRegistryTagList       = "package.registry_tag_list"
	actionRegistryTagRuleList   = "package.registry_tag_rule_list"
	actionRegistryRuleList      = "package.registry_rule_list"
	actionRegistryRuleCreate    = "package.registry_rule_create"
	actionRegistryGet           = "package.registry_get"
	actionRegistryTagRuleCreate = "package.registry_tag_rule_create"
	actionRegistryTagDelete     = "package.registry_tag_delete"
	actionRegistryListProject   = "package.registry_list_project"
	actionProjectGet            = "project.get"
	statusSuccess               = "success"
	toolRegistryTagProtUpdate   = "gitlab_registry_tag_protection_update"
	toolRegistryTagProtList     = "gitlab_registry_tag_protection_list"
	toolRegistryTagProtDelete   = "gitlab_registry_tag_protection_delete"
	toolRegistryTagProtCreate   = "gitlab_registry_tag_protection_create"
	toolRegistryProtUpdate      = "gitlab_registry_protection_update"
	toolRegistryProtList        = "gitlab_registry_protection_list"
	toolRegistryProtDelete      = "gitlab_registry_protection_delete"
	toolRegistryProtCreate      = "gitlab_registry_protection_create"
	toolRegistryListTags        = "gitlab_registry_list_tags"
	toolRegistryListProject     = "gitlab_registry_list_project"
	toolRegistryListGroup       = "gitlab_registry_list_group"
	toolRegistryGetTag          = "gitlab_registry_get_tag"
	toolRegistryGetRepo         = "gitlab_registry_get_repository"
	toolRegistryDeleteTagsBulk  = "gitlab_registry_delete_tags_bulk"
	toolRegistryDeleteTag       = "gitlab_registry_delete_tag"
	toolRegistryDeleteRepo      = "gitlab_registry_delete_repository"
)

// ActionSpecs returns canonical specs for container registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		registryReadSpec("registry_list_project", toolutil.RouteAction(client, ListProject), toolRegistryListProject),
		registryReadSpec("registry_list_group", toolutil.RouteAction(client, ListGroup), toolRegistryListGroup),
		registryReadSpec("registry_get", toolutil.RouteAction(client, GetRepository), toolRegistryGetRepo),
		registryDeleteSpec("registry_delete", toolutil.DestructiveAction(client, DeleteRepositoryOutput), toolRegistryDeleteRepo),
		registryReadSpec("registry_tag_list", toolutil.RouteAction(client, ListTags), toolRegistryListTags),
		registryReadSpec("registry_tag_get", toolutil.RouteAction(client, GetTag), toolRegistryGetTag),
		registryDeleteSpec("registry_tag_delete", toolutil.DestructiveAction(client, DeleteTagOutput), toolRegistryDeleteTag),
		registryDeleteSpec("registry_tag_delete_bulk", toolutil.DestructiveAction(client, DeleteTagsBulkOutput), toolRegistryDeleteTagsBulk),
		registryReadSpec("registry_rule_list", toolutil.RouteAction(client, ListProtectionRules), toolRegistryProtList),
		registryCreateSpec("registry_rule_create", toolutil.RouteAction(client, CreateProtectionRule), toolRegistryProtCreate),
		registryUpdateSpec("registry_rule_update", toolutil.RouteAction(client, UpdateProtectionRule), toolRegistryProtUpdate),
		registryDeleteSpec("registry_rule_delete", toolutil.DestructiveAction(client, DeleteProtectionRuleOutput), toolRegistryProtDelete),
		registryReadSpec("registry_tag_rule_list", toolutil.RouteAction(client, ListTagProtectionRules), toolRegistryTagProtList),
		registryCreateSpec("registry_tag_rule_create", toolutil.RouteAction(client, CreateTagProtectionRule), toolRegistryTagProtCreate),
		registryUpdateSpec("registry_tag_rule_update", toolutil.RouteAction(client, UpdateTagProtectionRule), toolRegistryTagProtUpdate),
		registryDeleteSpec("registry_tag_rule_delete", toolutil.DestructiveAction(client, DeleteTagProtectionRuleOutput), toolRegistryTagProtDelete),
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
	case toolRegistryListProject:
		usage = "Lists container registry image repositories for a project."
	case toolRegistryListGroup:
		usage = "List container registry repositories across a group."
	case toolRegistryGetRepo:
		usage = "Get details for one container registry repository."
	case toolRegistryDeleteRepo:
		usage = "Delete one container registry repository."
	case toolRegistryListTags:
		usage = "List tags in one container registry repository."
	case toolRegistryGetTag:
		usage = "Get metadata for one container registry tag."
	case toolRegistryDeleteTag:
		usage = "Delete one container registry tag."
	case toolRegistryDeleteTagsBulk:
		usage = "Delete container registry tags in bulk by name patterns."
	case toolRegistryProtList:
		usage = "List container registry protection rules for a project."
	case toolRegistryProtCreate:
		usage = "Create a container registry protection rule for a project."
	case toolRegistryProtUpdate:
		usage = "Update a container registry protection rule in a project."
	case toolRegistryProtDelete:
		usage = "Delete a container registry protection rule from a project."
	case toolRegistryTagProtList:
		usage = "List container registry tag protection rules for a project."
	case toolRegistryTagProtCreate:
		usage = "Create a container registry tag protection rule for a project."
	case toolRegistryTagProtUpdate:
		usage = "Update a container registry tag protection rule in a project."
	case toolRegistryTagProtDelete:
		usage = "Delete a container registry tag protection rule from a project."
	}

	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"container", "package", "registry"},
		Usage:          usage,
		RelatedActions: []string{actionProjectGet, "package.list"},
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
	case toolRegistryListProject:
		options.Aliases = []string{"list container images", "project registry repositories", "docker images in project"}
		options.RelatedActions = []string{actionRegistryGet, actionRegistryTagList, "package.registry_list_group"}
		options.IndividualTool.Description = "List container registry image repositories in a project. Returns: each repository's id, name, path, location, status, tag count, optional tags, and pagination metadata. See also: gitlab_registry_get_repository, gitlab_registry_list_tags, gitlab_registry_list_group."
	case toolRegistryListGroup:
		options.Aliases = []string{"list group container images", "group registry repositories", "docker images across group"}
		options.RelatedActions = []string{actionRegistryListProject, actionRegistryGet}
		options.IndividualTool.Description = "List container registry image repositories across a group. Returns: each repository's id, name, path, location, status, tag count, and pagination metadata. See also: gitlab_registry_list_project, gitlab_registry_get_repository."
	case toolRegistryGetRepo:
		options.Aliases = []string{"get container repository", "registry image details", "show docker repository"}
		options.RelatedActions = []string{actionRegistryListProject, actionRegistryTagList, "package.registry_delete"}
		options.IndividualTool.Description = "Get details of one container registry repository by its id. Returns: the repository's name, path, location, status, tag count, and optional tags. See also: gitlab_registry_list_project, gitlab_registry_list_tags, gitlab_registry_delete_repository."
	case toolRegistryDeleteRepo:
		options.Aliases = []string{"delete container repository", "remove registry image", "purge docker repository"}
		options.RelatedActions = []string{actionRegistryListProject, actionRegistryGet, actionRegistryTagDelete}
		options.IndividualTool.Description = "Delete one container registry repository (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_list_project, gitlab_registry_get_repository, gitlab_registry_delete_tag."
	case toolRegistryGetTag:
		options.Aliases = []string{"get image tag", "container tag details", "show registry tag"}
		options.RelatedActions = []string{actionRegistryTagList, actionRegistryTagDelete}
		options.IndividualTool.Description = "Get metadata of one container registry tag. Returns: the tag's name, path, location, revision, digest, total size, and creation time. See also: gitlab_registry_list_tags, gitlab_registry_delete_tag."
	case toolRegistryDeleteTag:
		options.Aliases = []string{"delete image tag", "remove container tag", "purge registry tag"}
		options.RelatedActions = []string{actionRegistryTagList, "package.registry_tag_delete_bulk", "package.registry_tag_get"}
		options.IndividualTool.Description = "Delete one container registry tag (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_list_tags, gitlab_registry_delete_tags_bulk, gitlab_registry_get_tag."
	case toolRegistryDeleteTagsBulk:
		options.Aliases = []string{"bulk delete image tags", "clean up old container tags", "prune registry tags"}
		options.RelatedActions = []string{actionRegistryTagList, actionRegistryTagDelete}
		options.IndividualTool.Description = "Delete container registry tags in bulk by name patterns and age (cannot be undone). Returns: a success confirmation; deletion runs asynchronously. See also: gitlab_registry_list_tags, gitlab_registry_delete_tag."
	case toolRegistryProtUpdate:
		options.Aliases = []string{"update registry protection rule", "change repository path access levels", "edit image push rule"}
		options.RelatedActions = []string{actionRegistryRuleList, "package.registry_rule_delete", actionRegistryRuleCreate}
		options.IndividualTool.Description = "Update a container registry repository-path protection rule. Returns: the updated rule with its path pattern and minimum push/delete access levels. See also: gitlab_registry_protection_list, gitlab_registry_protection_delete, gitlab_registry_protection_create."
	case toolRegistryProtDelete:
		options.Aliases = []string{"delete registry protection rule", "remove repository path protection", "unprotect image path"}
		options.RelatedActions = []string{actionRegistryRuleList, actionRegistryRuleCreate}
		options.IndividualTool.Description = "Delete a container registry repository-path protection rule (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_protection_list, gitlab_registry_protection_create."
	}
	switch individualTool {
	case toolRegistryProtList:
		options.Aliases = []string{"list registry protection rules", "container image push rules", "repository path protection"}
		options.RelatedActions = []string{actionRegistryRuleCreate, actionRegistryTagRuleList, actionProjectGet}
		options.IndividualTool.Description = "List container registry repository-path protection rules for a project. Returns: each rule's path pattern and minimum push/delete access levels. For tag-level protection use gitlab_registry_tag_protection_list. See also: gitlab_registry_protection_create, gitlab_registry_tag_protection_list."
	case toolRegistryProtCreate:
		options.Aliases = []string{"protect registry repository path", "restrict image push by path", "create registry protection rule"}
		options.RelatedActions = []string{actionRegistryRuleList, actionRegistryTagRuleCreate}
	case toolRegistryListTags:
		options.Aliases = []string{"list image tags", "container registry tags"}
		options.RelatedActions = []string{"package.registry_tag_get", actionRegistryTagRuleList, actionRegistryGet}
	case toolRegistryTagProtList:
		options.Aliases = []string{"list tag protection rules", "immutable tag rules", "protected image tags", "container tag protection"}
		options.RelatedActions = []string{actionRegistryTagRuleCreate, actionRegistryRuleList, actionRegistryTagList, actionProjectGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or full path that owns the container registry.",
				ExampleBinding: `params.project_id:"my-org/app"`,
			},
		}
		options.IndividualTool.Description = "List container registry tag protection rules for a project. Returns: each rule's tag name pattern and minimum push/delete access levels (empty = immutable). For repository-path protection use gitlab_registry_protection_list. See also: gitlab_registry_tag_protection_create, gitlab_registry_list_tags."
	case toolRegistryTagProtCreate:
		options.Aliases = []string{"protect image tags", "make tags immutable", "create tag protection rule", "restrict tag push or delete"}
		options.RelatedActions = []string{actionRegistryTagRuleList, "package.registry_tag_rule_update", actionRegistryRuleCreate}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"tag_name_pattern": {
				SemanticRole:     "match_pattern",
				ValueSource:      "An RE2 regular expression matching the tags to protect.",
				ExampleBinding:   `params.tag_name_pattern:"v.+"`,
				CommonConfusions: []string{"tag_name_pattern is an RE2 regex, not a glob; omit both minimum access levels to make matching tags fully immutable."},
			},
		}
		options.IndividualTool.Description = "Create a container registry tag protection rule. Returns: the created rule. Omit both minimum access levels to make matching tags immutable. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_update, gitlab_registry_protection_create."
	case toolRegistryTagProtUpdate:
		options.Aliases = []string{"update tag protection rule", "change tag protection access levels"}
		options.RelatedActions = []string{actionRegistryTagRuleList, "package.registry_tag_rule_delete"}
		options.IndividualTool.Description = "Update a container registry tag protection rule. Returns: the updated rule. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_delete."
	case toolRegistryTagProtDelete:
		options.Aliases = []string{"delete tag protection rule", "remove immutable tag rule", "unprotect image tags"}
		options.RelatedActions = []string{actionRegistryTagRuleList, actionRegistryTagRuleCreate}
		options.IndividualTool.Description = "Delete a container registry tag protection rule (cannot be undone). Returns: a success confirmation. See also: gitlab_registry_tag_protection_list, gitlab_registry_tag_protection_create."
	}
}

// DeleteRepositoryOutput deletes a registry repository and returns the canonical success message shape.
func DeleteRepositoryOutput(ctx context.Context, client *gitlabclient.Client, input DeleteRepositoryInput) (toolutil.DeleteOutput, error) {
	if err := DeleteRepository(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: "Successfully deleted registry repository."}, nil
}

// DeleteTagOutput deletes a registry tag and returns the canonical success message shape.
func DeleteTagOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTag(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: "Successfully deleted registry tag."}, nil
}

// DeleteTagsBulkOutput deletes registry tags in bulk and returns the canonical success message shape.
func DeleteTagsBulkOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagsBulkInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTagsBulk(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: "Successfully deleted registry tags (bulk)."}, nil
}

// DeleteProtectionRuleOutput deletes a registry protection rule and returns the canonical success message shape.
func DeleteProtectionRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteProtectionRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteProtectionRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: "Successfully deleted registry protection rule."}, nil
}

// DeleteTagProtectionRuleOutput deletes a registry tag protection rule and returns the canonical success message shape.
func DeleteTagProtectionRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteTagProtectionRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteTagProtectionRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: "Successfully deleted registry tag protection rule."}, nil
}

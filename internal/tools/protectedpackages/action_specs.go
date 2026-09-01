package protectedpackages

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionPackageProtectionRuleCreate = "package.protection_rule_create"
	actionPackageProtectionRuleList   = "package.protection_rule_list"
)

// ActionSpecs returns canonical specs for package protection rule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		protectedPackageReadSpec("protection_rule_list", toolutil.RouteAction(client, List), "gitlab_list_package_protection_rules"),
		protectedPackageCreateSpec("protection_rule_create", toolutil.RouteAction(client, Create), "gitlab_create_package_protection_rule"),
		protectedPackageUpdateSpec("protection_rule_update", toolutil.RouteAction(client, Update), "gitlab_update_package_protection_rule"),
		protectedPackageDeleteSpec("protection_rule_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_package_protection_rule"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("package protection rule %d from project %s", input.RuleID, input.ProjectID))
	return out, nil
}

func protectedPackageReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, protectedPackageOptions(individualTool))
}

func protectedPackageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, protectedPackageOptions(individualTool))
}

func protectedPackageUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, protectedPackageOptions(individualTool))
}

func protectedPackageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, protectedPackageOptions(individualTool))
}

func protectedPackageOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute protectedpackages domain action.", Tags: []string{"package", "protection"},
		OpenWorld:      true,
		OwnerPackage:   "protectedpackages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	applyProtectedPackageDiscovery(&options, individualTool)
	return options
}

// applyProtectedPackageDiscovery layers per-tool discovery metadata
// (natural-language aliases, related canonical actions, and a rich
// individual-tool description in the Returns:/See also: form) on top of the
// shared package protection options so models can chain list/create/update/
// delete actions and distinguish package protection from container registry
// protection rules.
func applyProtectedPackageDiscovery(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_package_protection_rules":
		options.Usage = "List package protection rules configured for one project. Use when the prompt asks which package name patterns are protected, who may push or delete packages, or to find a rule id before updating or deleting it."
		options.Aliases = []string{"list package protection rules", "protected package patterns", "package push rules"}
		options.RelatedActions = []string{actionPackageProtectionRuleCreate, "package.protection_rule_update", "project.get"}
		options.IndividualTool.Description = "List package protection rules for a project. Returns: each rule's id, project id, package name pattern, package type, and minimum push/delete access levels, with pagination metadata. For container image protection use gitlab_registry_protection_list. See also: gitlab_create_package_protection_rule, gitlab_update_package_protection_rule."
	case "gitlab_create_package_protection_rule":
		options.Usage = "Create a package protection rule for a project. Provide project_id, a package_name_pattern, and a package_type, then set the minimum push/delete access levels to restrict who can publish or remove matching packages."
		options.Aliases = []string{"protect package name pattern", "restrict package push", "create package protection rule"}
		options.RelatedActions = []string{actionPackageProtectionRuleList, "package.protection_rule_update", "package.protection_rule_delete"}
		options.IndividualTool.Description = "Create a package protection rule for a project. Returns: the new rule's id, project id, package name pattern, package type, and minimum push/delete access levels. See also: gitlab_list_package_protection_rules, gitlab_update_package_protection_rule, gitlab_delete_package_protection_rule."
	case "gitlab_update_package_protection_rule":
		options.Usage = "Update an existing package protection rule by its id. Use to change the package name pattern, package type, or the minimum push/delete access levels of a rule located via gitlab_list_package_protection_rules."
		options.Aliases = []string{"update package protection rule", "change package push access levels", "edit package protection rule"}
		options.RelatedActions = []string{actionPackageProtectionRuleList, actionPackageProtectionRuleCreate, "package.protection_rule_delete"}
		options.IndividualTool.Description = "Update an existing package protection rule by its id. Returns: the updated rule's id, project id, package name pattern, package type, and minimum push/delete access levels. See also: gitlab_list_package_protection_rules, gitlab_create_package_protection_rule, gitlab_delete_package_protection_rule."
	case "gitlab_delete_package_protection_rule":
		options.Usage = "Delete a package protection rule from a project by its id. Use when a package name pattern should no longer be protected. This removes the restriction permanently and cannot be undone."
		options.Aliases = []string{"delete package protection rule", "remove package protection", "unprotect package pattern"}
		options.RelatedActions = []string{actionPackageProtectionRuleList, actionPackageProtectionRuleCreate}
		options.IndividualTool.Description = "Delete a package protection rule from a project by its id (cannot be undone). Returns: a success confirmation. See also: gitlab_list_package_protection_rules, gitlab_create_package_protection_rule."
	}
}

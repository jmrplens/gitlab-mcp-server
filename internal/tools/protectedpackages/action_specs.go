package protectedpackages

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	options := protectedPackageOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedPackageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, protectedPackageOptions(individualTool))
}

func protectedPackageUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := protectedPackageOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedPackageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := protectedPackageOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedPackageOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"package", "protection"},
		OpenWorld:      true,
		OwnerPackage:   "protectedpackages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

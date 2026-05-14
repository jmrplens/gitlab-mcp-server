package protectedpackages

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for package protection rule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		protectedPackageReadSpec("protection_rule_list", toolutil.RouteAction(client, List), "gitlab_list_package_protection_rules"),
		protectedPackageCreateSpec("protection_rule_create", toolutil.RouteAction(client, Create), "gitlab_create_package_protection_rule"),
		protectedPackageUpdateSpec("protection_rule_update", toolutil.RouteAction(client, Update), "gitlab_update_package_protection_rule"),
		protectedPackageDeleteSpec("protection_rule_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_package_protection_rule"),
	}
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

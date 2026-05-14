package license

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for license tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		licenseReadSpec("license_get", toolutil.RouteAction(client, Get), "gitlab_get_license"),
		licenseCreateSpec("license_add", toolutil.RouteAction(client, Add), "gitlab_add_license"),
		licenseDeleteSpec("license_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_license"),
	}
}

func licenseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := licenseOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func licenseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, licenseOptions(individualTool))
}

func licenseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := licenseOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func licenseOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "license"},
		OpenWorld:      true,
		OwnerPackage:   "license",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

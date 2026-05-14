package securitysettings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ProjectActionSpecs returns canonical specs for project security settings actions.
func ProjectActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectSecurityReadSpec("security_settings_get", toolutil.RouteAction(client, GetProject), "gitlab_get_project_security_settings"),
		projectSecurityUpdateSpec("security_settings_update", toolutil.RouteAction(client, UpdateProject), "gitlab_update_project_secret_push_protection"),
	}
}

func projectSecurityReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := projectSecurityOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectSecurityUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := projectSecurityOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectSecurityOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "security"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		Edition:        "ultimate",
		OwnerPackage:   "securitysettings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

package groupsaml

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group SAML link actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupSAMLReadSpec("saml_link_list", toolutil.RouteAction(client, List), "gitlab_group_saml_link_list"),
		groupSAMLReadSpec("saml_link_get", toolutil.RouteAction(client, Get), "gitlab_group_saml_link_get"),
		groupSAMLCreateSpec("saml_link_add", toolutil.RouteAction(client, Add), "gitlab_group_saml_link_add"),
		groupSAMLDeleteSpec("saml_link_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_saml_link_delete"),
	}
}

func groupSAMLReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupSAMLOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupSAMLCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupSAMLOptions(individualTool))
}

func groupSAMLDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupSAMLOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupSAMLOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "saml"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsaml",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

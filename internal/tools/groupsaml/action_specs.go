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
	return toolutil.NewReadActionSpec(name, route, groupSAMLOptions(individualTool))
}

func groupSAMLCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupSAMLOptions(individualTool))
}

func groupSAMLDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupSAMLOptions(individualTool))
}

func groupSAMLOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupsaml domain action.", Tags: []string{"group", "saml"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsaml",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

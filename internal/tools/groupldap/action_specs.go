package groupldap

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group LDAP link actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupLDAPReadSpec("ldap_link_list", toolutil.RouteAction(client, List), "gitlab_group_ldap_link_list"),
		groupLDAPCreateSpec("ldap_link_add", toolutil.RouteAction(client, Add), "gitlab_group_ldap_link_add"),
		groupLDAPDeleteSpec("ldap_link_delete", toolutil.DestructiveVoidAction(client, DeleteWithCNOrFilter), "gitlab_group_ldap_link_delete"),
		groupLDAPDeleteSpec("ldap_link_delete_for_provider", toolutil.DestructiveVoidAction(client, DeleteForProvider), "gitlab_group_ldap_link_delete_for_provider"),
	}
}

func groupLDAPReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupLDAPOptions(individualTool))
}

func groupLDAPCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupLDAPOptions(individualTool))
}

func groupLDAPDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupLDAPOptions(individualTool))
}

func groupLDAPOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupldap domain action.", Tags: []string{"group", "ldap"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupldap",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

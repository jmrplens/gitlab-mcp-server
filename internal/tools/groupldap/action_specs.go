package groupldap

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := groupLDAPOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupLDAPCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupLDAPOptions(individualTool))
}

func groupLDAPDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupLDAPOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupLDAPOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "ldap"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupldap",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

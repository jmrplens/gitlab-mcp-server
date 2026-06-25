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
		groupLDAPCreateSpec("ldap_sync", toolutil.RouteAction(client, Sync), "gitlab_group_ldap_sync"),
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
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Manage a group's LDAP group links (Premium/Ultimate).", Tags: []string{"group", "ldap"},
		RelatedActions: []string{"group.get", "group.ldap_link_list", "group.ldap_sync"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupldap",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_group_ldap_link_list":
		options.Usage = "List the LDAP group links configured on a group (Premium/Ultimate). Use to see which LDAP CNs/filters and providers map to the group before adding, deleting, or syncing links."
		options.Aliases = []string{"list ldap links", "show group ldap links", "ldap group mappings"}
		options.RelatedActions = []string{"group.get", "group.ldap_link_add", "group.ldap_sync"}
		options.IndividualTool.Description = "List a GitLab group's LDAP group links. Returns: each link's CN/filter, provider, and access level. See also: gitlab_group_ldap_link_add, gitlab_group_ldap_sync, gitlab_group_get."
	case "gitlab_group_ldap_link_add":
		options.Usage = "Add an LDAP group link to a group by CN or filter (Premium/Ultimate). After adding links you can trigger gitlab_group_ldap_sync to apply membership immediately."
		options.Aliases = []string{"add ldap link", "create group ldap mapping", "link ldap group"}
		options.RelatedActions = []string{"group.ldap_link_list", "group.ldap_sync", "group.ldap_link_delete"}
		options.IndividualTool.Description = "Add an LDAP group link to a GitLab group by CN or filter (Premium/Ultimate). Returns: the created link with its CN/filter, provider, access level, and member role ID. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_sync, gitlab_group_ldap_link_delete."
	case "gitlab_group_ldap_sync":
		options.Usage = "Trigger an LDAP synchronization for a group's LDAP group links (Premium/Ultimate). Use after adding or editing LDAP links, or when group membership looks stale, to reconcile members against LDAP now. The API accepts the request and runs the sync asynchronously in the background."
		options.Aliases = []string{"sync ldap", "synchronize group ldap", "trigger ldap sync", "refresh ldap membership", "reconcile ldap members"}
		options.RelatedActions = []string{"group.ldap_link_list", "group.ldap_link_add", "group.get"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path whose LDAP links are synced.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Sync is asynchronous: a success response means the job was queued, not that membership is already updated."},
			},
		}
		options.IndividualTool.Description = "Trigger an asynchronous LDAP sync for a GitLab group's LDAP links. Returns: a confirmation that the sync was queued. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_add, gitlab_group_get."
	case "gitlab_group_ldap_link_delete":
		options.Usage = "Delete an LDAP group link from a group by CN or filter (Premium/Ultimate). Pass the same CN or filter (and provider, when set) used to create the link; trigger gitlab_group_ldap_sync afterwards to reconcile membership."
		options.Aliases = []string{"delete ldap link", "remove group ldap mapping", "unlink ldap group"}
		options.RelatedActions = []string{"group.ldap_link_list", "group.ldap_link_add", "group.ldap_sync"}
		options.IndividualTool.Description = "Delete a GitLab group's LDAP link by CN or filter (Premium/Ultimate). Returns: a success confirmation. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_add, gitlab_group_ldap_sync."
	case "gitlab_group_ldap_link_delete_for_provider":
		options.Usage = "Delete an LDAP group link for a specific provider by CN (Premium/Ultimate). Use when the same CN is linked through more than one LDAP provider and only one provider's link should be removed."
		options.Aliases = []string{"delete ldap link for provider", "remove ldap mapping for provider", "unlink ldap group for provider"}
		options.RelatedActions = []string{"group.ldap_link_list", "group.ldap_link_delete", "group.ldap_sync"}
		options.IndividualTool.Description = "Delete a GitLab group's LDAP link for a specific provider by CN (Premium/Ultimate). Returns: a success confirmation. See also: gitlab_group_ldap_link_list, gitlab_group_ldap_link_delete, gitlab_group_ldap_sync."
	}

	return options
}

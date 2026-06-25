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
		groupSAMLReadSpec("saml_users_list", toolutil.RouteAction(client, SAMLUsersList), "gitlab_group_saml_users_list"),
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
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Manage a group's SAML group links and SAML-provisioned users (Premium/Ultimate).", Tags: []string{"group", "saml"},
		RelatedActions: []string{"group.get", "group.saml_link_list", "group.saml_users_list"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsaml",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_group_saml_link_list":
		options.Usage = "List the SAML group links configured on a group (Premium/Ultimate). Use to see which SAML group names map to access levels. To list the actual users provisioned through SAML SSO, use gitlab_group_saml_users_list instead."
		options.Aliases = []string{"list saml links", "show group saml links", "saml group mappings"}
		options.RelatedActions = []string{"group.get", "group.saml_link_add", "group.saml_users_list"}
		options.IndividualTool.Description = "List a GitLab group's SAML group links. Returns: each link's SAML group name, access level, and provider. See also: gitlab_group_saml_users_list, gitlab_group_saml_link_add, gitlab_group_get."
	case "gitlab_group_saml_users_list":
		options.Usage = "List the users provisioned via SAML SSO for a top-level group (Premium/Ultimate). Use when the user asks which accounts came in through the group's SAML SSO, distinct from group SAML *links* (which map SAML group names to access levels)."
		options.Aliases = []string{"saml users", "list saml provisioned users", "group saml sso users", "users from saml sso", "saml account list"}
		options.RelatedActions = []string{"group.saml_link_list", "group.members", "group.get"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Top-level group numeric ID or full path with SAML SSO configured.",
				ExampleBinding:   `params.group_id:"my-org"`,
				CommonConfusions: []string{"Only top-level groups with SAML SSO have SAML users; subgroups return nothing.", "This lists users, not the SAML group links — use gitlab_group_saml_link_list for the link mappings."},
			},
			"username": {
				ValueSource:    "Exact username to filter to a single SAML user.",
				ExampleBinding: `params.username:"jdoe"`,
			},
		}
		options.IndividualTool.Description = "List SAML SSO-provisioned users of a top-level GitLab group, with keyset pagination. Returns: matching users with full profile (id, username, name, state, email, identities, scim_identities, custom_attributes, created_by) plus pagination metadata. See also: gitlab_group_saml_link_list, gitlab_group_members_list, gitlab_group_get."
	case "gitlab_group_saml_link_add":
		options.Usage = "Add a SAML group link mapping a SAML group name to an access level (Premium/Ultimate)."
		options.Aliases = []string{"add saml link", "create group saml mapping", "link saml group"}
		options.RelatedActions = []string{"group.saml_link_list", "group.saml_users_list", "group.saml_link_delete"}
	}

	return options
}

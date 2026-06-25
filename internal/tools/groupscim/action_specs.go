package groupscim

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for group SCIM identity actions. They are the
// "<domain>.<action>" identifiers projected from the gitlab_group_scim meta
// group and are used as cross-links in RelatedActions for dynamic find ranking.
const (
	actionGroupSCIMList   = "group_scim.list"
	actionGroupSCIMGet    = "group_scim.get"
	actionGroupSCIMUpdate = "group_scim.update"
	actionGroupSCIMDelete = "group_scim.delete"
)

// ActionSpecs returns canonical specs for group SCIM identity actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	updateAction := func(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
		if err := Update(ctx, client, input); err != nil {
			return UpdateOutput{}, err
		}
		return UpdateOutput{Updated: true, Message: "SCIM identity updated successfully."}, nil
	}

	return []toolutil.ActionSpec{
		groupSCIMReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_group_scim_identities"),
		groupSCIMReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_group_scim_identity"),
		groupSCIMUpdateSpec("update", toolutil.RouteAction(client, updateAction), "gitlab_update_group_scim_identity"),
		groupSCIMDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_group_scim_identity"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted SCIM identity %s from group %s.", input.UID, input.GroupID),
	}, nil
}

func groupSCIMReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupSCIMOptions(name, individualTool))
}

func groupSCIMUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupSCIMOptions(name, individualTool))
}

func groupSCIMDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupSCIMOptions(name, individualTool))
}

// groupSCIMActionMetaEntry holds the non-generic discovery metadata for one
// SCIM identity action: an action-specific Usage line, domain-specific
// natural-language Aliases, canonical RelatedActions cross-links, and the
// individual-tool description in "Returns: … See also: …" form.
type groupSCIMActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupSCIMActionMeta maps each canonical SCIM identity action name to its
// discovery metadata. These surfaces drive dynamic find ranking and the
// individual-tool descriptions (1:1 audit R-META). The phrasing centers on the
// distinctive "SCIM identity" SAML SSO provisioning vocabulary so the aliases
// stay unique to this Premium/Ultimate domain.
var groupSCIMActionMeta = map[string]groupSCIMActionMetaEntry{
	"list": {
		usage:       "List every SCIM identity provisioned for a top-level group via SAML SSO SCIM (Premium/Ultimate). Use this to enumerate which user accounts were synchronized through the group's SCIM provisioning and to discover the SCIM external UIDs needed by get, update, and delete. SCIM identities only exist after SAML SSO SCIM has synced users.",
		aliases:     []string{"list group scim identities", "list scim provisioned identities", "show group scim identity mappings", "enumerate scim external uids", "scim identity list"},
		related:     []string{actionGroupSCIMGet, actionGroupSCIMUpdate, actionGroupSCIMDelete},
		description: "List a top-level group's SCIM identities provisioned through SAML SSO SCIM. Returns: each identity with external_uid, user_id, and active status. See also: gitlab_get_group_scim_identity, gitlab_update_group_scim_identity, gitlab_delete_group_scim_identity.",
	},
	"get": {
		usage:       "Fetch a single SCIM identity for a group by its SCIM external UID (Premium/Ultimate). Use this after listing identities, or when the prompt already names a SCIM external UID, to inspect which GitLab user that SAML SSO SCIM-provisioned identity maps to and whether it is active.",
		aliases:     []string{"get group scim identity", "show scim identity", "fetch scim identity by uid", "lookup scim provisioned user", "scim identity details"},
		related:     []string{actionGroupSCIMList, actionGroupSCIMUpdate, actionGroupSCIMDelete},
		description: "Get one SCIM identity of a top-level group by its SCIM external UID. Returns: the identity's external_uid, user_id, and active status. See also: gitlab_list_group_scim_identities, gitlab_update_group_scim_identity, gitlab_delete_group_scim_identity.",
	},
	"update": {
		usage:       "Update the extern_uid field of an existing SCIM identity for a group (Premium/Ultimate). Use this when a SAML SSO SCIM-provisioned user's external identifier changed at the identity provider and must be re-pointed; it only rewrites the SCIM external UID, not the underlying GitLab user.",
		aliases:     []string{"update group scim identity", "change scim extern uid", "remap scim provisioned user", "rewrite scim external uid", "edit scim identity"},
		related:     []string{actionGroupSCIMGet, actionGroupSCIMList, actionGroupSCIMDelete},
		description: "Update the extern_uid of an existing group SCIM identity. Returns: a confirmation that the SCIM identity's external UID was rewritten. See also: gitlab_get_group_scim_identity, gitlab_list_group_scim_identities, gitlab_delete_group_scim_identity.",
	},
	"delete": {
		usage:       "Delete a single SCIM identity from a group by its SCIM external UID (Premium/Ultimate, destructive). Use this to de-provision a SAML SSO SCIM-synced identity link; verify the SCIM external UID with list first because removal of the identity mapping is permanent.",
		aliases:     []string{"delete group scim identity", "remove scim identity", "deprovision scim user", "unlink scim provisioned identity", "revoke scim identity"},
		related:     []string{actionGroupSCIMGet, actionGroupSCIMList, actionGroupSCIMUpdate},
		description: "Delete a SCIM identity from a top-level group by its SCIM external UID. Returns: a success confirmation naming the deleted SCIM identity and group. See also: gitlab_get_group_scim_identity, gitlab_list_group_scim_identities, gitlab_update_group_scim_identity.",
	},
}

func groupSCIMOptions(name, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases:      []string{individualTool},
		Usage:        "Use to execute groupscim domain action.",
		Tags:         []string{"scim", "identity"},
		OpenWorld:    true,
		Edition:      "premium",
		OwnerPackage: "groupscim",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
	if meta, ok := groupSCIMActionMeta[name]; ok {
		opts.Usage = meta.usage
		opts.Aliases = append([]string{individualTool}, meta.aliases...)
		opts.RelatedActions = append([]string(nil), meta.related...)
		opts.IndividualTool.Description = meta.description
	}
	return opts
}

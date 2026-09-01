package groupmembers

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupGet     = "group.get"
	actionGroupMembers = "group.members"

	actionBillableMembers           = "group_billable_members_list"
	actionBillableMemberMemberships = "group_billable_member_memberships_list"
	actionBillableMemberRemove      = "group_billable_member_remove"

	toolBillableMembers           = "gitlab_list_billable_group_members"
	toolBillableMemberMemberships = "gitlab_list_billable_member_memberships"
	toolBillableMemberRemove      = "gitlab_remove_billable_group_member"
)

// ActionSpecs returns canonical specs for group member actions. The get,
// get_inherited, add, edit, remove, share, and unshare routes are projected
// into the dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004). Each spec carries action-specific discovery metadata (Usage,
// natural-language Aliases, canonical RelatedActions, ParameterGuidance, and an
// individual-tool Description) per the 1:1 audit R-META requirement.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupMemberReadSpec("group_member_get", toolutil.RouteAction(client, GetMember), "gitlab_group_member_get"),
		groupMemberReadSpec("group_member_get_inherited", toolutil.RouteAction(client, GetInheritedMember), "gitlab_group_member_get_inherited"),
		groupMemberCreateSpec("group_member_add", toolutil.RouteAction(client, AddMember), "gitlab_group_member_add"),
		groupMemberUpdateSpec("group_member_edit", toolutil.RouteAction(client, EditMember), "gitlab_group_member_edit"),
		groupMemberDeleteSpec("group_member_remove", toolutil.DestructiveAction(client, removeMemberOutput), "gitlab_group_member_remove"),
		groupMemberCreateSpec("group_member_share", toolutil.RouteAction(client, ShareGroup), "gitlab_group_share"),
		groupMemberDeleteSpec("group_member_unshare", toolutil.DestructiveAction(client, unshareGroupOutput), "gitlab_group_unshare"),
		billableMemberReadSpec(actionBillableMembers, toolutil.RouteAction(client, ListBillableMembers), toolBillableMembers),
		billableMemberReadSpec(actionBillableMemberMemberships, toolutil.RouteAction(client, ListBillableMemberMemberships), toolBillableMemberMemberships),
		billableMemberDeleteSpec(actionBillableMemberRemove, toolutil.DestructiveAction(client, removeBillableMemberOutput), toolBillableMemberRemove),
	}
}

// billableMemberReadSpec builds a read-only, Premium/Ultimate-gated canonical
// spec for a billable-member action.
func billableMemberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, billableMemberOptions(individualTool))
}

// billableMemberDeleteSpec builds a destructive, Premium/Ultimate-gated
// canonical spec for a billable-member action.
func billableMemberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, billableMemberOptions(individualTool))
}

// billableMemberOptions returns the base options for a billable-member action,
// gated to Enterprise Premium/Ultimate (Edition "premium") and decorated with
// the action-specific discovery metadata from groupMemberActionMeta.
func billableMemberOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmembers domain action.", Tags: []string{"group", "member", "billable"},
		RelatedActions: []string{actionGroupGet, actionGroupMembers},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "groupmembers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateGroupMemberMeta(&options, individualTool)
	return options
}

func groupMemberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	decorateGroupMemberMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupMemberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	decorateGroupMemberMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func groupMemberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	decorateGroupMemberMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func groupMemberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMemberOptions(individualTool)
	decorateGroupMemberMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func groupMemberOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmembers domain action.", Tags: []string{"group", "member"},
		RelatedActions: []string{actionGroupGet, actionGroupMembers},
		OpenWorld:      true,
		OwnerPackage:   "groupmembers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupMemberMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description for each group-member tool, replacing the generic
// placeholder metadata from groupMemberOptions (1:1 audit R-META).
func decorateGroupMemberMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupMemberActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string{individualTool}, meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if len(meta.guidance) > 0 {
		options.ParameterGuidance = meta.guidance
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// groupMemberActionMetaEntry is the discovery metadata for one group-member
// action.
type groupMemberActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// groupIDGuidance is the shared parameter guidance for the group_id argument
// that every group-member action accepts.
func groupIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_group",
		ValueSource:      "Group ID or full namespace path that owns the membership.",
		ExampleBinding:   `params.group_id:"my-org/platform"`,
		CommonConfusions: []string{"Use the group here, not a project path or project member action. Project members use the project-member tools."},
	}
}

// userIDGuidance returns parameter guidance for the user_id argument with the
// supplied value-source description.
func userIDGuidance(valueSource string) toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "user_id",
		ValueSource:      valueSource,
		ExampleBinding:   "params.user_id:42",
		CommonConfusions: []string{"Use the numeric user ID, not the username. Resolve usernames with gitlab_list_users first."},
	}
}

// accessLevelGuidance returns parameter guidance for the access_level argument.
func accessLevelGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "access_level",
		ValueSource:      "Numeric role level for the membership.",
		ExampleBinding:   "params.access_level:30",
		CommonConfusions: []string{"Use the numeric level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), not the role name."},
	}
}

// shareGroupIDGuidance returns parameter guidance for the share_group_id
// argument used by the share/unshare actions.
func shareGroupIDGuidance(valueSource string) toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "share_group_id",
		ValueSource:      valueSource,
		ExampleBinding:   "params.share_group_id:100",
		CommonConfusions: []string{"share_group_id must be a numeric group ID, not a path, and is distinct from group_id (the group being shared)."},
	}
}

// groupMemberActionMeta maps each individual group-member tool to its discovery
// metadata.
var groupMemberActionMeta = map[string]groupMemberActionMetaEntry{
	"gitlab_group_member_get": {
		usage:   "Get one direct member of a group by group_id plus user_id. Use this when the prompt names a known user and group and you need that user's exact access level, expiry, or custom member role. Inherited members are not returned. Use group_member_get_inherited for those.",
		aliases: []string{"get group member", "show group member access", "check user role in group"},
		related: []string{actionGroupMembers, "group_member_get_inherited", "group_member_edit"},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"user_id":  userIDGuidance("Numeric user ID of the direct group member to inspect."),
		},
		description: "Get a single direct member of a group by user ID. Returns: access level, custom member role, created_by, expiry, public email, SAML identity, and seat usage. See also: gitlab_group_members_list, gitlab_group_member_get_inherited, gitlab_group_member_edit.",
	},
	"gitlab_group_member_get_inherited": {
		usage:   "Get a member of a group including membership inherited from ancestor groups, by group_id plus user_id. Use this when a user may be a member via a parent group rather than a direct membership.",
		aliases: []string{"get inherited group member", "show inherited member access", "check inherited role"},
		related: []string{"group_member_get", actionGroupMembers, "group_member_edit"},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"user_id":  userIDGuidance("Numeric user ID of the member to inspect, including inherited memberships."),
		},
		description: "Get a single member of a group including inherited memberships from ancestor groups. Returns: effective access level, custom member role, created_by, expiry, public email, SAML identity, and seat usage. See also: gitlab_group_member_get, gitlab_group_members_list, gitlab_group_member_edit.",
	},
	"gitlab_group_member_add": {
		usage:   "Add a user as a direct member of a group with a chosen access level. Use this to grant a known user a role in a group. Supply user_id or username plus access_level, and optionally member_role_id for a custom role (Premium/Ultimate) or expires_at.",
		aliases: []string{"add group member", "add existing user to group", "grant group access"},
		related: []string{actionGroupMembers, "group_member_edit", "group_member_get"},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id":     groupIDGuidance(),
			"user_id":      userIDGuidance("Numeric user ID to add as a member (alternative to username)."),
			"access_level": accessLevelGuidance(),
			"member_role_id": {
				SemanticRole:     "member_role_id",
				ValueSource:      "ID of a custom member role to assign (Premium/Ultimate). Its base access level must match access_level.",
				ExampleBinding:   "params.member_role_id:7",
				CommonConfusions: []string{"member_role_id references a custom role definition, not access_level. It is only available on Premium/Ultimate."},
			},
		},
		description: "Add a user as a direct member of a group. Returns: the created membership with access level, custom member role, expiry, and seat usage. See also: gitlab_group_members_list, gitlab_group_member_edit, gitlab_group_member_get.",
	},
	"gitlab_group_member_edit": {
		usage:   "Edit a direct group member's access level, expiry, or custom member role by group_id plus user_id. Use this to promote, demote, change expiry, or reassign the custom role of an existing direct member. Inherited members cannot be edited here.",
		aliases: []string{"edit group member", "change group member access level", "update group role"},
		related: []string{actionGroupMembers, "group_member_get", "group_member_remove"},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id":     groupIDGuidance(),
			"user_id":      userIDGuidance("Numeric user ID of the direct member to edit."),
			"access_level": accessLevelGuidance(),
			"member_role_id": {
				SemanticRole:     "member_role_id",
				ValueSource:      "ID of a custom member role to assign (Premium/Ultimate). Its base access level must match access_level.",
				ExampleBinding:   "params.member_role_id:7",
				CommonConfusions: []string{"member_role_id references a custom role definition, not access_level. It is only available on Premium/Ultimate."},
			},
		},
		description: "Edit a direct group member's access level, expiry, or custom member role. Returns: the updated membership. See also: gitlab_group_members_list, gitlab_group_member_get, gitlab_group_member_remove.",
	},
	"gitlab_group_member_remove": {
		usage:   "Remove a direct member from a group by group_id plus user_id. Destructive: requires confirmation. Inherited members cannot be removed here. Remove them from the ancestor group where they were added directly.",
		aliases: []string{"remove group member", "revoke group access", "delete group member"},
		related: []string{actionGroupMembers, "group_member_get", "group_member_edit"},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"user_id":  userIDGuidance("Numeric user ID of the direct member to remove."),
		},
		description: "Remove a direct member from a group (destructive, requires confirmation). Returns: a delete confirmation. See also: gitlab_group_members_list, gitlab_group_member_get, gitlab_group_member_edit.",
	},
	"gitlab_group_share": {
		usage:   "Share a group with another group so its members gain access at a chosen group_access level. Use this for cross-group collaboration. Supply group_id (the group to share) and share_group_id (the recipient group). Group shares accept only Guest/Reporter/Developer/Maintainer levels.",
		aliases: []string{"share group with group", "grant group access to another group", "add group share"},
		related: []string{"group_member_unshare", actionGroupGet, actionGroupMembers},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id":       groupIDGuidance(),
			"share_group_id": shareGroupIDGuidance("Numeric ID of the recipient group that should gain access."),
			"group_access": {
				SemanticRole:     "access_level",
				ValueSource:      "Access level granted to the recipient group (10/20/30/40 only).",
				ExampleBinding:   "params.group_access:30",
				CommonConfusions: []string{"Group shares accept only 10/20/30/40. 5, 15, 25, and 60 are rejected."},
			},
		},
		description: "Share a group with another group at a chosen access level. Returns: the shared group's id, name, path, and web URL. See also: gitlab_group_unshare, gitlab_group_get, gitlab_group_members_list.",
	},
	"gitlab_group_unshare": {
		usage:   "Stop sharing a group with another group by group_id plus share_group_id. Destructive: requires confirmation. Use this to revoke a previously created group share.",
		aliases: []string{"unshare group", "revoke group share", "remove group share"},
		related: []string{"group_member_share", actionGroupGet, actionGroupMembers},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id":       groupIDGuidance(),
			"share_group_id": shareGroupIDGuidance("Numeric ID of the group whose share should be revoked."),
		},
		description: "Revoke an existing group-to-group share (destructive, requires confirmation). Returns: a delete confirmation. See also: gitlab_group_share, gitlab_group_get, gitlab_group_members_list.",
	},
	"gitlab_list_billable_group_members": {
		usage:   "List the billable members of a group: the users who count toward the group's seat usage, including members inherited from subgroups and shared projects. Use this to audit license/seat consumption (Premium/Ultimate). Filter with search and order with sort.",
		aliases: []string{"list billable group members", "show seats used in group", "audit billable members", "list licensed users in group"},
		related: []string{actionBillableMemberMemberships, actionBillableMemberRemove, actionGroupMembers},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"search": {
				SemanticRole:     "search_query",
				ValueSource:      "Substring of a member's name or username to filter the billable list.",
				ExampleBinding:   `params.search:"jane"`,
				CommonConfusions: []string{"search filters by name/username, not by email or user ID."},
			},
			"sort": {
				SemanticRole:     "sort_order",
				ValueSource:      "Sort key for the billable list (e.g. name_asc, last_activity_on_desc).",
				ExampleBinding:   `params.sort:"last_activity_on_desc"`,
				CommonConfusions: []string{"sort is a combined field_direction token, not a separate order_by/direction pair."},
			},
		},
		description: "List the billable members of a group (Premium/Ultimate). Returns: each member's id, username, name, state, email, membership type, removable flag, is_last_owner, last activity, and last login, plus pagination. See also: gitlab_list_billable_member_memberships, gitlab_remove_billable_group_member, gitlab_group_members_list.",
	},
	"gitlab_list_billable_member_memberships": {
		usage:   "List the memberships through which a single billable member counts toward a group's seats: the source groups/projects and their access levels. Use this to understand why a user is billable before removing them (Premium/Ultimate). Supply group_id plus the billable member's user_id.",
		aliases: []string{"list billable member memberships", "why is user billable", "show member seat sources", "list user memberships in group"},
		related: []string{actionBillableMembers, actionBillableMemberRemove, actionGroupMembers},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"user_id":  userIDGuidance("Numeric user ID of the billable member whose memberships to list. From gitlab_list_billable_group_members."),
		},
		description: "List the memberships of a billable group member (Premium/Ultimate). Returns: each membership's source id, source full name, source members URL, access level (numeric + string), created/expiry dates, plus pagination. See also: gitlab_list_billable_group_members, gitlab_remove_billable_group_member, gitlab_group_members_list.",
	},
	"gitlab_remove_billable_group_member": {
		usage:   "Remove a billable member from a group to free a seat (Premium/Ultimate). Destructive: requires confirmation. Only members whose 'removable' flag is true can be removed here. The last owner cannot be removed. Supply group_id plus the billable member's user_id.",
		aliases: []string{"remove billable group member", "free a group seat", "revoke billable member", "remove licensed user from group"},
		related: []string{actionBillableMembers, actionBillableMemberMemberships, actionGroupMembers},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": groupIDGuidance(),
			"user_id":  userIDGuidance("Numeric user ID of the removable billable member. Check the 'removable' flag from gitlab_list_billable_group_members first."),
		},
		description: "Remove a billable member from a group, freeing a seat (Premium/Ultimate, destructive, requires confirmation). Returns: a removal confirmation. See also: gitlab_list_billable_group_members, gitlab_list_billable_member_memberships, gitlab_group_members_list.",
	},
}

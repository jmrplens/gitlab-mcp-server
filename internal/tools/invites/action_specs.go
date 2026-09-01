package invites

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs referenced as RelatedActions across surfaces.
const (
	actionInviteProject     = "access.invite_project"
	actionInviteGroup       = "access.invite_group"
	actionInviteListProject = "access.invite_list_project"
	actionInviteListGroup   = "access.invite_list_group"
)

// ActionSpecs returns canonical specs for project and group invitation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		inviteReadSpec("invite_list_project", toolutil.RouteAction(client, ListPendingProjectInvitations), "gitlab_project_invite_list_pending"),
		inviteReadSpec("invite_list_group", toolutil.RouteAction(client, ListPendingGroupInvitations), "gitlab_group_invite_list_pending"),
		inviteCreateSpec("invite_project", toolutil.RouteAction(client, ProjectInvites), "gitlab_project_invite"),
		inviteCreateSpec("invite_group", toolutil.RouteAction(client, GroupInvites), "gitlab_group_invite"),
	}
}

func inviteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := inviteOptions(individualTool)
	decorateInviteMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func inviteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := inviteOptions(individualTool)
	decorateInviteMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func inviteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute invites domain action.", Tags: []string{"access", "invite"},
		OpenWorld:      true,
		OwnerPackage:   "invites",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateInviteMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the
// "Returns: … See also: …" individual-tool description for each invite
// action (1:1 audit R-META).
func decorateInviteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := inviteActionMeta[individualTool]
	if !ok {
		return
	}
	options.Usage = meta.usage
	options.Aliases = append([]string(nil), meta.aliases...)
	options.RelatedActions = append([]string(nil), meta.related...)
	options.IndividualTool.Description = meta.description
	if len(meta.guidance) > 0 {
		options.ParameterGuidance = meta.guidance
	}
}

// inviteActionMetaEntry is the discovery metadata for one invite action.
type inviteActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
	guidance    map[string]toolutil.ParameterGuidance
}

// inviteGuidance builds the guidance map for an invite action: the
// scope-specific entry (project_id or group_id) plus the email, user_id,
// access_level, and expires_at entries that are identical for the project
// and group invite surfaces.
func inviteGuidance(scopeParam string, scope toolutil.ParameterGuidance) map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		scopeParam: scope,
		"email": {
			SemanticRole:     "invite_email",
			ValueSource:      "Email address of the person to invite when they may not have an account yet.",
			ExampleBinding:   `params.email:"newhire@example.com"`,
			CommonConfusions: []string{"Provide either email or user_id. Supply email for people who are not yet GitLab users."},
		},
		"user_id": {
			SemanticRole:     "user_id",
			ValueSource:      "Numeric user ID when inviting an existing GitLab user.",
			ExampleBinding:   "params.user_id:42",
			CommonConfusions: []string{"Use user_id for existing accounts and email otherwise. Do not pass a username here."},
		},
		"access_level": {
			SemanticRole:     "access_level",
			ValueSource:      "Role to grant: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner.",
			ExampleBinding:   "params.access_level:30",
			CommonConfusions: []string{"Pass the numeric access level, not a role name like 'developer'."},
		},
		"expires_at": {
			SemanticRole:     "calendar_date",
			ValueSource:      "Date the invitation should expire, when the user requests one.",
			ExampleBinding:   `params.expires_at:"2026-12-31"`,
			CommonConfusions: []string{"Use YYYY-MM-DD date only. This invitation field is not an RFC3339 timestamp."},
		},
	}
}

// inviteActionMeta maps each individual invite tool to its discovery metadata.
var inviteActionMeta = map[string]inviteActionMetaEntry{
	"gitlab_project_invite": {
		usage:   "Invite a user to a project by email address or user_id with a chosen access_level. Use when adding someone who is not yet a project member, including external users invited by email. For users who already have an account prefer project.member_add.",
		aliases: []string{"invite user to project", "add user to project by email", "send project invitation"},
		related: []string{actionInviteListProject, "project.member_add", "access.request_project", "project.members"},
		description: "Invite a user to a project by email or user ID with an access level. Returns: an invitation result with status and per-email messages. " +
			"See also: gitlab_project_invite_list_pending, gitlab_project_member_add, gitlab_access_request_request_project.",
		guidance: inviteGuidance("project_id", toolutil.ParameterGuidance{
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path the user is being invited to.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the target project here. Use group_id only with group.invite_group."},
		}),
	},
	"gitlab_group_invite": {
		usage:   "Invite a user to a group by email address or user_id with a chosen access_level. Use when adding someone who is not yet a group member, including external users invited by email. For users who already have an account prefer group.group_member_add.",
		aliases: []string{"invite user to group", "add user to group by email", "send group invitation"},
		related: []string{actionInviteListGroup, "group.group_member_add", "access.request_group", "group.members"},
		description: "Invite a user to a group by email or user ID with an access level. Returns: an invitation result with status and per-email messages. " +
			"See also: gitlab_group_invite_list_pending, gitlab_group_member_add, gitlab_access_request_request_group.",
		guidance: inviteGuidance("group_id", toolutil.ParameterGuidance{
			SemanticRole:     "scope_group",
			ValueSource:      "Group ID or full group path the user is being invited to.",
			ExampleBinding:   `params.group_id:"platform/backend"`,
			CommonConfusions: []string{"Use group_id for the group scope. Use project_id only with project.invite_project."},
		}),
	},
	"gitlab_project_invite_list_pending": {
		usage:   "List the pending (not yet accepted) invitations for a project. Use to audit outstanding email invitations before resending or revoking them. This lists invitations, not current members.",
		aliases: []string{"list pending project invitations", "show outstanding project invites", "pending project invitations"},
		related: []string{actionInviteProject, "project.members", "access.request_list_project"},
		description: "List a project's pending invitations. Returns: pending invitations with invite email, access level, creator, creation and expiry dates, plus pagination metadata. " +
			"See also: gitlab_project_invite, gitlab_project_members_list.",
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or full namespace path whose pending invitations should be listed.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Lists pending invitations, not accepted members. Use project.members for current members."},
			},
			"query": {
				ValueSource:      "Substring to filter pending invitations by invite email or name.",
				ExampleBinding:   `params.query:"@example.com"`,
				CommonConfusions: []string{"query filters within this project's pending invitations. It does not replace project_id."},
			},
		},
	},
	"gitlab_group_invite_list_pending": {
		usage:   "List the pending (not yet accepted) invitations for a group. Use to audit outstanding email invitations before resending or revoking them. This lists invitations, not current members.",
		aliases: []string{"list pending group invitations", "show outstanding group invites", "pending group invitations"},
		related: []string{actionInviteGroup, "group.members", "access.request_list_group"},
		description: "List a group's pending invitations. Returns: pending invitations with invite email, access level, creator, creation and expiry dates, plus pagination metadata. " +
			"See also: gitlab_group_invite, gitlab_group_members_list.",
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group ID or full group path whose pending invitations should be listed.",
				ExampleBinding:   `params.group_id:"platform/backend"`,
				CommonConfusions: []string{"Lists pending invitations, not accepted members. Use group.members for current members."},
			},
			"query": {
				ValueSource:      "Substring to filter pending invitations by invite email or name.",
				ExampleBinding:   `params.query:"@example.com"`,
				CommonConfusions: []string{"query filters within this group's pending invitations. It does not replace group_id."},
			},
		},
	},
}

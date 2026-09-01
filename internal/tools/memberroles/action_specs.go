package memberroles

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom member role actions
// exposed as MCP tools. The instance and group list, create, and delete
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_instance_member_roles — list instance-level custom member roles.
		memberRoleReadSpec("list_instance", toolutil.RouteAction(client, ListInstance), "gitlab_list_instance_member_roles"),
		// gitlab_create_instance_member_role — create an instance-level custom role.
		memberRoleCreateSpec("create_instance", toolutil.RouteAction(client, CreateInstance), "gitlab_create_instance_member_role"),
		// gitlab_delete_instance_member_role — remove an instance-level custom role (destructive).
		memberRoleDeleteSpec("delete_instance", toolutil.DestructiveAction(client, deleteInstanceOutput), "gitlab_delete_instance_member_role"),
		// gitlab_list_group_member_roles — list group-level custom member roles.
		memberRoleReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_list_group_member_roles"),
		// gitlab_create_group_member_role — create a group-level custom role.
		memberRoleCreateSpec("create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_create_group_member_role"),
		// gitlab_delete_group_member_role — remove a group-level custom role (destructive).
		memberRoleDeleteSpec("delete_group", toolutil.DestructiveAction(client, deleteGroupOutput), "gitlab_delete_group_member_role"),
	}
}

// deleteInstanceOutput adapts the package's [DeleteInstance] handler
// to the [toolutil.DestructiveAction] contract, returning a structured
// success result that names the deleted role in the message.
func deleteInstanceOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInstanceInput) (toolutil.DeleteOutput, error) {
	if err := DeleteInstance(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("instance member role %d", input.MemberRoleID))
	return out, nil
}

// deleteGroupOutput adapts the package's [DeleteGroup] handler to the
// [toolutil.DestructiveAction] contract, returning a structured
// success result that names the role and group in the message.
func deleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("member role %d from group %s", input.MemberRoleID, input.GroupID))
	return out, nil
}

// memberRoleReadSpec builds a read-only [toolutil.ActionSpec] for a
// custom member role action using the package's default
// [memberRoleOptions] decorated with action-specific discovery metadata.
func memberRoleReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleCreateSpec builds a create-style [toolutil.ActionSpec] for
// a custom member role action using the package's default
// [memberRoleOptions] decorated with action-specific discovery metadata.
func memberRoleCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleDeleteSpec builds a destructive [toolutil.ActionSpec] for
// a custom member role action using the package's default
// [memberRoleOptions] decorated with action-specific discovery metadata.
func memberRoleDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberRoleOptions(name, individualTool))
}

// memberRoleOptions returns the base [toolutil.ActionSpecOptions] for a
// custom member role action, layering each action's non-generic Usage,
// natural-language Aliases, canonical RelatedActions, ParameterGuidance, and
// the "Returns: … See also: …" IndividualTool.Description from
// [memberRoleActionMeta] on top of the catalog defaults (1:1 audit R-META).
func memberRoleOptions(name, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute memberroles domain action.", Tags: []string{"member_role"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "memberroles",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateMemberRoleMeta(&opts, name)
	return opts
}

// decorateMemberRoleMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description for the named member role action from
// [memberRoleActionMeta]. It is a no-op for actions with no metadata entry.
func decorateMemberRoleMeta(opts *toolutil.ActionSpecOptions, name string) {
	meta, ok := memberRoleActionMeta[name]
	if !ok {
		return
	}
	if meta.usage != "" {
		opts.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		opts.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		opts.RelatedActions = append([]string(nil), meta.related...)
	}
	if len(meta.guidance) > 0 {
		opts.ParameterGuidance = meta.guidance
	}
	if meta.description != "" {
		opts.IndividualTool.Description = meta.description
	}
}

// memberRoleActionMetaEntry is the discovery metadata for one custom
// member role action.
type memberRoleActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

const (
	actionMRListInstance   = "member_role.list_instance"
	actionMRListGroup      = "member_role.list_group"
	actionMRCreateInstance = "member_role.create_instance"
	actionMRCreateGroup    = "member_role.create_group"
	actionMRDeleteInstance = "member_role.delete_instance"
	actionMRDeleteGroup    = "member_role.delete_group"
)

// baseAccessLevelConfusion is the shared CommonConfusions note for the
// base_access_level parameter on the create actions.
var baseAccessLevelConfusion = []string{"Use a valid numeric level (5/10/15/20/25/30/40/50). 60=Admin is not accepted, and do not pass role names like Developer."}

// memberRoleActionMeta maps each canonical member role action name to its
// non-generic Usage, natural-language Aliases, canonical RelatedActions,
// ParameterGuidance, and individual-tool description. These surfaces drive
// dynamic find ranking and the individual-tool descriptions (1:1 audit R-META).
var memberRoleActionMeta = map[string]memberRoleActionMetaEntry{
	"list_instance": {
		usage:       "Lists all custom member roles defined at the instance level (Ultimate on self-managed, all tiers on GitLab.com). Use this when the prompt asks for custom member roles in general, because list_group is deprecated on self-managed 17+ and returns 400 in that environment.",
		aliases:     []string{"list instance member roles", "list custom roles", "show instance member roles", "list member roles"},
		related:     []string{actionMRListGroup, actionMRCreateInstance, actionMRDeleteInstance},
		description: "List instance-level custom member roles. Returns: each role with id, name, description, base_access_level, and its enabled permission flags. See also: gitlab_create_instance_member_role, gitlab_delete_instance_member_role, gitlab_list_group_member_roles.",
	},
	"list_group": {
		usage:   "Lists custom member roles available for a specific group_id. Deprecated on self-managed GitLab 17+. Use list_instance instead on self-managed. Still valid on GitLab.com Ultimate.",
		aliases: []string{"list group member roles", "show group custom roles", "list a group's member roles"},
		related: []string{actionMRListInstance, actionMRCreateGroup, actionMRDeleteGroup},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Top-level group ID or full group path whose custom member roles should be listed.",
				ExampleBinding:   `params.group_id:"my-group"`,
				CommonConfusions: []string{"Use a top-level group, not a subgroup or project. Group-level roles are deprecated on self-managed. Use list_instance there."},
			},
		},
		description: "List group-level custom member roles. Returns: each role with id, name, description, base_access_level, and its enabled permission flags. See also: gitlab_list_instance_member_roles, gitlab_create_group_member_role, gitlab_delete_group_member_role.",
	},
	"create_instance": {
		usage:   "Create a new instance-level custom member role. Provide name and base_access_level, then enable individual permission flags (e.g. read_code, admin_merge_request) only when requested. Requires admin + self-managed Ultimate.",
		aliases: []string{"create instance member role", "create custom role", "add instance member role", "define custom role"},
		related: []string{actionMRListInstance, actionMRDeleteInstance, actionMRCreateGroup},
		guidance: map[string]toolutil.ParameterGuidance{
			"name": {
				SemanticRole:   "member_role_name",
				ValueSource:    "Short, unique name for the custom role from the user's request.",
				ExampleBinding: `params.name:"Security Reviewer"`,
			},
			"base_access_level": {
				SemanticRole:     "access_level",
				ValueSource:      "Numeric base access level the custom role extends (5/10/15/20/25/30/40/50).",
				ExampleBinding:   "params.base_access_level:30",
				CommonConfusions: baseAccessLevelConfusion,
			},
		},
		description: "Create an instance-level custom member role. Returns: the created role with id, name, base_access_level, and its enabled permission flags. See also: gitlab_list_instance_member_roles, gitlab_delete_instance_member_role, gitlab_create_group_member_role.",
	},
	"create_group": {
		usage:   "Create a new group-level custom member role for a group_id. Provide name and base_access_level, then enable individual permission flags only when requested. Requires Owner + Ultimate. Deprecated on self-managed 17+.",
		aliases: []string{"create group member role", "add group custom role", "define group member role"},
		related: []string{actionMRListGroup, actionMRDeleteGroup, actionMRCreateInstance},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Top-level group ID or full group path that will own the custom role.",
				ExampleBinding:   `params.group_id:"my-group"`,
				CommonConfusions: []string{"Use a top-level group, not a subgroup or project. Group-level roles are deprecated on self-managed. Use create_instance there."},
			},
			"name": {
				SemanticRole:   "member_role_name",
				ValueSource:    "Short, unique name for the custom role within the group.",
				ExampleBinding: `params.name:"Security Reviewer"`,
			},
			"base_access_level": {
				SemanticRole:     "access_level",
				ValueSource:      "Numeric base access level the custom role extends (5/10/15/20/25/30/40/50).",
				ExampleBinding:   "params.base_access_level:30",
				CommonConfusions: baseAccessLevelConfusion,
			},
		},
		description: "Create a group-level custom member role. Returns: the created role with id, name, base_access_level, and its enabled permission flags. See also: gitlab_list_group_member_roles, gitlab_delete_group_member_role, gitlab_create_instance_member_role.",
	},
	"delete_instance": {
		usage:   "Permanently delete an instance-level custom member role by member_role_id. Destructive and irreversible. May fail while the role is still assigned. Requires admin + self-managed Ultimate.",
		aliases: []string{"delete instance member role", "remove custom role", "remove instance member role"},
		related: []string{actionMRListInstance, actionMRCreateInstance, actionMRDeleteGroup},
		guidance: map[string]toolutil.ParameterGuidance{
			"member_role_id": {
				SemanticRole:     "member_role_id",
				ValueSource:      "Numeric ID of the instance member role to delete, usually from gitlab_list_instance_member_roles output.",
				ExampleBinding:   "params.member_role_id:42",
				CommonConfusions: []string{"Use the role's member_role_id, not its name or a user/member ID. Verify it with gitlab_list_instance_member_roles first."},
			},
		},
		description: "Delete an instance-level custom member role. Returns: a success confirmation naming the deleted role. See also: gitlab_list_instance_member_roles, gitlab_create_instance_member_role.",
	},
	"delete_group": {
		usage:   "Permanently delete a group-level custom member role by group_id and member_role_id. Destructive and irreversible. May fail while the role is still assigned. Requires Owner + Ultimate. Deprecated on self-managed 17+.",
		aliases: []string{"delete group member role", "remove group custom role", "remove group member role"},
		related: []string{actionMRListGroup, actionMRCreateGroup, actionMRDeleteInstance},
		guidance: map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Top-level group ID or full group path that owns the custom role.",
				ExampleBinding:   `params.group_id:"my-group"`,
				CommonConfusions: []string{"Use the group that owns the role, not a subgroup or project. Group-level roles are deprecated on self-managed."},
			},
			"member_role_id": {
				SemanticRole:     "member_role_id",
				ValueSource:      "Numeric ID of the group member role to delete, usually from gitlab_list_group_member_roles output.",
				ExampleBinding:   "params.member_role_id:42",
				CommonConfusions: []string{"Use the role's member_role_id, not its name or a user/member ID. Verify it with gitlab_list_group_member_roles first."},
			},
		},
		description: "Delete a group-level custom member role. Returns: a success confirmation naming the deleted role and group. See also: gitlab_list_group_member_roles, gitlab_create_group_member_role.",
	},
}

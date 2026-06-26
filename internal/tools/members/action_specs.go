package members

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionMembersList = "members.list"
	actionMemberGet   = "member.get"
	actionMemberEdit  = "member.edit"
	actionUserGet     = "user.get"
	actionProjectGet  = "project.get"
	paramUserID       = "user_id"
	paramProjectID    = "project_id"
	paramAccessLevel  = "access_level"
)

// ActionSpecs returns canonical specs for project member actions exposed
// as MCP tools. The list, get, get-inherited, add, edit, and delete
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_project_members_list — list project members (direct + inherited).
		memberReadSpec("members", toolutil.RouteAction(client, List), "gitlab_project_members_list"),
		// gitlab_project_member_get — fetch a direct project member.
		memberReadSpec("member_get", toolutil.RouteAction(client, Get), "gitlab_project_member_get"),
		// gitlab_project_member_get_inherited — fetch a project member including inherited.
		memberReadSpec("member_inherited", toolutil.RouteAction(client, GetInherited), "gitlab_project_member_get_inherited"),
		// gitlab_project_member_add — add a user to a project.
		memberCreateSpec("member_add", toolutil.RouteAction(client, Add), "gitlab_project_member_add"),
		// gitlab_project_member_edit — edit a project member's access level.
		memberUpdateSpec("member_edit", toolutil.RouteAction(client, Edit), "gitlab_project_member_edit"),
		// gitlab_project_member_delete — remove a member from a project (destructive).
		memberDeleteSpec("member_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_member_delete"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that names the resource in the message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project member")
	return out, nil
}

// memberReadSpec builds a read-only [toolutil.ActionSpec] for a project
// member action using the package's default [memberOptions].
func memberReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, memberOptions(individualTool))
}

// memberCreateSpec builds a create-style [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, memberOptions(individualTool))
}

// memberUpdateSpec builds an update-style [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, memberOptions(individualTool))
}

// memberDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// project member action using the package's default [memberOptions].
func memberDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, memberOptions(individualTool))
}

// projectIDGuidance is the shared parameter guidance for the project_id
// parameter across every member action.
var projectIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "scope_project",
	ValueSource:      "Project ID or full namespace path (e.g. \"group/project\") that owns the membership.",
	ExampleBinding:   `params.project_id:"group/project"`,
	CommonConfusions: []string{"Use the project here, not a group ID; for group memberships use the group member actions instead."},
}

// userIDGuidance is the shared parameter guidance for the user_id parameter
// across the member get/edit/delete actions.
var userIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     paramUserID,
	ValueSource:      "Numeric user ID of the member, from gitlab_list_users or prior member list output.",
	ExampleBinding:   "params.user_id:42",
	CommonConfusions: []string{"Use the global user ID, not a username; pass username only on the add action."},
}

// memberOptions returns the [toolutil.ActionSpecOptions] for a project member
// action, including action-specific usage, natural-language aliases, canonical
// related actions, parameter guidance, and individual-tool description metadata
// (1:1 audit R-META).
func memberOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute members domain action.",
		Tags:           []string{"project", "member", "access"},
		RelatedActions: []string{actionProjectGet, actionUserGet},
		OpenWorld:      true,
		OwnerPackage:   "members",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_project_members_list":
		opts.Usage = "List the members of a project, including members inherited from parent groups. Filter with query, user_ids, and show_seat_info; order with order_by/sort; page with offset or keyset pagination."
		opts.Aliases = []string{individualTool, "list project members", "who has access to project", "show project team", "project collaborators"}
		opts.RelatedActions = []string{actionMemberGet, "member.add", actionProjectGet, actionUserGet}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{paramProjectID: projectIDGuidance}
		opts.IndividualTool.Description = "List a project's members (direct plus inherited from parent groups). Returns: each member's id, username, name, state, access level, member role, created_by, seat usage, and web URL. Supports query/user_ids/show_seat_info filtering, order_by/sort, and offset or keyset pagination. See also: gitlab_project_member_get, gitlab_project_member_add, gitlab_project_get."
	case "gitlab_project_member_get":
		opts.Usage = "Get one direct project member by project_id plus user_id. Use after a member list, or when the prompt already names a concrete user. Returns only direct members; use member.get_inherited to include parent-group inheritance."
		opts.Aliases = []string{individualTool, "get project member", "show member access level", "is user a member"}
		opts.RelatedActions = []string{actionMembersList, "member.get_inherited", actionMemberEdit, "member.delete"}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{paramProjectID: projectIDGuidance, paramUserID: userIDGuidance}
		opts.IndividualTool.Description = "Get a single direct project member by user ID. Returns: id, username, name, state, access level, member role, created_by, expiry, and web URL. Does not include inherited members. See also: gitlab_project_member_get_inherited, gitlab_project_members_list, gitlab_project_member_edit."
	case "gitlab_project_member_get_inherited":
		opts.Usage = "Get a project member including membership inherited from any parent group. Use when member.get returns not-found but the user has access through an ancestor group."
		opts.Aliases = []string{individualTool, "get inherited member", "member including inherited", "effective project access"}
		opts.RelatedActions = []string{actionMemberGet, actionMembersList, actionProjectGet}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{paramProjectID: projectIDGuidance, paramUserID: userIDGuidance}
		opts.IndividualTool.Description = "Get a project member including membership inherited from parent groups. Returns: id, username, name, state, effective access level, member role, and web URL. See also: gitlab_project_member_get, gitlab_project_members_list."
	case "gitlab_project_member_add":
		opts.Usage = "Add a user to a project team by user_id or username with an access_level. Optionally set expires_at and member_role_id (Premium/Ultimate custom role). Idempotent for already-existing members."
		opts.Aliases = []string{individualTool, "add project member", "grant project access", "add existing user to project", "give user access"}
		opts.RelatedActions = []string{actionMembersList, actionMemberGet, actionMemberEdit, actionUserGet}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramProjectID: projectIDGuidance,
			paramUserID: {
				SemanticRole:     paramUserID,
				ValueSource:      "Numeric user ID to add; provide either user_id or username.",
				ExampleBinding:   "params.user_id:42",
				CommonConfusions: []string{"Provide user_id or username, not both; access_level is also required."},
			},
			paramAccessLevel: {
				SemanticRole:     paramAccessLevel,
				ValueSource:      "Numeric access level: 10 Guest, 20 Reporter, 30 Developer, 40 Maintainer, 50 Owner (plus 5/15/25/60 on supported tiers).",
				ExampleBinding:   "params.access_level:30",
				CommonConfusions: []string{"Use the numeric level, not a role name; set member_role_id for a custom role."},
			},
		}
		opts.IndividualTool.Description = "Add a user to a project team by user_id or username with an access level. Optional: expires_at, member_role_id (Premium/Ultimate). Returns: the created membership. See also: gitlab_project_member_edit, gitlab_project_members_list, gitlab_list_users."
	case "gitlab_project_member_edit":
		opts.Usage = "Edit an existing project member's access_level, expires_at, or member_role_id. Requires at least the same access level as the target member."
		opts.Aliases = []string{individualTool, "edit project member", "change member access level", "promote member", "update membership"}
		opts.RelatedActions = []string{actionMembersList, actionMemberGet, "member.add", "member.delete"}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramProjectID: projectIDGuidance,
			paramUserID:    userIDGuidance,
			paramAccessLevel: {
				SemanticRole:     paramAccessLevel,
				ValueSource:      "New numeric access level: 10 Guest, 20 Reporter, 30 Developer, 40 Maintainer, 50 Owner (plus 5/15/25/60 on supported tiers).",
				ExampleBinding:   "params.access_level:40",
				CommonConfusions: []string{"Use the numeric level, not a role name; you must hold at least the same level as the target."},
			},
		}
		opts.IndividualTool.Description = "Edit a project member's access level, expiry, or custom member role. Returns: the updated membership. Requires at least the same access level as the target member. See also: gitlab_project_member_add, gitlab_project_member_delete, gitlab_project_members_list."
	case "gitlab_project_member_delete":
		opts.Usage = "Remove a member from a project by user_id. Destructive and requires confirmation. Requires Maintainer; the last Owner cannot be removed."
		opts.Aliases = []string{individualTool, "remove project member", "revoke project access", "delete membership", "kick user from project"}
		opts.RelatedActions = []string{actionMembersList, actionMemberGet, actionMemberEdit}
		opts.ParameterGuidance = map[string]toolutil.ParameterGuidance{paramProjectID: projectIDGuidance, paramUserID: userIDGuidance}
		opts.IndividualTool.Description = "Remove a member from a project (destructive, requires confirmation). Requires Maintainer; the last Owner of a project cannot be removed. Returns: a success confirmation naming the removed member. See also: gitlab_project_member_edit, gitlab_project_members_list."
	}

	return opts
}

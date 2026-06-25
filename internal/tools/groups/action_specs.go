package groups

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionGroupGet = "group.get"

// ActionSpecs returns canonical specs for core group and group hook actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_group_list — list groups visible to the authenticated user.
		groupReadSpec("list", toolutil.RouteAction(client, List), "gitlab_group_list"),
		// gitlab_group_get — fetch a single group by ID or path (returns a structured not-found result on 404).
		groupReadSpec("get", groupGetRoute(client), "gitlab_group_get"),
		// gitlab_group_create — create a new top-level group or subgroup.
		groupCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_group_create"),
		// gitlab_group_update — update an existing group.
		groupUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_group_update"),
		// gitlab_group_delete — delete a group and all its projects (destructive).
		groupDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_delete"),
		// gitlab_group_restore — restore a previously deleted group.
		groupUpdateSpec("restore", toolutil.RouteAction(client, Restore), "gitlab_group_restore"),
		// gitlab_group_archive — archive a group (destructive, idempotent).
		groupUpdateSpec("archive", toolutil.RouteAction(client, ArchiveOutput), "gitlab_group_archive"),
		// gitlab_group_unarchive — unarchive a group (destructive, idempotent).
		groupUpdateSpec("unarchive", toolutil.RouteAction(client, UnarchiveOutput), "gitlab_group_unarchive"),
		// gitlab_group_search — search for groups by name or path.
		groupReadSpec("search", toolutil.RouteAction(client, Search), "gitlab_group_search"),
		// gitlab_group_transfer_project — transfer a project into a group.
		groupUpdateSpec("transfer_project", toolutil.RouteAction(client, TransferProject), "gitlab_group_transfer_project"),
		// gitlab_group_projects — list the projects that belong to a group.
		groupReadSpec("projects", toolutil.RouteAction(client, ListProjects), "gitlab_group_projects"),
		// gitlab_group_members_list — list the members of a group.
		groupReadSpec("members", toolutil.RouteAction(client, MembersList), "gitlab_group_members_list"),
		// gitlab_subgroups_list — list the subgroups of a group.
		groupReadSpec("subgroups", toolutil.RouteAction(client, SubgroupsList), "gitlab_subgroups_list"),
		// gitlab_group_shared_with_list — list groups shared with a group.
		groupReadSpec("shared_with", toolutil.RouteAction(client, SharedWithList), "gitlab_group_shared_with_list"),
		// gitlab_group_invited_list — list groups invited to a group.
		groupReadSpec("invited_groups", toolutil.RouteAction(client, InvitedList), "gitlab_group_invited_list"),
		// gitlab_group_transfer_locations — list candidate parent groups for a transfer.
		groupReadSpec("transfer_locations", toolutil.RouteAction(client, TransferLocationsList), "gitlab_group_transfer_locations"),
		// gitlab_group_hook_list — list group webhooks.
		groupReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_group_hook_list"),
		// gitlab_group_hook_get — fetch a single group webhook by ID.
		groupReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_group_hook_get"),
		// gitlab_group_hook_add — create a new group webhook.
		groupCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_group_hook_add"),
		// gitlab_group_hook_edit — update an existing group webhook.
		groupUpdateSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_group_hook_edit"),
		// gitlab_group_hook_delete — delete a group webhook (destructive).
		groupDeleteSpec("hook_delete", toolutil.DestructiveVoidAction(client, DeleteHook), "gitlab_group_hook_delete"),
	}
}

func groupGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return groupNotFoundOutput{Identifier: fmt.Sprint(input["group_id"])}, nil
		}
		return result, err
	}
	return route
}

// ArchiveOutput archives a GitLab group and returns the legacy success message shape.
func ArchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Archive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s archived successfully", input.GroupID)}, nil
}

// UnarchiveOutput unarchives a GitLab group and returns the legacy success message shape.
func UnarchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Unarchive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s unarchived successfully", input.GroupID)}, nil
}

func groupReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	_ = actionName

	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groups domain action.", Tags: []string{"group"},
		OpenWorld:      true,
		OwnerPackage:   "groups",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	if applyGroupHookMetadata(individualTool, &options) {
		return options
	}

	switch individualTool {
	case "gitlab_group_get":
		options.Usage = "Get one exact group by group_id (numeric ID or full path). Use this when the prompt already targets a specific group and needs metadata such as visibility, parent, web URL, or statistics."
		options.Aliases = []string{"get group", "show group details", "lookup group by path"}
		options.RelatedActions = []string{"group.list", "group.members", "group.projects", "group.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path from the prompt or prior discovery step.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Use group_id for path or ID; do not send project_id for group lookups."},
			},
		}
		options.IndividualTool.Description = "Get one GitLab group by ID or path. Returns: group metadata, visibility, parent information, and web URL. See also: gitlab_group_list, gitlab_group_members_list, gitlab_group_projects, gitlab_group_update."
	case "gitlab_group_list":
		options.Usage = "List groups visible to the authenticated user. Use search, owned, min_access_level, and pagination when the user asks for matching or accessible groups."
		options.Aliases = []string{"list groups", "show visible groups", "find groups"}
		options.RelatedActions = []string{actionGroupGet, "group.search", "group.create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"search": {
				ValueSource:      "Group name/path keywords from the user query.",
				ExampleBinding:   `params.search:"platform"`,
				CommonConfusions: []string{"search filters visible groups; it does not accept project paths."},
			},
		}
		options.IndividualTool.Description = "List accessible GitLab groups with filtering and pagination. Returns: matching groups including path, name, and visibility metadata. See also: gitlab_group_get, gitlab_group_search, gitlab_group_create."
	case "gitlab_group_create":
		options.Usage = "Create a group with name and path. Optionally set parent_id, description, visibility, and project creation permissions when requested."
		options.Aliases = []string{"create group", "create subgroup", "new group"}
		options.RelatedActions = []string{actionGroupGet, "group.update", "group.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"name": {
				SemanticRole:   "group_name",
				ValueSource:    "Human-readable group display name from user intent.",
				ExampleBinding: `params.name:"Platform Team"`,
			},
			"path": {
				SemanticRole:     "group_path_segment",
				ValueSource:      "URL-safe group path segment.",
				ExampleBinding:   `params.path:"platform-team"`,
				CommonConfusions: []string{"path is a slug segment, not a full URL or namespace with slashes unless creating nested groups via parent_id."},
			},
		}
		options.IndividualTool.Description = "Create a GitLab group or subgroup. Returns: created group metadata including ID, full path, and visibility. See also: gitlab_group_get, gitlab_group_update, gitlab_group_delete."
	case "gitlab_group_members_list":
		options.Usage = "List the direct and inherited members of a group. Use query, user_ids, show_seat_info, and pagination when the user asks who belongs to a group or at what access level."
		options.Aliases = []string{"list group members", "show group members", "who is in this group"}
		options.RelatedActions = []string{actionGroupGet, "group.projects", "group.member_add"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path whose members are listed.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
		}
		options.IndividualTool.Description = "List the members of a GitLab group (direct and inherited). Returns: members with username, access level, and seat/role metadata. See also: gitlab_group_get, gitlab_group_projects, gitlab_group_member_add."
	case "gitlab_group_projects":
		options.Usage = "List the projects that belong to a group. Use include_subgroups, archived, visibility, topic, and ordering when the user asks which projects live under a group."
		options.Aliases = []string{"list group projects", "show projects in group", "group repositories"}
		options.RelatedActions = []string{actionGroupGet, "group.subgroups", "group.transfer_project"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path whose projects are listed.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Set include_subgroups=true to also include projects in descendant groups."},
			},
		}
		options.IndividualTool.Description = "List the projects in a GitLab group. Returns: projects with path, visibility, and archived status. See also: gitlab_group_get, gitlab_subgroups_list, gitlab_group_transfer_project."
	case "gitlab_group_search":
		options.Usage = "Search for groups by name or path keywords. Use when the user wants to find groups matching a term without already knowing an ID or path."
		options.Aliases = []string{"search groups", "find group by name", "lookup groups"}
		options.RelatedActions = []string{actionGroupGet, "group.list", "group.create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"query": {
				ValueSource:    "Group name/path keywords from the user query.",
				ExampleBinding: `params.query:"platform"`,
			},
		}
		options.IndividualTool.Description = "Search GitLab groups by name or path. Returns: matching groups with path, name, and visibility. See also: gitlab_group_list, gitlab_group_get, gitlab_group_create."
	case "gitlab_subgroups_list":
		options.Usage = "List the descendant groups (subgroups at all depths) of a group. Use search, min_access_level, visibility, and ordering when the user asks which groups nest under a group."
		options.Aliases = []string{"list subgroups", "show descendant groups", "nested groups"}
		options.RelatedActions = []string{actionGroupGet, "group.projects", "group.transfer_locations"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path whose subgroups are listed.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Returns descendants at all depths; set top_level_only=true for direct children only."},
			},
		}
		options.IndividualTool.Description = "List the subgroups (descendant groups) of a GitLab group. Returns: descendant groups with path, name, and visibility. See also: gitlab_group_get, gitlab_group_projects, gitlab_group_transfer_locations."
	case "gitlab_group_update":
		options.Usage = "Update an existing group's settings. Send group_id plus only the fields to change (name, path, visibility, default branch protection, merge policies, Duo, runner limits, etc.)."
		options.Aliases = []string{"update group", "edit group settings", "change group configuration"}
		options.RelatedActions = []string{actionGroupGet, "group.create", "group.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path to update.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
		}
		options.IndividualTool.Description = "Update a GitLab group's settings. Returns: the updated group metadata. See also: gitlab_group_get, gitlab_group_create, gitlab_group_delete."
	case "gitlab_group_delete":
		options.Usage = "Delete a group (marks it for deletion, or permanently removes it with permanently_remove=true). Destructive: this removes the group and all its projects. Confirm before calling."
		options.Aliases = []string{"delete group", "remove group", "destroy group"}
		options.RelatedActions = []string{actionGroupGet, "group.restore", "group.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path to delete.",
				ExampleBinding:   `params.group_id:"my-org/legacy"`,
				CommonConfusions: []string{"permanently_remove=true requires full_path and bypasses the retention window."},
			},
		}
		options.IndividualTool.Description = "Delete a GitLab group and its projects. Returns: a success confirmation. See also: gitlab_group_restore, gitlab_group_get, gitlab_group_update."
	case "gitlab_group_restore":
		options.Usage = "Restore a group that was marked for deletion (within the retention window, before permanent removal). Use after an accidental delete to recover the group."
		options.Aliases = []string{"restore group", "undelete group", "recover deleted group"}
		options.RelatedActions = []string{actionGroupGet, "group.delete", "group.list"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path that was marked for deletion.",
				ExampleBinding: `params.group_id:"my-org/legacy"`,
			},
		}
		options.IndividualTool.Description = "Restore a GitLab group marked for deletion. Returns: the restored group metadata. See also: gitlab_group_delete, gitlab_group_get, gitlab_group_list."
	case "gitlab_group_archive":
		options.Usage = "Archive a group, making it and its projects read-only. Use when the user wants to freeze a group without deleting it. Idempotent; archiving an archived group is a no-op."
		options.Aliases = []string{"archive group", "freeze group", "make group read-only"}
		options.RelatedActions = []string{actionGroupGet, "group.unarchive", "group.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path to archive.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
		}
		options.IndividualTool.Description = "Archive a GitLab group (read-only). Returns: a success confirmation. See also: gitlab_group_unarchive, gitlab_group_get, gitlab_group_update."
	case "gitlab_group_unarchive":
		options.Usage = "Unarchive a previously archived group, restoring write access. Idempotent; unarchiving a non-archived group is a no-op."
		options.Aliases = []string{"unarchive group", "unfreeze group", "restore group write access"}
		options.RelatedActions = []string{actionGroupGet, "group.archive", "group.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path to unarchive.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
		}
		options.IndividualTool.Description = "Unarchive a GitLab group (restore write access). Returns: a success confirmation. See also: gitlab_group_archive, gitlab_group_get, gitlab_group_update."
	default:
		applyGroupRelationMetadata(individualTool, &options)
	}

	return options
}

// applyGroupRelationMetadata fills in discovery metadata for the group-relation
// tools (transfer-project, shared-with, invited-groups, transfer-locations).
// Split out of groupOptionsForAction to keep its maintainability index above
// the linter threshold.
func applyGroupRelationMetadata(individualTool string, options *toolutil.ActionSpecOptions) {
	switch individualTool {
	case "gitlab_group_transfer_project":
		options.Usage = "Move an existing project into this group's namespace. Use when the user wants to relocate a project under a group. To discover which groups a group itself can be transferred into, use gitlab_group_transfer_locations instead."
		options.Aliases = []string{"transfer project to group", "move project into group", "relocate project namespace"}
		options.RelatedActions = []string{actionGroupGet, "group.transfer_locations", "group.projects"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Numeric ID or full path of the project to move.",
				ExampleBinding:   `params.project_id:"my-org/legacy-service"`,
				CommonConfusions: []string{"project_id is the project being moved; group_id is the destination group."},
			},
		}
		options.IndividualTool.Description = "Transfer a project into a GitLab group namespace. Returns: the updated project metadata. See also: gitlab_group_transfer_locations, gitlab_group_get, gitlab_group_projects."
	case "gitlab_group_shared_with_list":
		options.Usage = "List the groups that have been shared with this group (group-to-group shares granting members access). Use when the user asks which groups can access a group via sharing, not its members or subgroups."
		options.Aliases = []string{"groups shared with this group", "list shared groups", "group share grants", "who shares this group"}
		options.RelatedActions = []string{actionGroupGet, "group.members", "group.invited_groups", "group.subgroups"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path whose inbound shares are listed.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Lists groups shared *with* this group; it does not list this group's members or the projects it was shared into."},
			},
		}
		options.IndividualTool.Description = "List groups shared with a GitLab group (group-to-group shares). Returns: the shared groups with path, visibility, and access metadata. See also: gitlab_group_invited_list, gitlab_group_members_list, gitlab_subgroups_list."
	case "gitlab_group_invited_list":
		options.Usage = "List the groups invited to this group. Use when the user asks which groups were invited (directly or by inheritance) to collaborate on a group."
		options.Aliases = []string{"invited groups", "groups invited to group", "list group invitations", "group collaborators"}
		options.RelatedActions = []string{actionGroupGet, "group.shared_with", "group.members"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path whose invited groups are listed.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
			"relation": {
				ValueSource:      "Inheritance filter: direct, inherited, or both.",
				ExampleBinding:   `params.relation:["direct"]`,
				CommonConfusions: []string{"relation filters by how the invitation reaches the group; it is not an access level."},
			},
		}
		options.IndividualTool.Description = "List groups invited to a GitLab group. Returns: the invited groups with path and access metadata. See also: gitlab_group_shared_with_list, gitlab_group_members_list, gitlab_group_get."
	case "gitlab_group_transfer_locations":
		options.Usage = "List the parent groups this group can be transferred (moved) into. Use this BEFORE attempting a group transfer to discover valid destinations; the caller needs the Owner role on a destination for it to appear."
		options.Aliases = []string{"transfer locations", "where can I move this group", "candidate parent groups", "available group transfer targets", "valid destinations for group transfer"}
		options.RelatedActions = []string{actionGroupGet, "group.transfer_project", "group.subgroups"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path that would be moved.",
				ExampleBinding:   `params.group_id:"my-org/legacy-team"`,
				CommonConfusions: []string{"This lists destinations for moving the *group itself*; to move a project into a group, use gitlab_group_transfer_project."},
			},
		}
		options.IndividualTool.Description = "List candidate parent groups for transferring a GitLab group. Returns: eligible destination groups with id, name, and full path. See also: gitlab_group_transfer_project, gitlab_group_get, gitlab_subgroups_list."
	}
}

// applyGroupHookMetadata fills in discovery metadata for the five group-webhook
// tools. It returns true when it handled individualTool, letting the caller
// short-circuit. Split out of groupOptionsForAction to keep that function's
// cyclomatic complexity flat.
func applyGroupHookMetadata(individualTool string, options *toolutil.ActionSpecOptions) bool {
	switch individualTool {
	case "gitlab_group_hook_list":
		options.Usage = "List the webhooks configured on a group. Use when the user asks which webhooks fire for a group and its subgroups/projects. Requires Owner role."
		options.Aliases = []string{"list group hooks", "show group webhooks", "group webhook list"}
		options.RelatedActions = []string{"group.hook_get", "group.hook_add", actionGroupGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path whose webhooks are listed.",
				ExampleBinding: `params.group_id:"my-org/platform"`,
			},
		}
		options.IndividualTool.Description = "List the webhooks on a GitLab group. Returns: hooks with URL, enabled events, and SSL settings. See also: gitlab_group_hook_get, gitlab_group_hook_add, gitlab_group_get."
	case "gitlab_group_hook_get":
		options.Usage = "Fetch a single group webhook by hook_id. Use to inspect a webhook's URL, enabled events, and SSL/header settings. Requires Owner role."
		options.Aliases = []string{"get group hook", "show group webhook", "view group webhook details"}
		options.RelatedActions = []string{"group.hook_list", "group.hook_edit", "group.hook_delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"hook_id": {
				ValueSource:    "Numeric hook ID from gitlab_group_hook_list.",
				ExampleBinding: `params.hook_id:42`,
			},
		}
		options.IndividualTool.Description = "Get a single GitLab group webhook by ID. Returns: the hook's URL, enabled events, and SSL/header metadata. See also: gitlab_group_hook_list, gitlab_group_hook_edit, gitlab_group_hook_delete."
	case "gitlab_group_hook_delete":
		options.Usage = "Delete a group webhook by hook_id. Destructive and irreversible. Confirm before calling. Requires Owner role."
		options.Aliases = []string{"delete group hook", "remove group webhook", "destroy group webhook"}
		options.RelatedActions = []string{"group.hook_list", "group.hook_get", "group.hook_add"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"hook_id": {
				ValueSource:    "Numeric hook ID from gitlab_group_hook_list.",
				ExampleBinding: `params.hook_id:42`,
			},
		}
		options.IndividualTool.Description = "Delete a GitLab group webhook. Returns: a success confirmation. See also: gitlab_group_hook_list, gitlab_group_hook_get, gitlab_group_hook_add."
	case "gitlab_group_hook_add", "gitlab_group_hook_edit":
		applyGroupHookAddEditMetadata(individualTool, options)
	default:
		return false
	}
	return true
}

// applyGroupHookAddEditMetadata fills in shared and per-tool metadata for the
// hook add/edit tools, including the branch_filter_strategy enum override.
func applyGroupHookAddEditMetadata(individualTool string, options *toolutil.ActionSpecOptions) {
	// branch_filter_strategy is an enum on the GitLab side (wildcard, regex,
	// all_branches). The jsonschema tag on HookInput already lists the three
	// values in the description; this override adds the same values as a proper
	// schema enum so the LLM can rely on validation rather than just textual
	// inference.
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("branch_filter_strategy", map[string]any{
			"enum": []any{"wildcard", "regex", "all_branches"},
		}),
	}
	options.RelatedActions = []string{"group.hook_list", "group.hook_get", "group.hook_delete"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"url": {
			ValueSource:      "HTTP(S) endpoint that should receive webhook payloads.",
			ExampleBinding:   `params.url:"https://ci.example.com/gitlab-hook"`,
			CommonConfusions: []string{"Enable specific event flags (push_events, merge_requests_events, etc.); a hook with no events fires nothing."},
		},
	}
	if individualTool == "gitlab_group_hook_add" {
		options.Usage = "Create a group webhook. Send url plus the event flags to enable (push_events, merge_requests_events, pipeline_events, etc.) and optional token/custom_headers/custom_webhook_template. Requires Owner role."
		options.Aliases = []string{"add group hook", "create group webhook", "register group webhook"}
		options.IndividualTool.Description = "Create a webhook on a GitLab group. Returns: the created hook with URL, enabled events, and SSL/header metadata. See also: gitlab_group_hook_list, gitlab_group_hook_get, gitlab_group_hook_delete."
		return
	}
	options.Usage = "Update an existing group webhook by hook_id. Send only the fields to change; unset fields keep their current values. Requires Owner role."
	options.Aliases = []string{"edit group hook", "update group webhook", "modify group webhook"}
	options.IndividualTool.Description = "Update a GitLab group webhook. Returns: the updated hook with URL, enabled events, and SSL/header metadata. See also: gitlab_group_hook_list, gitlab_group_hook_get, gitlab_group_hook_delete."
}

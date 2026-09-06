package projects

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionProjectGet              = "project.get"
	actionProjectList             = "project.list"
	actionProjectUpdate           = "project.update"
	actionProjectDelete           = "project.delete"
	actionProjectFork             = "project.fork"
	actionProjectArchive          = "project.archive"
	actionProjectHookList         = "project.hook_list"
	actionProjectHookGet          = "project.hook_get"
	actionProjectHookEdit         = "project.hook_edit"
	actionProjectApprovalRuleList = "project.approval_rule_list"
	actionProjectApprovalRuleGet  = "project.approval_rule_get"
	actionProjectApprovalCfgGet   = "project.approval_config_get"
	actionProjectListUserStarred  = "project.list_user_starred"
	actionProjectListUserProjects = "project.list_user_projects"
	actionProjectListStarrers     = "project.list_starrers"
	actionProjectListInvGroups    = "project.list_invited_groups"
	actionProjectListForks        = "project.list_forks"
	actionProjectGetPushRules     = "project.get_push_rules"
	actionProjectEditPushRule     = "project.edit_push_rule"
	actionProjectDeletePushRule   = "project.delete_push_rule"
	actionProjectCreateForkRel    = "project.create_fork_relation"
	actionProjectAddPushRule      = "project.add_push_rule"
	actionGroupGet                = "group.get"
	statusSuccess                 = "success"
	tagWebhook                    = "webhook"
	tagApproval                   = "approval"
	tagUser                       = "user"
	tagPushRule                   = "push_rule"
	tagGroup                      = "group"
	tagMirror                     = "mirror"
	tagTargetBranchRule           = "target_branch_rule"
	paramProjectID                = "project_id"
	toolProjectDelete             = "gitlab_project_delete"
	toolProjectList               = "gitlab_project_list"
	toolProjectEditPushRule       = "gitlab_project_edit_push_rule"
	toolProjectAddPushRule        = "gitlab_project_add_push_rule"
)

// ActionSpecs returns canonical specs for project lifecycle and settings actions.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		projectCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_project_create"),
		projectGetSpec(projectGetRoute(client)).
			WithEmbeddedResource("gitlab://project/{project_id}"),
		projectReadSpec("list", toolutil.RouteAction(client, List), toolProjectList),
		projectDestructiveUpdateSpec("delete", toolutil.DestructiveAction(client, Delete), toolProjectDelete),
		projectMutationSpec("restore", toolutil.RouteAction(client, Restore), "gitlab_project_restore"),
		projectMutationSpec("update", toolutil.RouteAction(client, Update), "gitlab_project_update"),
		projectCreateSpec("fork", toolutil.RouteAction(client, Fork), "gitlab_project_fork"),
		projectCreateSpec("star", toolutil.RouteAction(client, Star), "gitlab_project_star"),
		projectMutationSpec("unstar", toolutil.RouteAction(client, Unstar), "gitlab_project_unstar"),
		projectMutationSpec("archive", toolutil.RouteAction(client, Archive), "gitlab_project_archive"),
		projectMutationSpec("unarchive", toolutil.RouteAction(client, Unarchive), "gitlab_project_unarchive"),
		projectDestructiveUpdateSpec("transfer", toolutil.RouteAction(client, Transfer), "gitlab_project_transfer"),
		projectReadSpec("list_forks", toolutil.RouteAction(client, ListForks), "gitlab_project_list_forks"),
		projectReadSpec("languages", toolutil.RouteAction(client, GetLanguages), "gitlab_project_languages"),
		projectReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_project_hook_list", tagWebhook),
		projectReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_project_hook_get", tagWebhook),
		projectCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_project_hook_add", tagWebhook),
		projectMutationSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_project_hook_edit", tagWebhook),
		projectDestructiveUpdateSpec("hook_delete", toolutil.RouteAction(client, DeleteHookOutput), "gitlab_project_hook_delete", tagWebhook),
		projectMutationSpec("hook_test", toolutil.RouteAction(client, TriggerTestHook), "gitlab_project_hook_test", tagWebhook),
		projectReadSpec("list_user_projects", toolutil.RouteAction(client, ListUserProjects), "gitlab_project_list_user_projects", tagUser),
		projectReadSpec("list_users", toolutil.RouteAction(client, ListProjectUsers), "gitlab_project_list_users", tagUser),
		projectReadSpec("list_groups", toolutil.RouteAction(client, ListProjectGroups), "gitlab_project_list_groups", tagGroup),
		projectReadSpec("list_starrers", toolutil.RouteAction(client, ListProjectStarrers), "gitlab_project_list_starrers", tagUser),
		projectCreateSpec("share_with_group", toolutil.RouteAction(client, ShareProjectWithGroup), "gitlab_project_share_with_group", tagGroup),
		projectDestructiveUpdateSpec("delete_shared_group", toolutil.RouteAction(client, DeleteSharedGroupOutput), "gitlab_project_delete_shared_group", tagGroup),
		projectReadSpec("list_invited_groups", toolutil.RouteAction(client, ListInvitedGroups), "gitlab_project_list_invited_groups", tagGroup),
		projectReadSpec("list_user_contributed", toolutil.RouteAction(client, ListUserContributedProjects), "gitlab_project_list_user_contributed", tagUser),
		projectReadSpec("list_user_starred", toolutil.RouteAction(client, ListUserStarredProjects), "gitlab_project_list_user_starred", tagUser),
		projectMutationSpec("hook_set_custom_header", toolutil.RouteAction(client, SetCustomHeaderOutput), "gitlab_project_hook_set_custom_header", tagWebhook),
		projectDestructiveUpdateSpec("hook_delete_custom_header", toolutil.RouteAction(client, DeleteCustomHeaderOutput), "gitlab_project_hook_delete_custom_header", tagWebhook),
		projectMutationSpec("hook_set_url_variable", toolutil.RouteAction(client, SetWebhookURLVariableOutput), "gitlab_project_hook_set_url_variable", tagWebhook),
		projectDestructiveUpdateSpec("hook_delete_url_variable", toolutil.RouteAction(client, DeleteWebhookURLVariableOutput), "gitlab_project_hook_delete_url_variable", tagWebhook),
		projectCreateSpec("create_fork_relation", toolutil.RouteAction(client, CreateForkRelation), "gitlab_project_create_fork_relation"),
		projectDestructiveUpdateSpec("delete_fork_relation", toolutil.RouteAction(client, DeleteForkRelationOutput), "gitlab_project_delete_fork_relation"),
		projectMutationSpec("upload_avatar", toolutil.RouteAction(client, UploadAvatar), "gitlab_project_upload_avatar", "avatar"),
		projectReadSpec("download_avatar", toolutil.RouteAction(client, DownloadAvatar), "gitlab_project_download_avatar", "avatar"),
		projectPremiumSpec(projectReadSpec("approval_config_get", toolutil.RouteAction(client, GetApprovalConfig), "gitlab_project_approval_config_get", tagApproval)),
		projectPremiumSpec(projectMutationSpec("approval_config_change", toolutil.RouteAction(client, ChangeApprovalConfig), "gitlab_project_approval_config_change", tagApproval)),
		projectPremiumSpec(projectReadSpec("approval_rule_list", toolutil.RouteAction(client, ListApprovalRules), "gitlab_project_approval_rule_list", tagApproval)),
		projectPremiumSpec(projectReadSpec("approval_rule_get", toolutil.RouteAction(client, GetApprovalRule), "gitlab_project_approval_rule_get", tagApproval)),
		projectPremiumSpec(projectCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateApprovalRule), "gitlab_project_approval_rule_create", tagApproval)),
		projectPremiumSpec(projectMutationSpec("approval_rule_update", toolutil.RouteAction(client, UpdateApprovalRule), "gitlab_project_approval_rule_update", tagApproval)),
		projectPremiumSpec(projectDestructiveUpdateSpec("approval_rule_delete", toolutil.RouteAction(client, DeleteApprovalRuleOutput), "gitlab_project_approval_rule_delete", tagApproval)),
		projectPremiumSpec(projectReadSpec("pull_mirror_get", toolutil.RouteAction(client, GetPullMirror), "gitlab_project_pull_mirror_get", tagMirror)),
		projectPremiumSpec(projectMutationSpec("pull_mirror_configure", toolutil.RouteAction(client, ConfigurePullMirror), "gitlab_project_pull_mirror_configure", tagMirror)),
		projectPremiumSpec(projectMutationSpec("start_mirroring", toolutil.RouteAction(client, StartMirroringOutput), "gitlab_project_start_mirroring", tagMirror)),
		projectMutationSpec("start_housekeeping", toolutil.RouteAction(client, StartHousekeepingOutput), "gitlab_project_start_housekeeping"),
		projectReadSpec("repository_storage_get", toolutil.RouteAction(client, GetRepositoryStorage), "gitlab_project_repository_storage_get"),
		projectCreateSpec("create_for_user", toolutil.RouteAction(client, CreateForUser), "gitlab_project_create_for_user", "admin"),
	}
	if enterprise {
		specs = append(
			specs,
			projectPremiumSpec(projectReadSpec("push_rule_get", toolutil.RouteAction(client, GetPushRules), "gitlab_project_get_push_rules", tagPushRule)),
			projectPremiumSpec(projectCreateSpec("push_rule_add", toolutil.RouteAction(client, AddPushRule), toolProjectAddPushRule, tagPushRule)),
			projectPremiumSpec(projectMutationSpec("push_rule_edit", toolutil.RouteAction(client, EditPushRule), toolProjectEditPushRule, tagPushRule)),
			projectPremiumSpec(projectDestructiveUpdateSpec("push_rule_delete", toolutil.RouteAction(client, DeletePushRuleOutput), "gitlab_project_delete_push_rule", tagPushRule)),
			projectPremiumSpec(projectReadSpec("target_branch_rule_list", toolutil.RouteAction(client, ListTargetBranchRules), "gitlab_project_list_target_branch_rules", tagTargetBranchRule)),
			projectPremiumSpec(projectCreateSpec("target_branch_rule_create", toolutil.RouteAction(client, CreateTargetBranchRule), "gitlab_project_create_target_branch_rule", tagTargetBranchRule)),
			projectPremiumSpec(projectDestructiveUpdateSpec("target_branch_rule_delete", toolutil.RouteAction(client, DeleteTargetBranchRuleOutput), "gitlab_project_delete_target_branch_rule", tagTargetBranchRule)),
		)
	}
	return specs
}

func projectGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				return projectNotFoundOutput{Identifier: fmt.Sprint(input[paramProjectID])}, nil
			}
			return result, err
		}
	})
}

// DeleteHookOutput deletes a project webhook and returns the legacy success message shape.
func DeleteHookOutput(ctx context.Context, client *gitlabclient.Client, input DeleteHookInput) (toolutil.DeleteOutput, error) {
	if err := DeleteHook(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: fmt.Sprintf("Successfully deleted webhook %d from project %s.", input.HookID, input.ProjectID)}, nil
}

// DeleteSharedGroupOutput removes a shared group and returns the legacy success message shape.
func DeleteSharedGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteSharedGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteSharedProjectFromGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: fmt.Sprintf("Successfully deleted shared group %d from project %s.", input.GroupID, input.ProjectID)}, nil
}

// SetCustomHeaderOutput sets a webhook custom header and returns the legacy success message shape.
func SetCustomHeaderOutput(ctx context.Context, client *gitlabclient.Client, input SetCustomHeaderInput) (toolutil.VoidOutput, error) {
	if err := SetCustomHeader(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Custom header %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteCustomHeaderOutput deletes a webhook custom header and returns the legacy success message shape.
func DeleteCustomHeaderOutput(ctx context.Context, client *gitlabclient.Client, input DeleteCustomHeaderInput) (toolutil.VoidOutput, error) {
	if err := DeleteCustomHeader(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Custom header %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// SetWebhookURLVariableOutput sets a webhook URL variable and returns the legacy success message shape.
func SetWebhookURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input SetWebhookURLVariableInput) (toolutil.VoidOutput, error) {
	if err := SetWebhookURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("URL variable %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteWebhookURLVariableOutput deletes a webhook URL variable and returns the legacy success message shape.
func DeleteWebhookURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input DeleteWebhookURLVariableInput) (toolutil.VoidOutput, error) {
	if err := DeleteWebhookURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("URL variable %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteForkRelationOutput removes a fork relation and returns the legacy success message shape.
func DeleteForkRelationOutput(ctx context.Context, client *gitlabclient.Client, input DeleteForkRelationInput) (toolutil.VoidOutput, error) {
	if err := DeleteForkRelation(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Fork relation removed from project %s", input.ProjectID)}, nil
}

// DeleteApprovalRuleOutput deletes a project approval rule and returns the legacy success message shape.
func DeleteApprovalRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteApprovalRuleInput) (toolutil.VoidOutput, error) {
	if err := DeleteApprovalRule(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Approval rule %d deleted from project %s", input.RuleID, input.ProjectID)}, nil
}

// StartMirroringOutput starts pull mirroring and returns the legacy success message shape.
func StartMirroringOutput(ctx context.Context, client *gitlabclient.Client, input StartMirroringInput) (toolutil.VoidOutput, error) {
	if err := StartMirroring(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Mirror update triggered for project %s", input.ProjectID)}, nil
}

// StartHousekeepingOutput starts housekeeping and returns the legacy success message shape.
func StartHousekeepingOutput(ctx context.Context, client *gitlabclient.Client, input StartHousekeepingInput) (toolutil.VoidOutput, error) {
	if err := StartHousekeeping(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: statusSuccess, Message: fmt.Sprintf("Housekeeping started for project %s", input.ProjectID)}, nil
}

// DeletePushRuleOutput deletes project push rules and returns the legacy success message shape.
func DeletePushRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeletePushRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeletePushRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: statusSuccess, Message: fmt.Sprintf("Successfully deleted push rules for project %s.", input.ProjectID)}, nil
}

func projectPremiumSpec(spec toolutil.ActionSpec) toolutil.ActionSpec {
	spec.Edition = "premium"
	return spec
}

func projectGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := projectOptions("gitlab_project_get")
	options.Usage = "Get one exact project by numeric ID or full namespace path. Use this when the prompt gives a concrete path like group/project and asks to find, show, verify, or read project metadata such as id or default_branch. Do not use search.projects for an exact path lookup."
	options.Aliases = []string{"gitlab_project_get", "get project", "show project", "look up project", "fetch project metadata"}
	options.RelatedActions = []string{actionProjectArchive, actionProjectDelete, actionProjectUpdate}
	options.IndividualTool.Description = "Get a single project by ID or namespace path. Returns: project metadata including id, path_with_namespace, default_branch, visibility, and web URL. See also: gitlab_project_update, gitlab_project_archive, gitlab_project_delete."
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		paramProjectID: {
			SemanticRole:   "scope_project",
			ValueSource:    "Concrete numeric project ID or full namespace path from the prompt or a prior project result.",
			ExampleBinding: `params.project_id:"my-org/tools/gitlab-mcp-server"`,
			CommonConfusions: []string{
				"Use project_id for a namespace path. Do not substitute full_path, path, remote_url, search, or query.",
				"A namespace path like group/project goes here directly. Do not call discover_project.resolve for it (that action is only for a full git remote URL).",
			},
		},
	}
	return toolutil.NewReadActionSpec("get", route, options)
}

func projectReadSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := projectOptionsForAction(name, individualTool, extraTags...)
	return toolutil.NewReadActionSpec(name, route, options)
}

func projectCreateSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectOptions(individualTool, extraTags...))
}

func projectMutationSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, projectOptions(individualTool, extraTags...))
}

func projectDestructiveUpdateSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, projectOptions(individualTool, extraTags...))
}

func projectOptions(individualTool string, extraTags ...string) toolutil.ActionSpecOptions {
	return projectOptionsForAction("", individualTool, extraTags...)
}

func projectOptionsForAction(actionName, individualTool string, extraTags ...string) toolutil.ActionSpecOptions {
	tags := append([]string{"project"}, extraTags...)
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute projects domain action.", Tags: tags,
		OpenWorld:      true,
		OwnerPackage:   "projects",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if slices.Contains(extraTags, tagPushRule) {
		options.Usage = "Use project push rule actions for the singleton project-level push rule. Do not pass push_rule_id. Get/add/edit/delete operate by project_id only. Use reject_unsigned_commits, not deny_unsigned_commits."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramProjectID: {
				SemanticRole:     "scope_project",
				ValueSource:      "Project that owns the singleton push rule.",
				CommonConfusions: []string{"Push rules are project-scoped singletons. There is no push_rule_id parameter."},
			},
		}
		if individualTool == toolProjectAddPushRule || individualTool == toolProjectEditPushRule {
			options.ParameterGuidance["commit_message_regex"] = toolutil.ParameterGuidance{
				ValueSource:      "Commit message regex requested by the user.",
				ExampleBinding:   `params.commit_message_regex:"^EVAL-"`,
				CommonConfusions: []string{"Use commit_message_regex directly. Do not send commit_message_regex_enabled or empty regex placeholders."},
			}
			options.ParameterGuidance["reject_unsigned_commits"] = toolutil.ParameterGuidance{
				ValueSource:      "Boolean that enables unsigned commit rejection.",
				ExampleBinding:   "params.reject_unsigned_commits:true",
				CommonConfusions: []string{"Use reject_unsigned_commits, not deny_unsigned_commits."},
			}
		}
		if individualTool == toolProjectAddPushRule {
			options.Usage += " For add, include at least one rule-setting parameter such as commit_message_regex, reject_unsigned_commits, prevent_secrets, branch_name_regex, or deny_delete_tag. Do not call add with project_id alone."
		}
	}
	if individualTool == toolProjectDelete {
		options.Usage = "Use to delete a project. For ordinary cleanup, send project_id and confirm only. Set permanently_remove only when explicitly requested. When permanently_remove is true, full_path must exactly match the project's path_with_namespace from project create/get."
	}
	if actionName == "list" && individualTool == toolProjectList {
		options.Usage = "List projects accessible to the authenticated user. For most recently updated projects, use order_by last_activity_at with sort desc and per_page for the requested count. Do not use last_activity_after as an order_by value."
		options.Aliases = []string{toolProjectList, "list projects", "show my projects", "browse repositories"}
		options.RelatedActions = []string{actionProjectGet, actionProjectListUserProjects, "search.projects"}
		options.IndividualTool.Description = "List projects accessible to the user with filtering and pagination. Returns: projects with namespace, visibility, last_activity_at, and pagination metadata. See also: gitlab_project_get, gitlab_project_list_user_projects."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"order_by": {
				SemanticRole: "project_list_sort_field",
				ValueSource:  "Use only GitLab project list ordering fields accepted by the projects API.",
				CommonConfusions: []string{
					"Use last_activity_at to sort by recent activity. last_activity_after is a date filter, not an order_by value.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("order_by", map[string]any{
				"enum":        []any{"id", "name", "path", "created_at", "updated_at", "last_activity_at", "similarity", "star_count"},
				"description": "Order projects by id, name, path, created_at, updated_at, last_activity_at, similarity (requires search query), or star_count.",
			}),
			toolutil.SchemaPropertyOverride("sort", map[string]any{"enum": []any{"asc", "desc"}}),
		}
	}
	decorateProjectMeta(&options, individualTool)
	decorateProjectSchemaOverrides(&options, individualTool)
	return options
}

// projectActionMetaEntry is the discovery metadata for one project action.
type projectActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// decorateProjectMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for every project action that would otherwise inherit the generic placeholder
// metadata from projectOptions. Actions whose dedicated builders already set
// rich metadata (get, list) are absent from the map and left untouched. For
// actions that already carry a tailored Usage (push rules, delete) only the
// missing aliases/related/description are filled, preserving the existing Usage.
func decorateProjectMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := projectActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// projectActionMeta maps each individual project tool to its discovery metadata.
// Entries omit the usage field when projectOptionsForAction already sets a
// tailored Usage (push rules, delete) so that bespoke text is preserved while
// aliases, related actions, and the individual-tool description are still added.
//
//nolint:funlen // A flat, reviewable lookup table; one entry per project action.
var projectActionMeta = map[string]projectActionMetaEntry{
	"gitlab_project_create": {
		usage:       "Create a new project in a namespace you can write to. Provide name (and optionally path, namespace_id, visibility, description). Use create_for_user to create on behalf of another user as admin.",
		aliases:     []string{"create project", "new project", "add project", "make a repository"},
		related:     []string{actionProjectGet, actionProjectUpdate, "project.create_for_user"},
		description: "Create a new GitLab project. Returns: the created project with id, path_with_namespace, default_branch, visibility, and web URL. See also: gitlab_project_get, gitlab_project_update, gitlab_project_create_for_user.",
	},
	"gitlab_project_create_for_user": {
		usage:       "Create a project owned by a specific user (admin only). Provide user_id and name. The project is created in that user's personal namespace.",
		aliases:     []string{"create project for user", "create project as admin", "provision user project", "admin create project"},
		related:     []string{"project.create", actionProjectGet, "project.transfer"},
		description: "Create a project on behalf of a user (admin). Returns: the created project with id, owner namespace, visibility, and web URL. See also: gitlab_project_create, gitlab_project_get.",
	},
	"gitlab_project_update": {
		usage:       "Update settings on an existing project such as name, description, visibility, default_branch, merge_method, feature access levels, or CI/CD options. Send project_id plus only the fields to change.",
		aliases:     []string{"update project", "edit project settings", "change project configuration", "modify project settings"},
		related:     []string{actionProjectGet, actionProjectArchive, "project.transfer"},
		description: "Update a project's settings. Returns: the updated project with its current configuration and web URL. See also: gitlab_project_get, gitlab_project_archive, gitlab_project_transfer.",
	},
	"gitlab_project_restore": {
		usage:       "Restore a project previously marked for delayed deletion, before the retention period expires. Send the project_id of the pending-deletion project.",
		aliases:     []string{"restore project", "undelete project", "recover deleted project"},
		related:     []string{actionProjectDelete, actionProjectGet, actionProjectList},
		description: "Restore a project marked for deletion. Returns: the restored project. See also: gitlab_project_delete, gitlab_project_get.",
	},
	"gitlab_project_fork": {
		usage:       "Fork a project into your namespace or a chosen namespace. Optionally set name, path, namespace_id/namespace_path, visibility, and branches to fork.",
		aliases:     []string{"fork project", "fork repository", "create a fork"},
		related:     []string{actionProjectListForks, actionProjectCreateForkRel, "project.delete_fork_relation"},
		description: "Fork a project. Returns: the new fork with id, path_with_namespace, forked_from_project, and web URL. See also: gitlab_project_list_forks, gitlab_project_create_fork_relation.",
	},
	"gitlab_project_star": {
		usage:       "Star a project for the authenticated user. Send project_id. Idempotent if already starred.",
		aliases:     []string{"star project", "add star to project", "favorite project"},
		related:     []string{"project.unstar", actionProjectListStarrers, actionProjectListUserStarred},
		description: "Star a project. Returns: the project with the updated star_count. See also: gitlab_project_unstar, gitlab_project_list_starrers.",
	},
	"gitlab_project_unstar": {
		usage:       "Remove the authenticated user's star from a project. Send project_id. Idempotent if not currently starred.",
		aliases:     []string{"unstar project", "remove star from project", "unfavorite project"},
		related:     []string{"project.star", actionProjectListStarrers, actionProjectListUserStarred},
		description: "Unstar a project. Returns: the project with the updated star_count. See also: gitlab_project_star, gitlab_project_list_starrers.",
	},
	"gitlab_project_archive": {
		usage:       "Archive a project so it becomes read-only. Send project_id. Requires Owner role. Use unarchive to reverse.",
		aliases:     []string{"archive project", "make project read-only", "freeze project", "lock project"},
		related:     []string{"project.unarchive", actionProjectGet, actionProjectDelete},
		description: "Archive a project, making it read-only in GitLab. Returns: the project with archived set to true. See also: gitlab_project_unarchive, gitlab_project_get.",
	},
	"gitlab_project_unarchive": {
		usage:       "Unarchive a previously archived project to restore write access. Send project_id. Requires Owner role.",
		aliases:     []string{"unarchive project", "restore archived project", "make project writable"},
		related:     []string{actionProjectArchive, actionProjectGet},
		description: "Unarchive a project. Returns: the project with archived set to false. See also: gitlab_project_archive, gitlab_project_get.",
	},
	"gitlab_project_transfer": {
		usage:       "Transfer a project to a different namespace (group or user). Send project_id and the target namespace ID or full path. Requires Owner on the source and create-project rights on the target.",
		aliases:     []string{"transfer project", "move project to another namespace", "change project group"},
		related:     []string{actionProjectGet, actionProjectUpdate, actionGroupGet},
		description: "Transfer a project to another namespace. Returns: the project in its new namespace. See also: gitlab_project_get, gitlab_group_get.",
	},
	"gitlab_project_list_forks": {
		usage:       "List the forks of a project with the same filters as the project list endpoint (visibility, owned, search, order_by, archived, and more). Send project_id.",
		aliases:     []string{"list project forks", "show forks of a project", "forks of this repository"},
		related:     []string{actionProjectFork, actionProjectGet, actionProjectCreateForkRel},
		description: "List a project's forks. Returns: forked projects with namespace, visibility, and pagination metadata. See also: gitlab_project_fork, gitlab_project_get.",
	},
	"gitlab_project_languages": {
		usage:       "Get the programming-language breakdown of a project's repository as language name and percentage. Send project_id. Empty repositories return no languages.",
		aliases:     []string{"project languages", "language breakdown", "repository languages"},
		related:     []string{actionProjectGet, actionProjectList},
		description: "Get a project's detected languages. Returns: language names with their percentage of the codebase. See also: gitlab_project_get.",
	},
	"gitlab_project_list_user_projects": {
		usage:       "List projects owned by a specific user. Send user_id (numeric ID or username) plus optional filters such as visibility, archived, search, and order_by.",
		aliases:     []string{"list a user's projects", "projects owned by user", "user's repositories"},
		related:     []string{actionProjectList, "project.list_user_contributed", actionProjectListUserStarred},
		description: "List a user's owned projects. Returns: projects with namespace, visibility, and pagination metadata. See also: gitlab_project_list_user_contributed, gitlab_project_list_user_starred.",
	},
	"gitlab_project_list_users": {
		usage:       "List the users who are members of a project. Send project_id and optionally search by name or username.",
		aliases:     []string{"list project users", "project user accounts", "enumerate project users"},
		related:     []string{actionProjectGet, "member.list", "project.list_groups"},
		description: "List a project's users. Returns: users with id, username, name, and state, plus pagination metadata. See also: gitlab_project_get, gitlab_project_members_list.",
	},
	"gitlab_project_list_groups": {
		usage:       "List the ancestor and shared groups associated with a project. Send project_id. Optionally filter by search, shared visibility, or minimum access level.",
		aliases:     []string{"list project groups", "project ancestor groups", "groups linked to project"},
		related:     []string{actionProjectGet, actionProjectListInvGroups, actionGroupGet},
		description: "List a project's associated groups. Returns: groups with id, full_path, and full_name, plus pagination metadata. See also: gitlab_project_list_invited_groups, gitlab_group_get.",
	},
	"gitlab_project_list_starrers": {
		usage:       "List the users who have starred a project, with the date each starred it. Send project_id and optionally search by name or username.",
		aliases:     []string{"list project starrers", "who starred this project", "project stargazers"},
		related:     []string{"project.star", "project.unstar", actionProjectListUserStarred},
		description: "List a project's starrers. Returns: users with their starred_since date, plus pagination metadata. See also: gitlab_project_star, gitlab_project_list_user_starred.",
	},
	"gitlab_project_share_with_group": {
		usage:       "Share a project with a group at a chosen access level. Send project_id, group_id, and group_access (10 Guest, 20 Reporter, 30 Developer, 40 Maintainer). Optionally expires_at (YYYY-MM-DD).",
		aliases:     []string{"share project with group", "grant group access to project", "add group to project"},
		related:     []string{"project.delete_shared_group", actionProjectListInvGroups, actionGroupGet},
		description: "Share a project with a group. Returns: a confirmation with the group ID and granted access role. See also: gitlab_project_delete_shared_group, gitlab_project_list_invited_groups.",
	},
	"gitlab_project_delete_shared_group": {
		usage:       "Remove a group's share link from a project, revoking that group's inherited access. Send project_id and group_id.",
		aliases:     []string{"unshare project from group", "remove group from project", "revoke group project access"},
		related:     []string{"project.share_with_group", actionProjectListInvGroups},
		description: "Remove a group share from a project. Returns: a success confirmation naming the group and project. See also: gitlab_project_share_with_group, gitlab_project_list_invited_groups.",
	},
	"gitlab_project_list_invited_groups": {
		usage:       "List the groups invited to (shared with) a project. Send project_id. Optionally filter by search, relation, or minimum access level.",
		aliases:     []string{"list invited groups", "groups shared with project", "project group invitations"},
		related:     []string{"project.share_with_group", "project.delete_shared_group", "project.list_groups"},
		description: "List groups invited to a project. Returns: invited groups with id, full_path, and full_name, plus pagination metadata. See also: gitlab_project_share_with_group, gitlab_project_list_groups.",
	},
	"gitlab_project_list_user_contributed": {
		usage:       "List projects a specific user has contributed to. Send user_id (numeric ID or username) plus optional filters such as visibility, archived, search, and order_by.",
		aliases:     []string{"list user contributed projects", "projects a user contributed to", "user contribution repositories"},
		related:     []string{actionProjectListUserProjects, actionProjectListUserStarred, actionProjectList},
		description: "List projects a user contributed to. Returns: projects with namespace, visibility, and pagination metadata. See also: gitlab_project_list_user_projects, gitlab_project_list_user_starred.",
	},
	"gitlab_project_list_user_starred": {
		usage:       "List projects a specific user has starred. Send user_id (numeric ID or username) plus optional filters such as visibility, archived, search, and order_by.",
		aliases:     []string{"list user starred projects", "projects a user starred", "user's favorite repositories"},
		related:     []string{actionProjectListUserProjects, "project.list_user_contributed", actionProjectListStarrers},
		description: "List projects a user starred. Returns: projects with namespace, visibility, and pagination metadata. See also: gitlab_project_list_user_projects, gitlab_project_list_starrers.",
	},
	"gitlab_project_create_fork_relation": {
		usage:       "Create a fork relationship linking a project to an upstream source project. Send project_id and the upstream fork_id (or forked_from_id).",
		aliases:     []string{"create fork relation", "link fork to upstream", "set forked-from project"},
		related:     []string{"project.delete_fork_relation", actionProjectFork, actionProjectListForks},
		description: "Create a fork relationship to an upstream project. Returns: a confirmation of the new relation. See also: gitlab_project_delete_fork_relation, gitlab_project_fork.",
	},
	"gitlab_project_delete_fork_relation": {
		usage:       "Remove a project's fork relationship to its upstream source, detaching it from the original project. Send project_id.",
		aliases:     []string{"delete fork relation", "unlink fork from upstream", "detach fork"},
		related:     []string{actionProjectCreateForkRel, actionProjectFork, actionProjectListForks},
		description: "Remove a project's fork relationship. Returns: a success confirmation naming the project. See also: gitlab_project_create_fork_relation, gitlab_project_fork.",
	},
	"gitlab_project_upload_avatar": {
		usage:       "Upload an avatar image for a project. Send project_id and the image content/path. Replaces any existing avatar.",
		aliases:     []string{"upload project avatar", "set project avatar", "change project image"},
		related:     []string{"project.download_avatar", actionProjectUpdate, actionProjectGet},
		description: "Upload a project avatar. Returns: the project with its new avatar_url. See also: gitlab_project_download_avatar, gitlab_project_update.",
	},
	"gitlab_project_download_avatar": {
		usage:       "Download the avatar image for a project. Send project_id. Returns the raw image bytes.",
		aliases:     []string{"download project avatar", "get project avatar image", "fetch project avatar", "save project avatar"},
		related:     []string{"project.upload_avatar", actionProjectGet},
		description: "Download a project's avatar. Returns: the avatar image content. See also: gitlab_project_upload_avatar, gitlab_project_get.",
	},
	"gitlab_project_start_housekeeping": {
		usage:       "Trigger Git housekeeping (garbage collection and repository optimization) for a project. Send project_id. Requires Owner role.",
		aliases:     []string{"start project housekeeping", "run repository garbage collection", "optimize project repository"},
		related:     []string{"project.repository_storage_get", actionProjectGet},
		description: "Start housekeeping for a project. Returns: a success confirmation naming the project. See also: gitlab_project_repository_storage_get, gitlab_project_get.",
	},
	"gitlab_project_repository_storage_get": {
		usage:       "Get the repository storage shard a project is stored on (admin only). Send project_id.",
		aliases:     []string{"get repository storage", "project storage shard", "where is project stored"},
		related:     []string{"project.start_housekeeping", actionProjectGet},
		description: "Get a project's repository storage shard. Returns: the storage shard name. See also: gitlab_project_start_housekeeping, gitlab_project_get.",
	},
	"gitlab_project_hook_list": {
		usage:       "List the webhooks configured on a project. Send project_id. Requires Maintainer or Owner role.",
		aliases:     []string{"list project webhooks", "show project hooks", "project webhook list"},
		related:     []string{actionProjectHookGet, "project.hook_add", actionProjectHookEdit},
		description: "List a project's webhooks. Returns: webhooks with URL, enabled events, and SSL settings, plus pagination metadata. See also: gitlab_project_hook_get, gitlab_project_hook_add.",
	},
	"gitlab_project_hook_get": {
		usage:       "Get one project webhook by hook_id, including its enabled events and configuration. Send project_id and hook_id.",
		aliases:     []string{"get project webhook", "show project hook", "inspect webhook"},
		related:     []string{actionProjectHookList, actionProjectHookEdit, "project.hook_delete"},
		description: "Get a single project webhook. Returns: the webhook with URL, enabled events, and SSL settings. See also: gitlab_project_hook_list, gitlab_project_hook_edit.",
	},
	"gitlab_project_hook_add": {
		usage:       "Add a webhook to a project. Send project_id and url plus the event flags to enable (push_events, merge_requests_events, etc.). Production GitLab requires HTTPS. Requires Maintainer role.",
		aliases:     []string{"add project webhook", "create project hook", "register webhook"},
		related:     []string{actionProjectHookList, actionProjectHookEdit, "project.hook_test"},
		description: "Add a webhook to a project. Returns: the created webhook with id, URL, and enabled events. See also: gitlab_project_hook_list, gitlab_project_hook_test.",
	},
	"gitlab_project_hook_edit": {
		usage:       "Edit an existing project webhook's URL, token, or enabled events. Send project_id, hook_id, and the fields to change.",
		aliases:     []string{"edit project webhook", "update project hook", "change webhook settings"},
		related:     []string{actionProjectHookGet, actionProjectHookList, "project.hook_test"},
		description: "Edit a project webhook. Returns: the updated webhook with URL and enabled events. See also: gitlab_project_hook_get, gitlab_project_hook_test.",
	},
	"gitlab_project_hook_delete": {
		usage:       "Delete a project webhook by hook_id. Send project_id and hook_id. Irreversible.",
		aliases:     []string{"delete project webhook", "remove project hook", "unregister webhook"},
		related:     []string{actionProjectHookList, actionProjectHookGet, "project.hook_add"},
		description: "Delete a project webhook. Returns: a success confirmation naming the webhook and project. See also: gitlab_project_hook_list, gitlab_project_hook_add.",
	},
	"gitlab_project_hook_test": {
		usage:       "Trigger a test event for a project webhook to verify delivery. Send project_id, hook_id, and the event type to fire (e.g. push_events). The project must contain matching content.",
		aliases:     []string{"test project webhook", "trigger webhook test", "send test hook event"},
		related:     []string{actionProjectHookGet, actionProjectHookList, actionProjectHookEdit},
		description: "Trigger a test event for a project webhook. Returns: a confirmation that the test event was fired. See also: gitlab_project_hook_get, gitlab_project_hook_edit.",
	},
	"gitlab_project_hook_set_custom_header": {
		usage:       "Set a custom HTTP header sent with a project webhook's requests. Send project_id, hook_id, key, and value. The value is write-only.",
		aliases:     []string{"set webhook custom header", "add webhook header", "configure webhook header", "create webhook custom header"},
		related:     []string{"project.hook_delete_custom_header", actionProjectHookGet, actionProjectHookEdit},
		description: "Set a custom header on a project webhook. Returns: a success confirmation naming the header and webhook. See also: gitlab_project_hook_delete_custom_header, gitlab_project_hook_get.",
	},
	"gitlab_project_hook_delete_custom_header": {
		usage:       "Delete a custom HTTP header from a project webhook. Send project_id, hook_id, and key.",
		aliases:     []string{"delete webhook custom header", "remove webhook header", "drop webhook custom header"},
		related:     []string{"project.hook_set_custom_header", actionProjectHookGet},
		description: "Delete a custom header from a project webhook. Returns: a success confirmation naming the header and webhook. See also: gitlab_project_hook_set_custom_header, gitlab_project_hook_get.",
	},
	"gitlab_project_hook_set_url_variable": {
		usage:       "Set a URL variable (placeholder substituted into the webhook URL) on a project webhook. Send project_id, hook_id, key, and value. The value is write-only.",
		aliases:     []string{"set webhook url variable", "add webhook url placeholder", "configure webhook url variable", "create webhook url variable"},
		related:     []string{"project.hook_delete_url_variable", actionProjectHookGet, actionProjectHookEdit},
		description: "Set a URL variable on a project webhook. Returns: a success confirmation naming the variable and webhook. See also: gitlab_project_hook_delete_url_variable, gitlab_project_hook_get.",
	},
	"gitlab_project_hook_delete_url_variable": {
		usage:       "Delete a URL variable from a project webhook. Send project_id, hook_id, and key.",
		aliases:     []string{"delete webhook url variable", "remove webhook url placeholder", "drop webhook url variable"},
		related:     []string{"project.hook_set_url_variable", actionProjectHookGet},
		description: "Delete a URL variable from a project webhook. Returns: a success confirmation naming the variable and webhook. See also: gitlab_project_hook_set_url_variable, gitlab_project_hook_get.",
	},
	"gitlab_project_approval_config_get": {
		usage:       "Get a project's merge-request approval configuration (approvals_before_merge, reset_approvals_on_push, author/committer approval rules). Send project_id. Premium/Ultimate.",
		aliases:     []string{"get approval config", "project approval settings", "merge approval configuration"},
		related:     []string{"project.approval_config_change", actionProjectApprovalRuleList},
		description: "Get a project's approval configuration. Returns: the approval settings such as reset_approvals_on_push and author approval flags. See also: gitlab_project_approval_config_change, gitlab_project_approval_rule_list.",
	},
	"gitlab_project_approval_config_change": {
		usage:       "Change a project's merge-request approval configuration (reset_approvals_on_push, author/committer approval, reauthentication). Send project_id plus the settings to change. Premium/Ultimate, Maintainer role.",
		aliases:     []string{"change approval config", "update project approval settings", "configure merge approvals"},
		related:     []string{actionProjectApprovalCfgGet, "project.approval_rule_create"},
		description: "Change a project's approval configuration. Returns: the updated approval settings. See also: gitlab_project_approval_config_get, gitlab_project_approval_rule_create.",
	},
	"gitlab_project_approval_rule_list": {
		usage:       "List a project's merge-request approval rules. Send project_id. Premium/Ultimate.",
		aliases:     []string{"list approval rules", "project approval rules", "show merge approval rules"},
		related:     []string{actionProjectApprovalRuleGet, "project.approval_rule_create", actionProjectApprovalCfgGet},
		description: "List a project's approval rules. Returns: rules with name, approvals_required, eligible approvers, and groups, plus pagination metadata. See also: gitlab_project_approval_rule_get, gitlab_project_approval_rule_create.",
	},
	"gitlab_project_approval_rule_get": {
		usage:       "Get one project approval rule by rule_id, including required approvals and eligible approvers. Send project_id and rule_id. Premium/Ultimate.",
		aliases:     []string{"get approval rule", "show approval rule", "fetch approval rule", "view approval rule"},
		related:     []string{actionProjectApprovalRuleList, "project.approval_rule_update", "project.approval_rule_delete"},
		description: "Get a single project approval rule. Returns: the rule with name, approvals_required, users, and groups. See also: gitlab_project_approval_rule_list, gitlab_project_approval_rule_update.",
	},
	"gitlab_project_approval_rule_create": {
		usage:       "Create a project approval rule. Send project_id, name, and approvals_required. Optionally user_ids, group_ids, usernames, protected_branch_ids, or rule_type. Premium/Ultimate, Maintainer role.",
		aliases:     []string{"create approval rule", "add merge approval rule", "new approval rule"},
		related:     []string{actionProjectApprovalRuleList, "project.approval_rule_update", "project.approval_config_change"},
		description: "Create a project approval rule. Returns: the created rule with name, approvals_required, users, and groups. See also: gitlab_project_approval_rule_list, gitlab_project_approval_rule_update.",
	},
	"gitlab_project_approval_rule_update": {
		usage:       "Update a project approval rule by rule_id (name, approvals_required, approvers, protected branches). Send project_id and rule_id plus the fields to change. Premium/Ultimate, Maintainer role.",
		aliases:     []string{"update approval rule", "edit merge approval rule", "change approval rule"},
		related:     []string{actionProjectApprovalRuleGet, actionProjectApprovalRuleList, "project.approval_rule_delete"},
		description: "Update a project approval rule. Returns: the updated rule with name, approvals_required, users, and groups. See also: gitlab_project_approval_rule_get, gitlab_project_approval_rule_delete.",
	},
	"gitlab_project_approval_rule_delete": {
		usage:       "Delete a project approval rule by rule_id. Send project_id and rule_id. Irreversible. Premium/Ultimate, Maintainer role.",
		aliases:     []string{"delete approval rule", "remove merge approval rule", "drop approval rule", "destroy approval rule"},
		related:     []string{actionProjectApprovalRuleList, actionProjectApprovalRuleGet, actionProjectApprovalCfgGet},
		description: "Delete a project approval rule. Returns: a success confirmation naming the rule and project. See also: gitlab_project_approval_rule_list, gitlab_project_approval_config_get.",
	},
	"gitlab_project_pull_mirror_get": {
		usage:       "Get a project's pull-mirror configuration (the upstream URL it mirrors from and its sync status). Send project_id. Premium/Ultimate.",
		aliases:     []string{"get pull mirror", "show mirror configuration", "project mirror status"},
		related:     []string{"project.pull_mirror_configure", "project.start_mirroring"},
		description: "Get a project's pull-mirror configuration. Returns: the mirror URL, enabled flag, and last sync status. See also: gitlab_project_pull_mirror_configure, gitlab_project_start_mirroring.",
	},
	"gitlab_project_pull_mirror_configure": {
		usage:       "Configure a project's pull mirror (upstream URL, trigger builds, protected-branches-only, overwrite-diverged). Send project_id plus the mirror settings. Premium/Ultimate.",
		aliases:     []string{"configure pull mirror", "set up project mirror", "edit mirror settings"},
		related:     []string{"project.pull_mirror_get", "project.start_mirroring"},
		description: "Configure a project's pull mirror. Returns: the updated mirror configuration. See also: gitlab_project_pull_mirror_get, gitlab_project_start_mirroring.",
	},
	"gitlab_project_start_mirroring": {
		usage:       "Trigger an immediate pull-mirror sync for a project so it fetches from its configured upstream now. Send project_id. Premium/Ultimate.",
		aliases:     []string{"start mirroring", "sync pull mirror now", "trigger pull mirror sync"},
		related:     []string{"project.pull_mirror_get", "project.pull_mirror_configure"},
		description: "Trigger a pull-mirror sync. Returns: a success confirmation naming the project. See also: gitlab_project_pull_mirror_get, gitlab_project_pull_mirror_configure.",
	},
	"gitlab_project_get_push_rules": {
		aliases:     []string{"get push rules", "show project push rules", "project commit rules"},
		related:     []string{actionProjectAddPushRule, actionProjectEditPushRule, actionProjectDeletePushRule},
		description: "Get a project's push rules. Returns: the singleton push-rule configuration such as commit_message_regex and reject_unsigned_commits. See also: gitlab_project_add_push_rule, gitlab_project_edit_push_rule.",
	},
	toolProjectAddPushRule: {
		aliases:     []string{"add push rule", "create project push rule", "set commit rules"},
		related:     []string{actionProjectGetPushRules, actionProjectEditPushRule, actionProjectDeletePushRule},
		description: "Add push rules to a project. Returns: the created push-rule configuration. See also: gitlab_project_get_push_rules, gitlab_project_edit_push_rule.",
	},
	toolProjectEditPushRule: {
		aliases:     []string{"edit push rule", "update project push rule", "change commit rules"},
		related:     []string{actionProjectGetPushRules, actionProjectAddPushRule, actionProjectDeletePushRule},
		description: "Edit a project's push rules. Returns: the updated push-rule configuration. See also: gitlab_project_get_push_rules, gitlab_project_add_push_rule.",
	},
	"gitlab_project_delete_push_rule": {
		aliases:     []string{"delete push rule", "remove project push rule", "clear commit rules"},
		related:     []string{actionProjectGetPushRules, actionProjectAddPushRule, actionProjectEditPushRule},
		description: "Delete a project's push rules. Returns: a success confirmation naming the project. See also: gitlab_project_get_push_rules, gitlab_project_add_push_rule.",
	},
	toolProjectDelete: {
		aliases:     []string{"delete project", "remove project", "destroy repository"},
		related:     []string{"project.restore", actionProjectGet, actionProjectArchive},
		description: "Delete a project. Returns: a deletion status. On instances with delayed deletion the project is marked for deletion. See also: gitlab_project_restore, gitlab_project_archive.",
	},
	"gitlab_project_list_target_branch_rules": {
		usage:       "List a project's target branch rules (the merge request target branch workflow that auto-selects a default target branch from the source branch pattern). Pass the full project path as project_id. The GraphQL query does not accept a numeric ID. Premium/Ultimate.",
		aliases:     []string{"list target branch rules", "show target branch rules", "merge request target branch rules"},
		related:     []string{"project.target_branch_rule_create", "project.target_branch_rule_delete"},
		description: "List a project's target branch rules. Returns: each rule with id, source-branch name pattern, target_branch, and created_at. See also: gitlab_project_create_target_branch_rule, gitlab_project_delete_target_branch_rule.",
	},
	"gitlab_project_create_target_branch_rule": {
		usage:       "Create a target branch rule mapping a source branch name pattern to a default target branch for new merge requests. Pass the numeric project_id, name (the source pattern, e.g. release/*), and target_branch. Premium/Ultimate.",
		aliases:     []string{"create target branch rule", "add target branch rule", "set default target branch for source pattern"},
		related:     []string{"project.target_branch_rule_list", "project.target_branch_rule_delete"},
		description: "Create a target branch rule. Returns: the created rule with id, source-branch name pattern, target_branch, and created_at. See also: gitlab_project_list_target_branch_rules, gitlab_project_delete_target_branch_rule.",
	},
	"gitlab_project_delete_target_branch_rule": {
		usage:       "Delete a target branch rule by its rule_id (find it via the list action). Premium/Ultimate.",
		aliases:     []string{"delete target branch rule", "remove target branch rule", "drop merge request target branch rule"},
		related:     []string{"project.target_branch_rule_list", "project.target_branch_rule_create"},
		description: "Delete a target branch rule. Returns: a success confirmation naming the rule. See also: gitlab_project_list_target_branch_rules, gitlab_project_create_target_branch_rule.",
	},
}

// projectListOrderByOverride is the shared order_by enum for project list
// endpoints that proxy the full ListProjectsOptions parameter set.
// "similarity" is only meaningful when a search query is also provided;
// "star_count" is valid on all project list calls.
var projectListOrderByOverride = toolutil.SchemaPropertyOverride("order_by", map[string]any{
	"enum":        []any{"id", "name", "path", "created_at", "updated_at", "last_activity_at", "similarity", "star_count"},
	"description": "Order projects by id, name, path, created_at, updated_at, last_activity_at, similarity (requires search query), or star_count.",
})

// projectSettingsEnumOverrides are the input-schema enum constraints shared by
// project create and update operations for fixed-value CI/CD and merge settings
// parameters whose valid set is enumerated in the GitLab API documentation.
var projectSettingsEnumOverrides = []toolutil.InputSchemaOverride{
	toolutil.SchemaPropertyOverride("merge_method", map[string]any{
		"enum":        []any{"merge", "rebase_merge", "ff"},
		"description": "Merge method: merge (default merge commit), rebase_merge (rebase then merge commit), or ff (fast-forward, no merge commit).",
	}),
	toolutil.SchemaPropertyOverride("squash_option", map[string]any{
		"enum":        []any{"never", "always", "default_on", "default_off"},
		"description": "Squash commits on merge: never (not allowed), always (always squash), default_on (squash by default, overridable per MR), or default_off (no squash by default, overridable per MR).",
	}),
	toolutil.SchemaPropertyOverride("build_git_strategy", map[string]any{
		"enum":        []any{"fetch", "clone"},
		"description": "Git strategy for CI builds: fetch (reuse existing clone, faster) or clone (fresh clone every time).",
	}),
	toolutil.SchemaPropertyOverride("auto_devops_deploy_strategy", map[string]any{
		"enum":        []any{"continuous", "manual", "timed_incremental"},
		"description": "Auto DevOps deploy strategy: continuous (auto-deploy to production), manual (require manual gate), or timed_incremental (deploy incrementally with time delays).",
	}),
	toolutil.SchemaPropertyOverride("auto_cancel_pending_pipelines", map[string]any{
		"enum":        []any{"enabled", "disabled"},
		"description": "Auto-cancel redundant pending pipelines when a newer pipeline starts: enabled or disabled.",
	}),
	// doc/api/resource_groups.md lists the four process modes; client-go's
	// ResourceGroupProcessMode declares the same four.
	toolutil.SchemaPropertyOverride("resource_group_default_process_mode", map[string]any{
		"enum":        []any{"unordered", "oldest_first", "newest_first", "newest_ready_first"},
		"description": "Default process mode for resource groups: unordered (run in parallel), oldest_first, newest_first, or newest_ready_first.",
	}),
	// dap_powered was removed as an input in GitLab 19.4; it can still be read
	// back from projects configured before then, so the output keeps it.
	toolutil.SchemaEnumOverride("reviewer_assignment_strategy", "disabled", "code_owners"),
}

// projectUpdateEnumOverrides extends the shared settings enums with the two
// CI/CD role parameters that only the update input carries.
var projectUpdateEnumOverrides = slices.Concat(projectSettingsEnumOverrides, []toolutil.InputSchemaOverride{
	toolutil.SchemaEnumOverride("ci_restrict_pipeline_cancellation_role", "developer", "maintainer", "no_one"),
	toolutil.SchemaEnumOverride("ci_pipeline_variables_minimum_override_role", "owner", "maintainer", "developer", "no_one_allowed"),
})

// projectHookTestEventOverride pins the webhook test trigger to the event names
// GitLab accepts on POST /projects/:id/hooks/:hook_id/test/:trigger.
var projectHookTestEventOverride = toolutil.SchemaEnumOverride("event",
	"push_events", "tag_push_events", "issues_events", "confidential_issues_events", "note_events",
	"merge_requests_events", "job_events", "pipeline_events", "wiki_page_events", "releases_events",
	"milestone_events", "emoji_events", "resource_access_token_events", "resource_deploy_token_events")

// projectApprovalRuleTypeOverride pins rule_type to the values the approval
// rules API documents for POST /projects/:id/approval_rules. report_approver is
// among them even though GitLab reserves it for policy-created rules.
var projectApprovalRuleTypeOverride = toolutil.SchemaEnumOverride("rule_type", "any_approver", "regular", "report_approver")

// projectActionSchemaOverrides maps individual project tool names to their
// additional input-schema enum overrides. decorateProjectSchemaOverrides
// appends these onto any overrides already set by projectOptionsForAction so
// that inline and map-based overrides coexist without conflict.
//
//nolint:gochecknoglobals // Intentional package-level lookup table; mirrors projectActionMeta pattern.
var projectActionSchemaOverrides = map[string][]toolutil.InputSchemaOverride{
	"gitlab_project_create":                projectSettingsEnumOverrides,
	"gitlab_project_update":                projectUpdateEnumOverrides,
	"gitlab_project_create_for_user":       projectSettingsEnumOverrides,
	"gitlab_project_hook_test":             {projectHookTestEventOverride},
	"gitlab_project_approval_rule_create":  {projectApprovalRuleTypeOverride},
	"gitlab_project_list_forks":            {projectListOrderByOverride},
	"gitlab_project_list_user_projects":    {projectListOrderByOverride},
	"gitlab_project_list_user_contributed": {projectListOrderByOverride},
	"gitlab_project_list_user_starred":     {projectListOrderByOverride},
}

// decorateProjectSchemaOverrides appends input-schema enum overrides for
// project actions that expose fixed enumerated values on specific parameters.
// It appends to any overrides already set in projectOptionsForAction (e.g. the
// inline order_by/sort overrides on the list action) so both coexist without
// conflict.
func decorateProjectSchemaOverrides(options *toolutil.ActionSpecOptions, individualTool string) {
	overrides, ok := projectActionSchemaOverrides[individualTool]
	if !ok {
		return
	}
	options.InputSchemaOverrides = append(options.InputSchemaOverrides, overrides...)
}

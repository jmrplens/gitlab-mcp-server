package projects

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project lifecycle and settings actions.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		projectCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_project_create"),
		projectGetSpec(projectGetRoute(client)),
		projectReadSpec("list", toolutil.RouteAction(client, List), "gitlab_project_list"),
		projectDestructiveUpdateSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_project_delete"),
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
		projectReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_project_hook_list", "webhook"),
		projectReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_project_hook_get", "webhook"),
		projectCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_project_hook_add", "webhook"),
		projectMutationSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_project_hook_edit", "webhook"),
		projectDestructiveUpdateSpec("hook_delete", toolutil.RouteAction(client, DeleteHookOutput), "gitlab_project_hook_delete", "webhook"),
		projectMutationSpec("hook_test", toolutil.RouteAction(client, TriggerTestHook), "gitlab_project_hook_test", "webhook"),
		projectReadSpec("list_user_projects", toolutil.RouteAction(client, ListUserProjects), "gitlab_project_list_user_projects", "user"),
		projectReadSpec("list_users", toolutil.RouteAction(client, ListProjectUsers), "gitlab_project_list_users", "user"),
		projectReadSpec("list_groups", toolutil.RouteAction(client, ListProjectGroups), "gitlab_project_list_groups", "group"),
		projectReadSpec("list_starrers", toolutil.RouteAction(client, ListProjectStarrers), "gitlab_project_list_starrers", "user"),
		projectCreateSpec("share_with_group", toolutil.RouteAction(client, ShareProjectWithGroup), "gitlab_project_share_with_group", "group"),
		projectDestructiveUpdateSpec("delete_shared_group", toolutil.RouteAction(client, DeleteSharedGroupOutput), "gitlab_project_delete_shared_group", "group"),
		projectReadSpec("list_invited_groups", toolutil.RouteAction(client, ListInvitedGroups), "gitlab_project_list_invited_groups", "group"),
		projectReadSpec("list_user_contributed", toolutil.RouteAction(client, ListUserContributedProjects), "gitlab_project_list_user_contributed", "user"),
		projectReadSpec("list_user_starred", toolutil.RouteAction(client, ListUserStarredProjects), "gitlab_project_list_user_starred", "user"),
		projectMutationSpec("hook_set_custom_header", toolutil.RouteAction(client, SetCustomHeaderOutput), "gitlab_project_hook_set_custom_header", "webhook"),
		projectDestructiveUpdateSpec("hook_delete_custom_header", toolutil.RouteAction(client, DeleteCustomHeaderOutput), "gitlab_project_hook_delete_custom_header", "webhook"),
		projectMutationSpec("hook_set_url_variable", toolutil.RouteAction(client, SetWebhookURLVariableOutput), "gitlab_project_hook_set_url_variable", "webhook"),
		projectDestructiveUpdateSpec("hook_delete_url_variable", toolutil.RouteAction(client, DeleteWebhookURLVariableOutput), "gitlab_project_hook_delete_url_variable", "webhook"),
		projectCreateSpec("create_fork_relation", toolutil.RouteAction(client, CreateForkRelation), "gitlab_project_create_fork_relation"),
		projectDestructiveUpdateSpec("delete_fork_relation", toolutil.RouteAction(client, DeleteForkRelationOutput), "gitlab_project_delete_fork_relation"),
		projectMutationSpec("upload_avatar", toolutil.RouteAction(client, UploadAvatar), "gitlab_project_upload_avatar", "avatar"),
		projectReadSpec("download_avatar", toolutil.RouteAction(client, DownloadAvatar), "gitlab_project_download_avatar", "avatar"),
		projectReadSpec("approval_config_get", toolutil.RouteAction(client, GetApprovalConfig), "gitlab_project_approval_config_get", "approval"),
		projectMutationSpec("approval_config_change", toolutil.RouteAction(client, ChangeApprovalConfig), "gitlab_project_approval_config_change", "approval"),
		projectReadSpec("approval_rule_list", toolutil.RouteAction(client, ListApprovalRules), "gitlab_project_approval_rule_list", "approval"),
		projectReadSpec("approval_rule_get", toolutil.RouteAction(client, GetApprovalRule), "gitlab_project_approval_rule_get", "approval"),
		projectCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateApprovalRule), "gitlab_project_approval_rule_create", "approval"),
		projectMutationSpec("approval_rule_update", toolutil.RouteAction(client, UpdateApprovalRule), "gitlab_project_approval_rule_update", "approval"),
		projectDestructiveUpdateSpec("approval_rule_delete", toolutil.RouteAction(client, DeleteApprovalRuleOutput), "gitlab_project_approval_rule_delete", "approval"),
		projectReadSpec("pull_mirror_get", toolutil.RouteAction(client, GetPullMirror), "gitlab_project_pull_mirror_get", "mirror"),
		projectMutationSpec("pull_mirror_configure", toolutil.RouteAction(client, ConfigurePullMirror), "gitlab_project_pull_mirror_configure", "mirror"),
		projectMutationSpec("start_mirroring", toolutil.RouteAction(client, StartMirroringOutput), "gitlab_project_start_mirroring", "mirror"),
		projectMutationSpec("start_housekeeping", toolutil.RouteAction(client, StartHousekeepingOutput), "gitlab_project_start_housekeeping"),
		projectReadSpec("repository_storage_get", toolutil.RouteAction(client, GetRepositoryStorage), "gitlab_project_repository_storage_get"),
		projectCreateSpec("create_for_user", toolutil.RouteAction(client, CreateForUser), "gitlab_project_create_for_user", "admin"),
	}
	if enterprise {
		specs = append(
			specs,
			projectPremiumSpec(projectReadSpec("push_rule_get", toolutil.RouteAction(client, GetPushRules), "gitlab_project_get_push_rules", "push_rule")),
			projectPremiumSpec(projectCreateSpec("push_rule_add", toolutil.RouteAction(client, AddPushRule), "gitlab_project_add_push_rule", "push_rule")),
			projectPremiumSpec(projectMutationSpec("push_rule_edit", toolutil.RouteAction(client, EditPushRule), "gitlab_project_edit_push_rule", "push_rule")),
			projectPremiumSpec(projectDestructiveUpdateSpec("push_rule_delete", toolutil.RouteAction(client, DeletePushRuleOutput), "gitlab_project_delete_push_rule", "push_rule")),
		)
	}
	return specs
}

func projectGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return projectNotFoundOutput{Identifier: fmt.Sprint(input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

// DeleteHookOutput deletes a project webhook and returns the legacy success message shape.
func DeleteHookOutput(ctx context.Context, client *gitlabclient.Client, input DeleteHookInput) (toolutil.DeleteOutput, error) {
	if err := DeleteHook(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted webhook %d from project %s.", input.HookID, input.ProjectID)}, nil
}

// DeleteSharedGroupOutput removes a shared group and returns the legacy success message shape.
func DeleteSharedGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteSharedGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteSharedProjectFromGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted shared group %d from project %s.", input.GroupID, input.ProjectID)}, nil
}

// SetCustomHeaderOutput sets a webhook custom header and returns the legacy success message shape.
func SetCustomHeaderOutput(ctx context.Context, client *gitlabclient.Client, input SetCustomHeaderInput) (toolutil.VoidOutput, error) {
	if err := SetCustomHeader(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Custom header %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteCustomHeaderOutput deletes a webhook custom header and returns the legacy success message shape.
func DeleteCustomHeaderOutput(ctx context.Context, client *gitlabclient.Client, input DeleteCustomHeaderInput) (toolutil.VoidOutput, error) {
	if err := DeleteCustomHeader(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Custom header %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// SetWebhookURLVariableOutput sets a webhook URL variable and returns the legacy success message shape.
func SetWebhookURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input SetWebhookURLVariableInput) (toolutil.VoidOutput, error) {
	if err := SetWebhookURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("URL variable %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteWebhookURLVariableOutput deletes a webhook URL variable and returns the legacy success message shape.
func DeleteWebhookURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input DeleteWebhookURLVariableInput) (toolutil.VoidOutput, error) {
	if err := DeleteWebhookURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("URL variable %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID)}, nil
}

// DeleteForkRelationOutput removes a fork relation and returns the legacy success message shape.
func DeleteForkRelationOutput(ctx context.Context, client *gitlabclient.Client, input DeleteForkRelationInput) (toolutil.VoidOutput, error) {
	if err := DeleteForkRelation(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Fork relation removed from project %s", input.ProjectID)}, nil
}

// DeleteApprovalRuleOutput deletes a project approval rule and returns the legacy success message shape.
func DeleteApprovalRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteApprovalRuleInput) (toolutil.VoidOutput, error) {
	if err := DeleteApprovalRule(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Approval rule %d deleted from project %s", input.RuleID, input.ProjectID)}, nil
}

// StartMirroringOutput starts pull mirroring and returns the legacy success message shape.
func StartMirroringOutput(ctx context.Context, client *gitlabclient.Client, input StartMirroringInput) (toolutil.VoidOutput, error) {
	if err := StartMirroring(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Mirror update triggered for project %s", input.ProjectID)}, nil
}

// StartHousekeepingOutput starts housekeeping and returns the legacy success message shape.
func StartHousekeepingOutput(ctx context.Context, client *gitlabclient.Client, input StartHousekeepingInput) (toolutil.VoidOutput, error) {
	if err := StartHousekeeping(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Housekeeping started for project %s", input.ProjectID)}, nil
}

// DeletePushRuleOutput deletes project push rules and returns the legacy success message shape.
func DeletePushRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeletePushRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeletePushRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted push rules for project %s.", input.ProjectID)}, nil
}

func projectPremiumSpec(spec toolutil.ActionSpec) toolutil.ActionSpec {
	spec.Edition = "premium"
	return spec
}

func projectGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := projectOptions("gitlab_project_get")
	options.Usage = "Get one exact project by numeric ID or full namespace path. Use this when the prompt gives a concrete path like group/project and asks to find, show, verify, or read project metadata such as id or default_branch; do not use search.projects for an exact path lookup."
	options.RelatedActions = []string{"project.archive", "project.delete", "project.update"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_project",
			ValueSource:      "Concrete numeric project ID or full namespace path from the prompt or a prior project result.",
			ExampleBinding:   `params.project_id:"my-org/tools/gitlab-mcp-server"`,
			CommonConfusions: []string{"Use project_id for a namespace path; do not substitute full_path, path, remote_url, search, or query."},
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
	if slices.Contains(extraTags, "push_rule") {
		options.Usage = "Use project push rule actions for the singleton project-level push rule. Do not pass push_rule_id; get/add/edit/delete operate by project_id only. Use reject_unsigned_commits, not deny_unsigned_commits."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project that owns the singleton push rule.",
				CommonConfusions: []string{"Push rules are project-scoped singletons; there is no push_rule_id parameter."},
			},
		}
		if individualTool == "gitlab_project_add_push_rule" || individualTool == "gitlab_project_edit_push_rule" {
			options.ParameterGuidance["commit_message_regex"] = toolutil.ParameterGuidance{
				ValueSource:      "Commit message regex requested by the user.",
				ExampleBinding:   `params.commit_message_regex:"^EVAL-"`,
				CommonConfusions: []string{"Use commit_message_regex directly; do not send commit_message_regex_enabled or empty regex placeholders."},
			}
			options.ParameterGuidance["reject_unsigned_commits"] = toolutil.ParameterGuidance{
				ValueSource:      "Boolean that enables unsigned commit rejection.",
				ExampleBinding:   "params.reject_unsigned_commits:true",
				CommonConfusions: []string{"Use reject_unsigned_commits, not deny_unsigned_commits."},
			}
		}
		if individualTool == "gitlab_project_add_push_rule" {
			options.Usage += " For add, include at least one rule-setting parameter such as commit_message_regex, reject_unsigned_commits, prevent_secrets, branch_name_regex, or deny_delete_tag; do not call add with project_id alone."
		}
	}
	if individualTool == "gitlab_project_delete" {
		options.Usage = "Use to delete a project. For ordinary cleanup, send project_id and confirm only. Set permanently_remove only when explicitly requested; when permanently_remove is true, full_path must exactly match the project's path_with_namespace from project create/get."
	}
	if actionName == "list" && individualTool == "gitlab_project_list" {
		options.Usage = "List projects accessible to the authenticated user. For most recently updated projects, use order_by last_activity_at with sort desc and per_page for the requested count; do not use last_activity_after as an order_by value."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"order_by": {
				SemanticRole: "project_list_sort_field",
				ValueSource:  "Use only GitLab project list ordering fields accepted by the projects API.",
				CommonConfusions: []string{
					"Use last_activity_at to sort by recent activity; last_activity_after is a date filter, not an order_by value.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("order_by", map[string]any{
				"enum":        []any{"id", "name", "path", "created_at", "updated_at", "last_activity_at"},
				"description": "Order projects by id, name, path, created_at, updated_at, or last_activity_at.",
			}),
			toolutil.SchemaPropertyOverride("sort", map[string]any{"enum": []any{"asc", "desc"}}),
		}
	}
	return options
}

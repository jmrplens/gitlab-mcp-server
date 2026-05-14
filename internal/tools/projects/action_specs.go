package projects

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project lifecycle and settings actions.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		projectCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_project_create"),
		projectGetSpec(toolutil.RouteAction(client, Get)),
		projectReadSpec("list", toolutil.RouteAction(client, List), "gitlab_project_list"),
		projectDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_project_delete"),
		projectUpdateSpec("restore", toolutil.RouteAction(client, Restore), "gitlab_project_restore"),
		projectUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_project_update"),
		projectCreateSpec("fork", toolutil.RouteAction(client, Fork), "gitlab_project_fork"),
		projectCreateSpec("star", toolutil.RouteAction(client, Star), "gitlab_project_star"),
		projectUpdateSpec("unstar", toolutil.RouteAction(client, Unstar), "gitlab_project_unstar"),
		projectUpdateSpec("archive", toolutil.RouteAction(client, Archive), "gitlab_project_archive"),
		projectUpdateSpec("unarchive", toolutil.RouteAction(client, Unarchive), "gitlab_project_unarchive"),
		projectUpdateSpec("transfer", toolutil.RouteAction(client, Transfer), "gitlab_project_transfer"),
		projectReadSpec("list_forks", toolutil.RouteAction(client, ListForks), "gitlab_project_list_forks"),
		projectReadSpec("languages", toolutil.RouteAction(client, GetLanguages), "gitlab_project_languages"),
		projectReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_project_hook_list", "webhook"),
		projectReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_project_hook_get", "webhook"),
		projectCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_project_hook_add", "webhook"),
		projectUpdateSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_project_hook_edit", "webhook"),
		projectDeleteSpec("hook_delete", toolutil.DestructiveVoidAction(client, DeleteHook), "gitlab_project_hook_delete", "webhook"),
		projectMutationSpec("hook_test", toolutil.RouteAction(client, TriggerTestHook), "gitlab_project_hook_test", "webhook"),
		projectReadSpec("list_user_projects", toolutil.RouteAction(client, ListUserProjects), "gitlab_project_list_user_projects", "user"),
		projectReadSpec("list_users", toolutil.RouteAction(client, ListProjectUsers), "gitlab_project_list_users", "user"),
		projectReadSpec("list_groups", toolutil.RouteAction(client, ListProjectGroups), "gitlab_project_list_groups", "group"),
		projectReadSpec("list_starrers", toolutil.RouteAction(client, ListProjectStarrers), "gitlab_project_list_starrers", "user"),
		projectCreateSpec("share_with_group", toolutil.RouteAction(client, ShareProjectWithGroup), "gitlab_project_share_with_group", "group"),
		projectDeleteSpec("delete_shared_group", toolutil.DestructiveVoidAction(client, DeleteSharedProjectFromGroup), "gitlab_project_delete_shared_group", "group"),
		projectReadSpec("list_invited_groups", toolutil.RouteAction(client, ListInvitedGroups), "gitlab_project_list_invited_groups", "group"),
		projectReadSpec("list_user_contributed", toolutil.RouteAction(client, ListUserContributedProjects), "gitlab_project_list_user_contributed", "user"),
		projectReadSpec("list_user_starred", toolutil.RouteAction(client, ListUserStarredProjects), "gitlab_project_list_user_starred", "user"),
		projectUpdateSpec("hook_set_custom_header", toolutil.RouteVoidAction(client, SetCustomHeader), "gitlab_project_hook_set_custom_header", "webhook"),
		projectDeleteSpec("hook_delete_custom_header", toolutil.DestructiveVoidAction(client, DeleteCustomHeader), "gitlab_project_hook_delete_custom_header", "webhook"),
		projectUpdateSpec("hook_set_url_variable", toolutil.RouteVoidAction(client, SetWebhookURLVariable), "gitlab_project_hook_set_url_variable", "webhook"),
		projectDeleteSpec("hook_delete_url_variable", toolutil.DestructiveVoidAction(client, DeleteWebhookURLVariable), "gitlab_project_hook_delete_url_variable", "webhook"),
		projectCreateSpec("create_fork_relation", toolutil.RouteAction(client, CreateForkRelation), "gitlab_project_create_fork_relation"),
		projectDeleteSpec("delete_fork_relation", toolutil.DestructiveVoidAction(client, DeleteForkRelation), "gitlab_project_delete_fork_relation"),
		projectUpdateSpec("upload_avatar", toolutil.RouteAction(client, UploadAvatar), "gitlab_project_upload_avatar", "avatar"),
		projectReadSpec("download_avatar", toolutil.RouteAction(client, DownloadAvatar), "gitlab_project_download_avatar", "avatar"),
		projectReadSpec("approval_config_get", toolutil.RouteAction(client, GetApprovalConfig), "gitlab_project_approval_config_get", "approval"),
		projectUpdateSpec("approval_config_change", toolutil.RouteAction(client, ChangeApprovalConfig), "gitlab_project_approval_config_change", "approval"),
		projectReadSpec("approval_rule_list", toolutil.RouteAction(client, ListApprovalRules), "gitlab_project_approval_rule_list", "approval"),
		projectReadSpec("approval_rule_get", toolutil.RouteAction(client, GetApprovalRule), "gitlab_project_approval_rule_get", "approval"),
		projectCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateApprovalRule), "gitlab_project_approval_rule_create", "approval"),
		projectUpdateSpec("approval_rule_update", toolutil.RouteAction(client, UpdateApprovalRule), "gitlab_project_approval_rule_update", "approval"),
		projectDeleteSpec("approval_rule_delete", toolutil.DestructiveVoidAction(client, DeleteApprovalRule), "gitlab_project_approval_rule_delete", "approval"),
		projectReadSpec("pull_mirror_get", toolutil.RouteAction(client, GetPullMirror), "gitlab_project_pull_mirror_get", "mirror"),
		projectUpdateSpec("pull_mirror_configure", toolutil.RouteAction(client, ConfigurePullMirror), "gitlab_project_pull_mirror_configure", "mirror"),
		projectMutationSpec("start_mirroring", toolutil.RouteVoidAction(client, StartMirroring), "gitlab_project_start_mirroring", "mirror"),
		projectMutationSpec("start_housekeeping", toolutil.RouteVoidAction(client, StartHousekeeping), "gitlab_project_start_housekeeping"),
		projectReadSpec("repository_storage_get", toolutil.RouteAction(client, GetRepositoryStorage), "gitlab_project_repository_storage_get"),
		projectCreateSpec("create_for_user", toolutil.RouteAction(client, CreateForUser), "gitlab_project_create_for_user", "admin"),
	}
	if enterprise {
		specs = append(specs,
			projectPremiumSpec(projectReadSpec("push_rule_get", toolutil.RouteAction(client, GetPushRules), "gitlab_project_get_push_rules", "push_rule")),
			projectPremiumSpec(projectCreateSpec("push_rule_add", toolutil.RouteAction(client, AddPushRule), "gitlab_project_add_push_rule", "push_rule")),
			projectPremiumSpec(projectUpdateSpec("push_rule_edit", toolutil.RouteAction(client, EditPushRule), "gitlab_project_edit_push_rule", "push_rule")),
			projectPremiumSpec(projectDeleteSpec("push_rule_delete", toolutil.DestructiveVoidAction(client, DeletePushRule), "gitlab_project_delete_push_rule", "push_rule")),
		)
	}
	return specs
}

func projectPremiumSpec(spec toolutil.ActionSpec) toolutil.ActionSpec {
	spec.Edition = "premium"
	return spec
}

func projectGetSpec(route toolutil.ActionRoute) toolutil.ActionSpec {
	options := projectOptions("gitlab_project_get")
	options.ReadOnly = true
	options.Idempotent = true
	options.RelatedActions = []string{"project.archive", "project.delete", "project.update"}
	return toolutil.NewActionSpec("get", route, options)
}

func projectReadSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := projectOptions(individualTool, extraTags...)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectCreateSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, projectOptions(individualTool, extraTags...))
}

func projectMutationSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := projectOptions(individualTool, extraTags...)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectUpdateSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := projectOptions(individualTool, extraTags...)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectDeleteSpec(name string, route toolutil.ActionRoute, individualTool string, extraTags ...string) toolutil.ActionSpec {
	options := projectOptions(individualTool, extraTags...)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func projectOptions(individualTool string, extraTags ...string) toolutil.ActionSpecOptions {
	tags := append([]string{"project"}, extraTags...)
	return toolutil.ActionSpecOptions{
		Tags:           tags,
		OpenWorld:      true,
		OwnerPackage:   "projects",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

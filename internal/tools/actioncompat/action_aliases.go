package actioncompat

import (
	"sort"
	"strings"
	"sync"
)

const (
	SourceCompatibility = "compatibility"
	SourceStandalone    = "standalone"
)

const defaultActionAliasReason = "Historical Dynamic compatibility alias for canonical action selection."

const (
	actionGroupServiceAccountPATRevoke   = "group.service_account_pat_revoke"
	actionProjectServiceAccountPATRevoke = "project.service_account_pat_revoke"
	actionFeatureFlagCreate              = "feature_flags.feature_flag_create"
	actionBranchProtect                  = "branch.protect"
	actionExternalStatusCheckListProject = "external_status_check.list_project"
	actionFeatureFlagUserListList        = "feature_flags.ff_user_list_list"
	actionGroupEpicBoardList             = "group.epic_board_list"
	actionGroupEpicDiscussionUpdateNote  = "group.epic_discussion_update_note"
	actionGroupEpicDiscussionDeleteNote  = "group.epic_discussion_delete_note"
	actionGroupLabelUpdate               = "group.group_label_update"
	actionGroupProtectedBranchProtect    = "group.protected_branch_protect"
	actionGroupProtectedEnvProtect       = "group.protected_env_protect"
	actionGroupProtectedEnvUpdate        = "group.protected_env_update"
	actionInteractiveIssueCreate         = "interactive.issue_create"
	actionIssueLinkCreate                = "issue.link_create"
	actionIssueNoteList                  = "issue.note_list"
	actionIssueSpentTimeAdd              = "issue.spent_time_add"
	actionIssueTimeEstimateSet           = "issue.time_estimate_set"
	actionIssueUpdate                    = "issue.update"
	actionJobList                        = "job.list"
	actionMergeRequestEmojiMRCreate      = "merge_request.emoji_mr_create"
	actionMergeRequestSpentTimeAdd       = "merge_request.spent_time_add"
	actionMRReviewDraftNotePublishAll    = "mr_review.draft_note_publish_all"
	actionPackageList                    = "package.list"
	actionPackagePublishDirectory        = "package.publish_directory"
	actionPipelineScheduleCreate         = "pipeline.schedule_create"
	actionPipelineScheduleUpdate         = "pipeline.schedule_update"
	actionProjectProtectedEnvProtect     = "environment.protected_protect"
	actionProjectProtectedEnvUpdate      = "environment.protected_update"
	actionProjectHookAdd                 = "project.hook_add"
	actionProjectMemberAdd               = "project.member_add"
	actionProjectMemberDelete            = "project.member_delete"
	actionProjectMemberEdit              = "project.member_edit"
	actionProjectPushRuleAdd             = "project.push_rule_add"
	actionProjectPushRuleEdit            = "project.push_rule_edit"
	actionReleaseCreate                  = "release.create"
	actionReleaseLinkCreate              = "release.link_create"
	actionReleaseLinkCreateBatch         = "release.link_create_batch"
	actionReleaseLinkDelete              = "release.link_delete"
	actionReleaseLinkGet                 = "release.link_get"
	actionReleaseLinkList                = "release.link_list"
	actionReleaseLinkUpdate              = "release.link_update"
	actionRepositoryFileGet              = "repository.file_get"
	actionRunnerUpdate                   = "runner.update"
	actionSnippetProjectCreate           = "snippet.project_create"
	actionAdminTerraformStateDelete      = "admin.terraform_state_delete"
	actionAdminTerraformStateGet         = "admin.terraform_state_get"
	actionAdminTerraformStateList        = "admin.terraform_state_list"
	actionAdminTerraformStateLock        = "admin.terraform_state_lock"
	actionAdminTerraformStateUnlock      = "admin.terraform_state_unlock"
	actionAdminTerraformVersionDelete    = "admin.terraform_version_delete"
)

// ActionAlias describes one historical action ID alias and its canonical action.
type ActionAlias struct {
	Alias          string
	Canonical      string
	Source         string
	Searchable     bool
	Deprecated     bool
	RemovalVersion string
	Reason         string
}

// ActionAliases returns historical action ID aliases that are projected into
// ActionSpec compatibility metadata before the catalog is built.
func ActionAliases() []ActionAlias {
	return cloneActionAliases(defaultActionAliases())
}

func defaultActionAliases() []ActionAlias {
	return []ActionAlias{
		compatActionAlias("badge.create", "project.badge_add"),
		compatActionAlias("badge.delete", "project.badge_delete"),
		compatActionAlias("broadcast_message.create", "admin.broadcast_message_create"),
		compatActionAlias("broadcast_message.delete", "admin.broadcast_message_delete"),
		compatActionAlias("ci_catalog.resource_list", "ci_catalog.list"),
		compatActionAlias("ci_job_token_scope.inbound_allowlist.list", "job.token_scope_list_inbound"),
		compatActionAlias("deploy_key.create", "access.deploy_key_add"),
		compatActionAlias("deploy_key.delete", "access.deploy_key_delete"),
		compatActionAlias("deploy_key.get", "access.deploy_key_get"),
		compatActionAlias("deploy_key.list", "access.deploy_key_list_project"),
		compatActionAlias("deploy_key.update", "access.deploy_key_update"),
		compatActionAlias("deploy_token.create", "access.deploy_token_create_project"),
		compatActionAlias("deploy_token.delete", "access.deploy_token_delete_project"),
		compatActionAlias("deploy_token.get", "access.deploy_token_get_project"),
		compatActionAlias("deploy_token.list", "access.deploy_token_list_project"),
		compatActionAlias("branch.protected_list", "branch.get_protected"),
		compatActionAlias("branch.update_protection", "branch.update_protected"),
		compatActionAlias("enterprise_user.group_list", "enterprise_user.list"),
		compatActionAlias("external_status_check.list_project_checks", actionExternalStatusCheckListProject),
		compatActionAlias("feature_flag.list", "feature_flags.feature_flag_list"),
		compatActionAlias("geo.node_list", "geo.list"),
		compatActionAlias("gitlab_server.health_check", "server.health_check"),
		compatActionAlias("feature_flag_user_list.create", "feature_flags.ff_user_list_create"),
		compatActionAlias("feature_flag_user_list.delete", "feature_flags.ff_user_list_delete"),
		compatActionAlias("feature_flag_user_list.get", "feature_flags.ff_user_list_get"),
		compatActionAlias("feature_flag_user_list.list", actionFeatureFlagUserListList),
		compatActionAlias("feature_flag_user_list.update", "feature_flags.ff_user_list_update"),
		compatActionAlias("feature_flags.feature_flag_user_list", actionFeatureFlagUserListList),
		compatActionAlias("feature_flags.feature_flag_user_list_list", actionFeatureFlagUserListList),
		compatActionAlias("feature_flags.feature_flag_user_lists_list", actionFeatureFlagUserListList),
		compatActionAlias("gitlab_issue.create", "issue.create"),
		compatActionAlias("gitlab_issue.delete", "issue.delete"),
		compatActionAlias("group.custom_member_roles_list", "member_role.list_group"),
		compatActionAlias("group.group_board_list", actionGroupEpicBoardList),
		compatActionAlias("group.epic_discussion_note_update", actionGroupEpicDiscussionUpdateNote),
		compatActionAlias("group.epic_discussion_note_delete", actionGroupEpicDiscussionDeleteNote),
		compatActionAlias("group.ldap_link_delete", "group.ldap_link_delete_for_provider"),
		compatActionAlias("issue.note.create", "issue.note_create"),
		compatActionAlias("issue.note.delete", "issue.note_delete"),
		compatActionAlias("issue.note.get", "issue.note_get"),
		compatActionAlias("issue.note.list", actionIssueNoteList),
		compatActionAlias("issue.note.update", "issue.note_update"),
		compatActionAlias("issue.close", actionIssueUpdate),
		compatActionAlias("issue_note.get", "issue.note_get"),
		compatActionAlias("issue_note.list", actionIssueNoteList),
		compatActionAlias("issue_note.delete", "issue.note_delete"),
		compatActionAlias("issue_note.update", "issue.note_update"),
		compatActionAlias("issue.notes", actionIssueNoteList),
		compatActionAlias("issue.notes.list", actionIssueNoteList),
		compatActionAlias("issue.reopen", actionIssueUpdate),
		compatActionAlias("job.artifact_download", "job.download_single_artifact"),
		compatActionAlias("pipeline.jobs", "job.list"),
		compatActionAlias("merge_train.list", "merge_train.list_project"),
		compatActionAlias("merge_request.accept", "merge_request.merge"),
		compatActionAlias("merge_request.changes", "mr_review.changes_get"),
		compatActionAlias("merge_request.emoji_award_create", actionMergeRequestEmojiMRCreate),
		compatActionAlias("merge_request.emoji_award_delete", "merge_request.emoji_mr_delete"),
		compatActionAlias("merge_request.emoji_mr_award_create", actionMergeRequestEmojiMRCreate),
		compatActionAlias("merge_request.emoji_mr_award_delete", "merge_request.emoji_mr_delete"),
		compatActionAlias("merge_request.award_emoji_add", actionMergeRequestEmojiMRCreate),
		compatActionAlias("merge_request.award_emoji_create", actionMergeRequestEmojiMRCreate),
		compatActionAlias("merge_request_note.create", "mr_review.note_create"),
		compatActionAlias("merge_request_note.delete", "mr_review.note_delete"),
		compatActionAlias("merge_request_note.get", "mr_review.note_get"),
		compatActionAlias("merge_request_note.update", "mr_review.note_update"),
		compatActionAlias("merge_request.add_spent_time", actionMergeRequestSpentTimeAdd),
		compatActionAlias("merge_request.set_time_estimate", "merge_request.time_estimate_set"),
		compatActionAlias("merge_request.time_estimate", "merge_request.time_estimate_set"),
		compatActionAlias("merge_request.time_spent_add", actionMergeRequestSpentTimeAdd),
		compatActionAlias("merge_request.time_spent_set", actionMergeRequestSpentTimeAdd),
		compatActionAlias("mr_review.draft_notes_publish", actionMRReviewDraftNotePublishAll),
		compatActionAlias("mr_review.publish", actionMRReviewDraftNotePublishAll),
		compatActionAlias("package.files", "package.file_list"),
		compatActionAlias("package.list_generic", actionPackageList),
		compatActionAlias("personal_snippet.raw", "snippet.content"),
		compatActionAlias("project.releases.list", "release.list"),
		compatActionAlias("project.hook_create", actionProjectHookAdd),
		compatActionAlias("project.hooks.list", "project.hook_list"),
		compatActionAlias("project.member_remove", actionProjectMemberDelete),
		compatActionAlias("project.member_update", actionProjectMemberEdit),
		compatActionAlias("project.schedule_storage_move", "storage_move.schedule_project"),
		compatActionAlias("project.status_check_list", actionExternalStatusCheckListProject),
		compatActionAlias("project.status_checks.list", actionExternalStatusCheckListProject),
		compatActionAlias("project_member.add", actionProjectMemberAdd),
		compatActionAlias("project_member.delete", actionProjectMemberDelete),
		compatActionAlias("project_member.edit", actionProjectMemberEdit),
		compatActionAlias("project_member.get", "project.member_get"),
		compatActionAlias("project_member.remove", actionProjectMemberDelete),
		compatActionAlias("project_member.update", actionProjectMemberEdit),
		compatActionAlias("project_access_token.create", "access.token_project_create"),
		compatActionAlias("project_access_token.revoke", "access.token_project_revoke"),
		compatActionAlias("terraform_state.delete", actionAdminTerraformStateDelete),
		compatActionAlias("terraform_state.delete_version", actionAdminTerraformVersionDelete),
		compatActionAlias("terraform_state.get", actionAdminTerraformStateGet),
		compatActionAlias("terraform_state.list", actionAdminTerraformStateList),
		compatActionAlias("terraform_state.lock", actionAdminTerraformStateLock),
		compatActionAlias("terraform_state.unlock", actionAdminTerraformStateUnlock),
		compatActionAlias("terraform_state.version_delete", actionAdminTerraformVersionDelete),
		unsearchableActionAlias("repository_tree", "repository.tree", "Canonicalization compatibility alias; omitted from search to avoid over-ranking repository.tree."),
		unsearchableActionAlias("repository_tree.list", "repository.tree", "Canonicalization compatibility alias; omitted from search to avoid over-ranking repository.tree."),
		compatActionAlias("repository_file.create", "repository.file_create"),
		compatActionAlias("repository_file.delete", "repository.file_delete"),
		compatActionAlias("repository_file.get", actionRepositoryFileGet),
		compatActionAlias("repository_file.read", actionRepositoryFileGet),
		compatActionAlias("repository_files.get_raw_file", "repository.file_raw"),
		compatActionAlias("issue.link", actionIssueLinkCreate),
		compatActionAlias("pipeline.schedule_variable_create", "pipeline.schedule_create_variable"),
		compatActionAlias("pipeline.schedule_variable_delete", "pipeline.schedule_delete_variable"),
		compatActionAlias("pipeline.schedule_variable_update", "pipeline.schedule_edit_variable"),
		compatActionAlias("project.badge_update", "project.badge_edit"),
		compatActionAlias("merge_request.time_spent_reset", "merge_request.spent_time_reset"),
		compatActionAlias("generic_package.list", actionPackageList),
		compatActionAlias("issue_note.create", "issue.note_create"),
		compatActionAlias("gitlab_release.create", actionReleaseCreate),
		compatActionAlias("gitlab_release/create", actionReleaseCreate),
		compatActionAlias("release.create_link", actionReleaseLinkCreate),
		compatActionAlias("release.asset_link.create", actionReleaseLinkCreate),
		compatActionAlias("release.asset_link.delete", "release.link_delete"),
		compatActionAlias("release.asset_link.get", "release.link_get"),
		compatActionAlias("release.asset_link.list", actionReleaseLinkList),
		compatActionAlias("release.asset_link.update", "release.link_update"),
		compatActionAlias("gitlab_release.link_create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("gitlab_release/link_create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("gitlab_release_link.link_create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("gitlab_release_link/link_create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("release_link.create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("release_link.link_create", actionReleaseLinkCreate),
		compatActionAlias("release_link.link_create_batch", actionReleaseLinkCreateBatch),
		compatActionAlias("release_link.link_list", actionReleaseLinkList),
		compatActionAlias("release.generate_notes", "analyze.release_notes"),
		compatActionAlias("package.list_project", actionPackageList),
		compatActionAlias("package.list_project_packages", actionPackageList),
		compatActionAlias("variable.create", "ci_variable.create"),
		compatActionAlias("group.variable.create", "ci_variable.group_create"),
		compatActionAlias("group.audit_events", "audit_event.list_group"),
		compatActionAlias("service_account.delete", "group.service_account_delete"),
		compatActionAlias("service_account_pat.revoke", actionGroupServiceAccountPATRevoke),
		compatActionAlias("group_service_account.delete", "group.service_account_delete"),
		compatActionAlias("group_service_account.pat_revoke", actionGroupServiceAccountPATRevoke),
		compatActionAlias("group_service_account.personal_access_token_revoke", actionGroupServiceAccountPATRevoke),
		compatActionAlias("group_service_account.revoke_pat", actionGroupServiceAccountPATRevoke),
		compatActionAlias("group_service_account.update", "group.service_account_update"),
		compatActionAlias("project_service_account.delete", "project.service_account_delete"),
		compatActionAlias("project_service_account.pat_revoke", actionProjectServiceAccountPATRevoke),
		compatActionAlias("project_service_account.personal_access_token_revoke", actionProjectServiceAccountPATRevoke),
		compatActionAlias("project_service_account.revoke_pat", actionProjectServiceAccountPATRevoke),
		compatActionAlias("project_service_account.update", "project.service_account_update"),
		standaloneActionAlias("gitlab_discover_project", "discover_project.resolve"),
		standaloneActionAlias("interactive_issue.create", actionInteractiveIssueCreate),
		standaloneActionAlias("interactive_issue_create", actionInteractiveIssueCreate),
		standaloneActionAlias("gitlab_interactive_issue.create", actionInteractiveIssueCreate),
		standaloneActionAlias("gitlab_interactive_issue_create", actionInteractiveIssueCreate),
		standaloneActionAlias("gitlab_interactive_mr_create", "interactive.mr_create"),
		standaloneActionAlias("gitlab_interactive_project_create", "interactive.project_create"),
		standaloneActionAlias("gitlab_interactive_release_create", "interactive.release_create"),
		compatActionAlias("job.token_scope_remove_inbound", "job.token_scope_remove_project"),
		compatActionAlias("mr_review.draft_notes_publish_all", actionMRReviewDraftNotePublishAll),
		compatActionAlias("repository.tag.delete", "tag.delete"),
		compatActionAlias("runner.delete", "runner.remove"),
		compatActionAlias("wiki.show", "wiki.get"),
		compatActionAlias("generic_package.publish_directory", actionPackagePublishDirectory),
		compatActionAlias("generic_packages.publish_directory", actionPackagePublishDirectory),
		compatActionAlias("gitlab_package.publish_directory", actionPackagePublishDirectory),
		compatActionAlias("gitlab_package/publish_directory", actionPackagePublishDirectory),
		compatActionAlias("webhook.add", actionProjectHookAdd),
		compatActionAlias("webhook.create", actionProjectHookAdd),
		compatActionAlias("webhook.delete", "project.hook_delete"),
	}
}

func compatActionAlias(alias, canonical string) ActionAlias {
	return ActionAlias{Alias: alias, Canonical: canonical, Source: SourceCompatibility, Searchable: true, Reason: defaultActionAliasReason}
}

func standaloneActionAlias(alias, canonical string) ActionAlias {
	return ActionAlias{Alias: alias, Canonical: canonical, Source: SourceStandalone, Searchable: true, Reason: defaultActionAliasReason}
}

func unsearchableActionAlias(alias, canonical, reason string) ActionAlias {
	return ActionAlias{Alias: alias, Canonical: canonical, Source: SourceCompatibility, Reason: reason}
}

func cloneActionAliases(aliases []ActionAlias) []ActionAlias {
	out := append([]ActionAlias(nil), aliases...)
	for index := range out {
		out[index].Alias = strings.TrimSpace(strings.ToLower(out[index].Alias))
		out[index].Canonical = strings.TrimSpace(strings.ToLower(out[index].Canonical))
		out[index].Source = strings.TrimSpace(out[index].Source)
		out[index].Reason = strings.TrimSpace(out[index].Reason)
	}
	sort.SliceStable(out, func(left, right int) bool {
		if out[left].Canonical != out[right].Canonical {
			return out[left].Canonical < out[right].Canonical
		}
		return out[left].Alias < out[right].Alias
	})
	return out
}

// NormalizeActionAlias canonicalizes an unambiguous historical action alias.
func NormalizeActionAlias(actionID string) (string, bool) {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	if actionID == "" {
		return "", false
	}
	matches := actionAliasTargetIndex()[actionID]
	if len(matches) != 1 {
		return actionID, false
	}
	return matches[0], true
}

var (
	actionAliasTargetIndexOnce sync.Once
	actionAliasTargets         map[string][]string
)

func actionAliasTargetIndex() map[string][]string {
	actionAliasTargetIndexOnce.Do(func() {
		targets := make(map[string][]string)
		for _, alias := range ActionAliases() {
			targets[alias.Alias] = append(targets[alias.Alias], alias.Canonical)
		}
		for alias, matches := range targets {
			sort.Strings(matches)
			targets[alias] = compactStrings(matches)
		}
		actionAliasTargets = targets
	})
	return actionAliasTargets
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

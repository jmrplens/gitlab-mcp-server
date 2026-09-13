//go:build e2e

// actions_b3_test.go names the catalog actions the project and group
// families call that actions_test.go does not already: the project reads
// and state changes, its hooks, labels, milestones, members, badges, boards,
// export and import, integrations, Pages, mirrors and uploads; the group's
// state changes, sharing, members, uploads, export and import, relations
// export, milestones, releases and variables; and the Free families that
// hang off a project or a group: custom emoji, freeze periods, work item
// saved views, the model registry, cluster agents and feature flags.
//
// They are typed constants for the reason actions_test.go states: the
// static gate reads them out of the type checker's record and a literal
// would be invisible to it.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The project reads and the state changes of one project.
const (
	actionProjectList                 harness.ActionID = "project.list"
	actionProjectListUserProjects     harness.ActionID = "project.list_user_projects"
	actionProjectLanguages            harness.ActionID = "project.languages"
	actionProjectListUsers            harness.ActionID = "project.list_users"
	actionProjectListGroups           harness.ActionID = "project.list_groups"
	actionProjectListStarrers         harness.ActionID = "project.list_starrers"
	actionProjectStatisticsGet        harness.ActionID = "project.statistics_get"
	actionProjectRepositoryStorageGet harness.ActionID = "project.repository_storage_get"
	actionProjectListForks            harness.ActionID = "project.list_forks"
	actionProjectStar                 harness.ActionID = "project.star"
	actionProjectUnstar               harness.ActionID = "project.unstar"
	actionProjectArchive              harness.ActionID = "project.archive"
	actionProjectUnarchive            harness.ActionID = "project.unarchive"
	actionProjectFork                 harness.ActionID = "project.fork"
	actionProjectStartHousekeeping    harness.ActionID = "project.start_housekeeping"
)

// A project shared with a group, its avatar, its restore after a delete,
// its transfer, its creation on another user's behalf and its fork relation.
const (
	actionProjectShareWithGroup     harness.ActionID = "project.share_with_group"
	actionProjectListInvitedGroups  harness.ActionID = "project.list_invited_groups"
	actionProjectDeleteSharedGroup  harness.ActionID = "project.delete_shared_group"
	actionProjectUploadAvatar       harness.ActionID = "project.upload_avatar"
	actionProjectDownloadAvatar     harness.ActionID = "project.download_avatar"
	actionProjectRestore            harness.ActionID = "project.restore"
	actionProjectTransfer           harness.ActionID = "project.transfer"
	actionProjectCreateForUser      harness.ActionID = "project.create_for_user"
	actionProjectCreateForkRelation harness.ActionID = "project.create_fork_relation"
	actionProjectDeleteForkRelation harness.ActionID = "project.delete_fork_relation"
)

// A project's webhooks and what hangs off one.
const (
	actionProjectHookAdd                harness.ActionID = "project.hook_add"
	actionProjectHookList               harness.ActionID = "project.hook_list"
	actionProjectHookGet                harness.ActionID = "project.hook_get"
	actionProjectHookEdit               harness.ActionID = "project.hook_edit"
	actionProjectHookDelete             harness.ActionID = "project.hook_delete"
	actionProjectHookSetCustomHeader    harness.ActionID = "project.hook_set_custom_header"
	actionProjectHookDeleteCustomHeader harness.ActionID = "project.hook_delete_custom_header"
	actionProjectHookSetURLVariable     harness.ActionID = "project.hook_set_url_variable"
	actionProjectHookDeleteURLVariable  harness.ActionID = "project.hook_delete_url_variable"
	actionProjectHookTest               harness.ActionID = "project.hook_test"
)

// A project label's subscription and its promotion to the group.
const (
	actionProjectLabelCreate      harness.ActionID = "project.label_create"
	actionProjectLabelGet         harness.ActionID = "project.label_get"
	actionProjectLabelSubscribe   harness.ActionID = "project.label_subscribe"
	actionProjectLabelUnsubscribe harness.ActionID = "project.label_unsubscribe"
	actionProjectLabelPromote     harness.ActionID = "project.label_promote"
)

// A project milestone and the two listings scoped to one.
const (
	actionProjectMilestoneCreate        harness.ActionID = "project.milestone_create"
	actionProjectMilestoneList          harness.ActionID = "project.milestone_list"
	actionProjectMilestoneIssues        harness.ActionID = "project.milestone_issues"
	actionProjectMilestoneMergeRequests harness.ActionID = "project.milestone_merge_requests"
)

// A project's direct members and the inherited membership read.
const (
	actionProjectMemberAdd       harness.ActionID = "project.member_add"
	actionProjectMemberEdit      harness.ActionID = "project.member_edit"
	actionProjectMemberDelete    harness.ActionID = "project.member_delete"
	actionProjectMemberInherited harness.ActionID = "project.member_inherited"
)

// A project's badges.
const (
	actionProjectBadgeAdd     harness.ActionID = "project.badge_add"
	actionProjectBadgeGet     harness.ActionID = "project.badge_get"
	actionProjectBadgeList    harness.ActionID = "project.badge_list"
	actionProjectBadgeEdit    harness.ActionID = "project.badge_edit"
	actionProjectBadgeDelete  harness.ActionID = "project.badge_delete"
	actionProjectBadgePreview harness.ActionID = "project.badge_preview"
)

// A project's issue boards and their label columns.
const (
	actionProjectBoardCreate     harness.ActionID = "project.board_create"
	actionProjectBoardList       harness.ActionID = "project.board_list"
	actionProjectBoardGet        harness.ActionID = "project.board_get"
	actionProjectBoardUpdate     harness.ActionID = "project.board_update"
	actionProjectBoardDelete     harness.ActionID = "project.board_delete"
	actionProjectBoardListList   harness.ActionID = "project.board_list_list"
	actionProjectBoardListCreate harness.ActionID = "project.board_list_create"
	actionProjectBoardListGet    harness.ActionID = "project.board_list_get"
	actionProjectBoardListUpdate harness.ActionID = "project.board_list_update"
	actionProjectBoardListDelete harness.ActionID = "project.board_list_delete"
)

// A project's export and import.
const (
	actionProjectExportSchedule harness.ActionID = "project.export_schedule"
	actionProjectExportStatus   harness.ActionID = "project.export_status"
	actionProjectExportDownload harness.ActionID = "project.export_download"
	actionProjectImportStatus   harness.ActionID = "project.import_status"
	actionProjectImportFromFile harness.ActionID = "project.import_from_file"
)

// A project's integrations, and the group integrations the project tool
// also carries, the group Datadog reads among them.
const (
	actionProjectIntegrationList               harness.ActionID = "project.integration_list"
	actionProjectIntegrationGet                harness.ActionID = "project.integration_get"
	actionProjectIntegrationSet                harness.ActionID = "project.integration_set"
	actionProjectIntegrationSetJira            harness.ActionID = "project.integration_set_jira"
	actionProjectIntegrationDelete             harness.ActionID = "project.integration_delete"
	actionProjectIntegrationSetGroup           harness.ActionID = "project.integration_set_group"
	actionProjectIntegrationListGroup          harness.ActionID = "project.integration_list_group"
	actionProjectIntegrationGetGroup           harness.ActionID = "project.integration_get_group"
	actionProjectIntegrationDeleteGroup        harness.ActionID = "project.integration_delete_group"
	actionProjectIntegrationGetGroupDatadog    harness.ActionID = "project.integration_get_group_datadog"
	actionProjectIntegrationDeleteGroupDatadog harness.ActionID = "project.integration_delete_group_datadog"
)

// A project's Pages settings, domains and deployment.
const (
	actionProjectPagesGet           harness.ActionID = "project.pages_get"
	actionProjectPagesUpdate        harness.ActionID = "project.pages_update"
	actionProjectPagesDomainList    harness.ActionID = "project.pages_domain_list"
	actionProjectPagesDomainListAll harness.ActionID = "project.pages_domain_list_all"
	actionProjectPagesDomainCreate  harness.ActionID = "project.pages_domain_create"
	actionProjectPagesDomainGet     harness.ActionID = "project.pages_domain_get"
	actionProjectPagesDomainUpdate  harness.ActionID = "project.pages_domain_update"
	actionProjectPagesDomainDelete  harness.ActionID = "project.pages_domain_delete"
	actionProjectPagesUnpublish     harness.ActionID = "project.pages_unpublish"
)

// A project's push mirrors.
const (
	actionProjectMirrorList         harness.ActionID = "project.mirror_list"
	actionProjectMirrorAdd          harness.ActionID = "project.mirror_add"
	actionProjectMirrorGet          harness.ActionID = "project.mirror_get"
	actionProjectMirrorGetPublicKey harness.ActionID = "project.mirror_get_public_key"
	actionProjectMirrorEdit         harness.ActionID = "project.mirror_edit"
	actionProjectMirrorDelete       harness.ActionID = "project.mirror_delete"
	actionProjectMirrorForcePush    harness.ActionID = "project.mirror_force_push"
)

// A project's markdown uploads.
const (
	actionProjectUpload               harness.ActionID = "project.upload"
	actionProjectUploadList           harness.ActionID = "project.upload_list"
	actionProjectUploadDelete         harness.ActionID = "project.upload_delete"
	actionProjectUploadDeleteBySecret harness.ActionID = "project.upload_delete_by_secret"
)

// The group reads the old suite drove beside the lifecycle.
const (
	actionGroupList              harness.ActionID = "group.list"
	actionGroupMembers           harness.ActionID = "group.members"
	actionGroupSubgroups         harness.ActionID = "group.subgroups"
	actionGroupSharedWith        harness.ActionID = "group.shared_with"
	actionGroupInvitedGroups     harness.ActionID = "group.invited_groups"
	actionGroupTransferLocations harness.ActionID = "group.transfer_locations"
)

// A group's archive state, its issues, its avatar, its shared projects,
// the two transfers, its restore and the two ways of sharing it with
// another group.
const (
	actionGroupArchive            harness.ActionID = "group.archive"
	actionGroupUnarchive          harness.ActionID = "group.unarchive"
	actionGroupIssues             harness.ActionID = "group.issues"
	actionGroupUploadAvatar       harness.ActionID = "group.upload_avatar"
	actionGroupSharedProjects     harness.ActionID = "group.shared_projects"
	actionGroupTransferProject    harness.ActionID = "group.transfer_project"
	actionGroupTransfer           harness.ActionID = "group.transfer"
	actionGroupRestore            harness.ActionID = "group.restore"
	actionGroupShareWithGroup     harness.ActionID = "group.share_with_group"
	actionGroupUnshareFromGroup   harness.ActionID = "group.unshare_from_group"
	actionGroupMemberShare        harness.ActionID = "group.group_member_share"
	actionGroupMemberUnshare      harness.ActionID = "group.group_member_unshare"
	actionGroupMemberEdit         harness.ActionID = "group.group_member_edit"
	actionGroupMemberRemove       harness.ActionID = "group.group_member_remove"
	actionGroupUploadList         harness.ActionID = "group.group_upload_list"
	actionGroupUploadDeleteByID   harness.ActionID = "group.group_upload_delete_by_id"
	actionGroupUploadDeleteSecret harness.ActionID = "group.group_upload_delete_by_secret"
)

// A group's export, import and relations export.
const (
	actionGroupExportSchedule      harness.ActionID = "group.group_export_schedule"
	actionGroupExportDownload      harness.ActionID = "group.group_export_download"
	actionGroupImportFile          harness.ActionID = "group.group_import_file"
	actionGroupRelationsSchedule   harness.ActionID = "group.group_relations_schedule"
	actionGroupRelationsListStatus harness.ActionID = "group.group_relations_list_status"
)

// A group's milestones and the release aggregation over its projects.
const (
	actionGroupMilestoneCreate harness.ActionID = "group.group_milestone_create"
	actionGroupMilestoneList   harness.ActionID = "group.group_milestone_list"
	actionGroupMilestoneGet    harness.ActionID = "group.group_milestone_get"
	actionGroupMilestoneDelete harness.ActionID = "group.group_milestone_delete"
	actionGroupReleaseList     harness.ActionID = "group.release_list"
)

// A group's CI variables, on the CI variable tool.
const (
	actionGroupVariableCreate harness.ActionID = "ci_variable.group_create"
	actionGroupVariableList   harness.ActionID = "ci_variable.group_list"
	actionGroupVariableGet    harness.ActionID = "ci_variable.group_get"
	actionGroupVariableUpdate harness.ActionID = "ci_variable.group_update"
	actionGroupVariableDelete harness.ActionID = "ci_variable.group_delete"
)

// A group's custom emoji.
const (
	actionCustomEmojiCreate harness.ActionID = "custom_emoji.create"
	actionCustomEmojiList   harness.ActionID = "custom_emoji.list"
	actionCustomEmojiDelete harness.ActionID = "custom_emoji.delete"
)

// A project's deploy freeze periods, on the environment tool.
const (
	actionFreezePeriodCreate harness.ActionID = "environment.freeze_create"
	actionFreezePeriodList   harness.ActionID = "environment.freeze_list"
	actionFreezePeriodGet    harness.ActionID = "environment.freeze_get"
	actionFreezePeriodUpdate harness.ActionID = "environment.freeze_update"
	actionFreezePeriodDelete harness.ActionID = "environment.freeze_delete"
)

// A namespace's work item saved views, on the issue tool.
const (
	actionSavedViewList        harness.ActionID = "issue.work_item_saved_view_list"
	actionSavedViewCreate      harness.ActionID = "issue.work_item_saved_view_create"
	actionSavedViewGet         harness.ActionID = "issue.work_item_saved_view_get"
	actionSavedViewUpdate      harness.ActionID = "issue.work_item_saved_view_update"
	actionSavedViewSubscribe   harness.ActionID = "issue.work_item_saved_view_subscribe"
	actionSavedViewUnsubscribe harness.ActionID = "issue.work_item_saved_view_unsubscribe"
	actionSavedViewDelete      harness.ActionID = "issue.work_item_saved_view_delete"
)

// The model registry download, the one action of its tool the old suite
// drove.
const actionModelRegistryDownload harness.ActionID = "model_registry.download"

// A project's cluster agents and their tokens, on the admin tool.
const (
	actionClusterAgentRegister    harness.ActionID = "admin.cluster_agent_register"
	actionClusterAgentList        harness.ActionID = "admin.cluster_agent_list"
	actionClusterAgentGet         harness.ActionID = "admin.cluster_agent_get"
	actionClusterAgentTokenCreate harness.ActionID = "admin.cluster_agent_token_create"
	actionClusterAgentTokenList   harness.ActionID = "admin.cluster_agent_token_list"
	actionClusterAgentTokenGet    harness.ActionID = "admin.cluster_agent_token_get"
	actionClusterAgentTokenRevoke harness.ActionID = "admin.cluster_agent_token_revoke"
	actionClusterAgentDelete      harness.ActionID = "admin.cluster_agent_delete"
)

// A project feature flag's read, update and delete, after a create that
// builds it.
const (
	actionFeatureFlagCreate harness.ActionID = "feature_flags.feature_flag_create"
	actionFeatureFlagGet    harness.ActionID = "feature_flags.feature_flag_get"
	actionFeatureFlagUpdate harness.ActionID = "feature_flags.feature_flag_update"
	actionFeatureFlagDelete harness.ActionID = "feature_flags.feature_flag_delete"
)

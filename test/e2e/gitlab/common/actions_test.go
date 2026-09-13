//go:build e2e

// actions_test.go names every catalog action this package calls, once.
//
// They are typed constants rather than string literals at the call sites
// because the push-time static gate reads constants of the harness's
// ActionID type out of the type checker's record, follows them through
// helper parameters, and checks each against the catalog and against the
// package's tier. A literal would be invisible to it. Keeping them together
// also means a renamed action fails to compile in one place.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The Free actions the MCP-layer tests drive.
const (
	// actionServerStatus is the diagnostics read every surface serves.
	actionServerStatus harness.ActionID = "server.status"
	// actionServerHealthCheck is its twin that declares no individual tool.
	actionServerHealthCheck harness.ActionID = "server.health_check"
	// actionUserCurrent reads the authenticated user, which is how a session
	// running with another credential proves whose it is.
	actionUserCurrent harness.ActionID = "user.current"
	// actionIssueList is the read the protective modes are shown to keep.
	actionIssueList harness.ActionID = "issue.list"
	// actionIssueCreate is the write the protective modes are shown to stop.
	actionIssueCreate harness.ActionID = "issue.create"
	// actionProjectGet is a read of the World's project.
	actionProjectGet harness.ActionID = "project.get"
	// actionProjectDelete is the destructive action safe mode previews.
	actionProjectDelete harness.ActionID = "project.delete"
	// actionAdminMetadataGet belongs to the admin group, which a token
	// without admin_mode is not served.
	actionAdminMetadataGet harness.ActionID = "admin.metadata_get"
)

// A project's service accounts and their tokens.
const (
	actionProjectServiceAccountList      harness.ActionID = "project.service_account_list"
	actionProjectServiceAccountCreate    harness.ActionID = "project.service_account_create"
	actionProjectServiceAccountUpdate    harness.ActionID = "project.service_account_update"
	actionProjectServiceAccountDelete    harness.ActionID = "project.service_account_delete"
	actionProjectServiceAccountPATCreate harness.ActionID = "project.service_account_pat_create"
	actionProjectServiceAccountPATList   harness.ActionID = "project.service_account_pat_list"
	actionProjectServiceAccountPATRotate harness.ActionID = "project.service_account_pat_rotate"
	actionProjectServiceAccountPATRevoke harness.ActionID = "project.service_account_pat_revoke"
)

// The instance's service accounts, and the user delete that removes one.
const (
	actionUserListServiceAccounts  harness.ActionID = "user.list_service_accounts"
	actionUserCreateServiceAccount harness.ActionID = "user.create_service_account"
	actionUserUpdateServiceAccount harness.ActionID = "user.update_service_account"
	actionUserDelete               harness.ActionID = "user.delete"
)

// The project and snippet halves of the storage move family, which are
// Free; the group half is licensed and lives in the ee package.
const (
	actionStorageMoveRetrieveAllProject   harness.ActionID = "storage_move.retrieve_all_project"
	actionStorageMoveRetrieveProject      harness.ActionID = "storage_move.retrieve_project"
	actionStorageMoveGetProject           harness.ActionID = "storage_move.get_project"
	actionStorageMoveGetProjectForProject harness.ActionID = "storage_move.get_project_for_project"
	actionStorageMoveScheduleProject      harness.ActionID = "storage_move.schedule_project"
	actionStorageMoveScheduleAllProject   harness.ActionID = "storage_move.schedule_all_project"
	actionStorageMoveRetrieveAllSnippet   harness.ActionID = "storage_move.retrieve_all_snippet"
	actionStorageMoveRetrieveSnippet      harness.ActionID = "storage_move.retrieve_snippet"
	actionStorageMoveGetSnippet           harness.ActionID = "storage_move.get_snippet"
	actionStorageMoveGetSnippetForSnippet harness.ActionID = "storage_move.get_snippet_for_snippet"
	actionStorageMoveScheduleSnippet      harness.ActionID = "storage_move.schedule_snippet"
	actionStorageMoveScheduleAllSnippet   harness.ActionID = "storage_move.schedule_all_snippet"
)

// Group labels, driven for their archived flag.
const (
	actionGroupLabelCreate harness.ActionID = "group.group_label_create"
	actionGroupLabelList   harness.ActionID = "group.group_label_list"
	actionGroupLabelUpdate harness.ActionID = "group.group_label_update"
	actionGroupLabelDelete harness.ActionID = "group.group_label_delete"
)

// The group's own lifecycle and one membership write, which the old suite
// drove only to build the state its Enterprise scenarios stood on.
const (
	actionGroupCreate    harness.ActionID = "group.create"
	actionGroupGet       harness.ActionID = "group.get"
	actionGroupUpdate    harness.ActionID = "group.update"
	actionGroupDelete    harness.ActionID = "group.delete"
	actionGroupMemberAdd harness.ActionID = "group.group_member_add"
)

// A project's own lifecycle, and the objects the old suite created in one
// through the server before every Enterprise scenario: a branch, a commit,
// a merge request, an environment, an issue and its update, a pipeline.
const (
	actionProjectCreate          harness.ActionID = "project.create"
	actionProjectUpdate          harness.ActionID = "project.update"
	actionBranchCreate           harness.ActionID = "branch.create"
	actionRepositoryCommitCreate harness.ActionID = "repository.commit_create"
	actionMergeRequestCreate     harness.ActionID = "merge_request.create"
	actionEnvironmentCreate      harness.ActionID = "environment.create"
	actionIssueUpdate            harness.ActionID = "issue.update"
	actionPipelineCreate         harness.ActionID = "pipeline.create"
)

// A snippet's create and delete, and the read that proves the delete.
const (
	actionSnippetCreate harness.ActionID = "snippet.create"
	actionSnippetGet    harness.ActionID = "snippet.get"
	actionSnippetDelete harness.ActionID = "snippet.delete"
)

// The Free half of the group board family: the reads and the rename of a
// board the fixture library built, and the label columns of one. The
// create and the delete are Premium and live in the ee package.
const (
	actionGroupBoardList       harness.ActionID = "group.group_board_list"
	actionGroupBoardGet        harness.ActionID = "group.group_board_get"
	actionGroupBoardUpdate     harness.ActionID = "group.group_board_update"
	actionGroupBoardListLists  harness.ActionID = "group.group_board_list_lists"
	actionGroupBoardCreateList harness.ActionID = "group.group_board_create_list"
	actionGroupBoardGetList    harness.ActionID = "group.group_board_get_list"
	actionGroupBoardUpdateList harness.ActionID = "group.group_board_update_list"
	actionGroupBoardDeleteList harness.ActionID = "group.group_board_delete_list"
)

// A group's service accounts and their tokens, Free like the project's.
const (
	actionGroupServiceAccountList      harness.ActionID = "group.service_account_list"
	actionGroupServiceAccountCreate    harness.ActionID = "group.service_account_create"
	actionGroupServiceAccountUpdate    harness.ActionID = "group.service_account_update"
	actionGroupServiceAccountDelete    harness.ActionID = "group.service_account_delete"
	actionGroupServiceAccountPATCreate harness.ActionID = "group.service_account_pat_create"
	actionGroupServiceAccountPATList   harness.ActionID = "group.service_account_pat_list"
	actionGroupServiceAccountPATRotate harness.ActionID = "group.service_account_pat_rotate"
	actionGroupServiceAccountPATRevoke harness.ActionID = "group.service_account_pat_revoke"
)

// Work items, the Free half of the issue tool's GraphQL surface.
const (
	actionWorkItemTypeList harness.ActionID = "issue.work_item_type_list"
	actionWorkItemCreate   harness.ActionID = "issue.work_item_create"
	actionWorkItemList     harness.ActionID = "issue.work_item_list"
	actionWorkItemGet      harness.ActionID = "issue.work_item_get"
	actionWorkItemUpdate   harness.ActionID = "issue.work_item_update"
	actionWorkItemDelete   harness.ActionID = "issue.work_item_delete"
)

// The runner reads and the two project-scoped writes an instance runner
// refuses.
const (
	actionRunnerListAll        harness.ActionID = "runner.list_all"
	actionRunnerListProject    harness.ActionID = "runner.list_project"
	actionRunnerListGroup      harness.ActionID = "runner.list_group"
	actionRunnerGet            harness.ActionID = "runner.get"
	actionRunnerListManagers   harness.ActionID = "runner.list_managers"
	actionRunnerEnableProject  harness.ActionID = "runner.enable_project"
	actionRunnerDisableProject harness.ActionID = "runner.disable_project"
)

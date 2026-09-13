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

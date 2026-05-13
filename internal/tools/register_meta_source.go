package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectmirrors"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerProjectMeta registers the gitlab_project meta-tool with actions:
// create, get, list, update, delete, restore, fork, star, unstar, archive, unarchive, transfer, list_forks, languages,
// hook_list, hook_get, hook_add, hook_edit, hook_delete, hook_test,
// list_user_projects, list_users, list_groups, list_starrers, share_with_group, delete_shared_group, list_invited_groups,
// list_user_contributed, list_user_starred,
// members, upload, upload_list, upload_delete, label_list, label_get, label_create, label_update, label_delete,
// label_subscribe, label_unsubscribe, label_promote, milestone_list, milestone_get, milestone_create,
// milestone_update, milestone_delete, milestone_issues, milestone_merge_requests,
// integration_list, integration_get, integration_delete, integration_set_jira,
// badge_list, badge_get, badge_add, badge_edit, badge_delete, badge_preview,
// board_list, board_get, board_create, board_update, board_delete,
// board_list_list, board_list_get, board_list_create, board_list_update, board_list_delete,
// export_schedule, export_status, export_download, import_from_file, import_status,
// statistics_get, pages_get, pages_update, pages_unpublish,
// pages_domain_list_all, pages_domain_list, pages_domain_get, pages_domain_create,
// pages_domain_update, and pages_domain_delete.
func registerProjectMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := actionMap{
		"create":                   routeAction(client, projects.Create),
		"get":                      routeAction(client, projects.Get),
		"list":                     routeAction(client, projects.List),
		"update":                   routeAction(client, projects.Update),
		"delete":                   destructiveAction(client, projects.Delete),
		"restore":                  routeAction(client, projects.Restore),
		"fork":                     routeAction(client, projects.Fork),
		"star":                     routeAction(client, projects.Star),
		"unstar":                   routeAction(client, projects.Unstar),
		"archive":                  routeAction(client, projects.Archive),
		"unarchive":                routeAction(client, projects.Unarchive),
		"transfer":                 routeAction(client, projects.Transfer),
		"list_forks":               routeAction(client, projects.ListForks),
		"languages":                routeAction(client, projects.GetLanguages),
		"hook_list":                routeAction(client, projects.ListHooks),
		"hook_get":                 routeAction(client, projects.GetHook),
		"hook_add":                 routeAction(client, projects.AddHook),
		"hook_edit":                routeAction(client, projects.EditHook),
		"hook_delete":              destructiveVoidAction(client, projects.DeleteHook),
		"hook_test":                routeAction(client, projects.TriggerTestHook),
		"list_user_projects":       routeAction(client, projects.ListUserProjects),
		"list_users":               routeAction(client, projects.ListProjectUsers),
		"list_groups":              routeAction(client, projects.ListProjectGroups),
		"list_starrers":            routeAction(client, projects.ListProjectStarrers),
		"share_with_group":         routeAction(client, projects.ShareProjectWithGroup),
		"delete_shared_group":      destructiveVoidAction(client, projects.DeleteSharedProjectFromGroup),
		"list_invited_groups":      routeAction(client, projects.ListInvitedGroups),
		"list_user_contributed":    routeAction(client, projects.ListUserContributedProjects),
		"list_user_starred":        routeAction(client, projects.ListUserStarredProjects),
		"members":                  routeAction(client, members.List),
		"member_get":               routeAction(client, members.Get),
		"member_inherited":         routeAction(client, members.GetInherited),
		"member_add":               routeAction(client, members.Add),
		"member_edit":              routeAction(client, members.Edit),
		"member_delete":            destructiveVoidAction(client, members.Delete),
		"upload":                   routeActionWithRequest(client, uploads.Upload),
		"upload_list":              routeAction(client, uploads.List),
		"upload_delete":            destructiveVoidAction(client, uploads.Delete),
		"label_list":               routeAction(client, labels.List),
		"label_get":                routeAction(client, labels.Get),
		"label_create":             routeAction(client, labels.Create),
		"label_update":             routeAction(client, labels.Update),
		"label_delete":             destructiveVoidAction(client, labels.Delete),
		"label_subscribe":          routeAction(client, labels.Subscribe),
		"label_unsubscribe":        routeVoidAction(client, labels.Unsubscribe),
		"label_promote":            routeVoidAction(client, labels.Promote),
		"milestone_list":           routeAction(client, milestones.List),
		"milestone_get":            routeAction(client, milestones.Get),
		"milestone_create":         routeAction(client, milestones.Create),
		"milestone_update":         routeAction(client, milestones.Update),
		"milestone_delete":         destructiveVoidAction(client, milestones.Delete),
		"milestone_issues":         routeAction(client, milestones.GetIssues),
		"milestone_merge_requests": routeAction(client, milestones.GetMergeRequests),
		"integration_list":         routeAction(client, integrations.List),
		"integration_get":          routeAction(client, integrations.Get),
		"integration_delete":       destructiveVoidAction(client, integrations.Delete),
		"integration_set_jira":     routeAction(client, integrations.SetJira),
		"badge_list":               routeAction(client, badges.ListProject),
		"badge_get":                routeAction(client, badges.GetProject),
		"badge_add":                routeAction(client, badges.AddProject),
		"badge_edit":               routeAction(client, badges.EditProject),
		"badge_delete":             destructiveVoidAction(client, badges.DeleteProject),
		"badge_preview":            routeAction(client, badges.PreviewProject),
		"board_list":               routeAction(client, boards.ListBoards),
		"board_get":                routeAction(client, boards.GetBoard),
		"board_create":             routeAction(client, boards.CreateBoard),
		"board_update":             routeAction(client, boards.UpdateBoard),
		"board_delete":             destructiveVoidAction(client, boards.DeleteBoard),
		"board_list_list":          routeAction(client, boards.ListBoardLists),
		"board_list_get":           routeAction(client, boards.GetBoardList),
		"board_list_create":        routeAction(client, boards.CreateBoardList),
		"board_list_update":        routeAction(client, boards.UpdateBoardList),
		"board_list_delete":        destructiveVoidAction(client, boards.DeleteBoardList),
		"export_schedule":          routeAction(client, projectimportexport.ScheduleExport),
		"export_status":            routeAction(client, projectimportexport.GetExportStatus),
		"export_download":          routeAction(client, projectimportexport.ExportDownload),
		"import_from_file":         routeAction(client, projectimportexport.ImportFromFile),
		"import_status":            routeAction(client, projectimportexport.GetImportStatus),
		"statistics_get":           routeAction(client, projectstatistics.Get),
		"pages_get":                routeAction(client, pages.GetPages),
		"pages_update":             routeAction(client, pages.UpdatePages),
		"pages_unpublish":          destructiveVoidAction(client, pages.UnpublishPages),
		"pages_domain_list_all":    routeAction(client, pages.ListAllDomains),
		"pages_domain_list":        routeAction(client, pages.ListDomains),
		"pages_domain_get":         routeAction(client, pages.GetDomain),
		"pages_domain_create":      routeAction(client, pages.CreateDomain),
		"pages_domain_update":      routeAction(client, pages.UpdateDomain),
		"pages_domain_delete":      destructiveVoidAction(client, pages.DeleteDomain),

		// Extended project operations
		"hook_set_custom_header":    routeVoidAction(client, projects.SetCustomHeader),
		"hook_delete_custom_header": destructiveVoidAction(client, projects.DeleteCustomHeader),
		"hook_set_url_variable":     routeVoidAction(client, projects.SetWebhookURLVariable),
		"hook_delete_url_variable":  destructiveVoidAction(client, projects.DeleteWebhookURLVariable),
		"create_fork_relation":      routeAction(client, projects.CreateForkRelation),
		"delete_fork_relation":      destructiveVoidAction(client, projects.DeleteForkRelation),
		"upload_avatar":             routeAction(client, projects.UploadAvatar),
		"download_avatar":           routeAction(client, projects.DownloadAvatar),
		"approval_config_get":       routeAction(client, projects.GetApprovalConfig),
		"approval_config_change":    routeAction(client, projects.ChangeApprovalConfig),
		"approval_rule_list":        routeAction(client, projects.ListApprovalRules),
		"approval_rule_get":         routeAction(client, projects.GetApprovalRule),
		"approval_rule_create":      routeAction(client, projects.CreateApprovalRule),
		"approval_rule_update":      routeAction(client, projects.UpdateApprovalRule),
		"approval_rule_delete":      destructiveVoidAction(client, projects.DeleteApprovalRule),
		"pull_mirror_get":           routeAction(client, projects.GetPullMirror),
		"pull_mirror_configure":     routeAction(client, projects.ConfigurePullMirror),
		"start_mirroring":           routeVoidAction(client, projects.StartMirroring),
		"start_housekeeping":        routeVoidAction(client, projects.StartHousekeeping),
		"repository_storage_get":    routeAction(client, projects.GetRepositoryStorage),
		"create_for_user":           routeAction(client, projects.CreateForUser),
		// Remote mirrors (Free tier — verified via GitLab docs)
		"mirror_list":           routeAction(client, projectmirrors.List),
		"mirror_get":            routeAction(client, projectmirrors.Get),
		"mirror_get_public_key": routeAction(client, projectmirrors.GetPublicKey),
		"mirror_add":            routeAction(client, projectmirrors.Add),
		"mirror_edit":           routeAction(client, projectmirrors.Edit),
		"mirror_delete":         destructiveVoidAction(client, projectmirrors.Delete),
		"mirror_force_push":     destructiveVoidAction(client, projectmirrors.ForcePushUpdate),
	}

	if enterprise {
		routes["push_rule_get"] = routeAction(client, projects.GetPushRules)
		routes["push_rule_add"] = routeAction(client, projects.AddPushRule)
		routes["push_rule_edit"] = routeAction(client, projects.EditPushRule)
		routes["push_rule_delete"] = destructiveVoidAction(client, projects.DeletePushRule)
		routes["security_settings_get"] = routeAction(client, securitysettings.GetProject)
		routes["security_settings_update"] = routeAction(client, securitysettings.UpdateProject)
	}

	desc := `Manage GitLab projects end-to-end: lifecycle (create/fork/transfer/archive/delete), visibility & access (members, share, approval rules, integrations, webhooks), and advanced features (mirrors, Pages, badges, boards, labels, milestones, uploads, avatars, import/export, housekeeping). Delete, unpublish, force-push and *_delete actions are destructive.
When to use: project-level configuration and metadata. NOT for: file content/commits (use gitlab_repository), branches (gitlab_branch), wiki pages (gitlab_wiki), issues (gitlab_issue), MRs (gitlab_merge_request).

Behavior:
- Idempotent reads: every get/list/*_get/*_list action plus badge_preview, languages, repository_storage_get, statistics_get.
- Idempotent mutations: update / *_update / *_edit / star / unstar / archive / unarchive / hook_set_*. NON-idempotent: create, fork, *_create, hook_add, hook_test — each invocation queues a new webhook delivery.
- Side effects: hook_add/edit/test trigger webhook deliveries; member_add/share/edit and integration_set_* notify users; transfer relocates the project and members; export_schedule / import_from_file / start_mirroring / start_housekeeping queue long-running async work (poll *_status); upload_avatar / upload mutate storage.
- Destructive: delete (unless restore window applies), *_delete (hook/label/milestone/badge/board/integration/approval_rule/mirror/upload/pages_domain/board_list), pages_unpublish, mirror_force_push, delete_shared_group, delete_fork_relation. archive is reversible via unarchive.

Returns: list/*_list actions return paginated arrays {page, per_page, total, next_page}. CRUD/get/configure/upload actions return the resource object — including label_subscribe (returns the updated label). Pure-mutation actions (delete, *_delete, mirror_force_push, start_*, *_promote, label_unsubscribe) return {success, message}.
Errors: 404 (hint: project_id may be a numeric ID or URL-encoded path like 'group%2Frepo'), 403 (hint: most mutations require Maintainer+; settings/transfers require Owner; instance-level actions require admin), 400 (hint: visibility ∈ private/internal/public; merge_method ∈ merge/rebase_merge/ff; namespace_id must be writable by the caller).

Param conventions: * = required. Most actions need project_id* (numeric ID or URL-encoded path like 'group/repo'). List actions accept page, per_page. Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner.

Project CRUD:
- create: name*, namespace_id, description, visibility (private/internal/public), initialize_with_readme, default_branch, path, topics, merge_method (merge/rebase_merge/ff), squash_option (never/always/default_on/default_off), ci_config_path, feature toggles (issues/merge_requests/wiki/jobs/lfs/request_access_enabled)
- get: project_id*
- list: owned, search, visibility, archived, order_by, sort, topic, simple, min_access_level, last_activity_after/before, starred, membership, search_namespaces, statistics, include_pending_delete, include_hidden
- update: project_id*, name, description, visibility, default_branch, merge_method, topics, squash_option, merge_commit_template, squash_commit_template, merge_pipelines_enabled, merge_trains_enabled, approvals_before_merge, feature toggles
- delete: project_id*, permanently_remove, full_path (required when permanently_remove=true). Delayed deletion by default; permanently_remove bypasses it
- restore: project_id*

Project actions:
- fork: project_id*, name, path, namespace_id, namespace_path, visibility, branches, mr_default_target_self
- star / unstar / archive / unarchive / languages: project_id*
- transfer: project_id*, namespace* (ID or path)
- list_forks: project_id*, owned, search, visibility, order_by, sort
- create_fork_relation: project_id*, forked_from_id*
- delete_fork_relation: project_id*

Users and groups:
- list_user_projects: user_id* (ID or username), search, visibility, archived, order_by, sort, simple
- list_users / list_starrers: project_id*, search
- list_groups: project_id*, search, with_shared, shared_visible_only, skip_groups, shared_min_access_level
- share_with_group: project_id*, group_id*, group_access* (10-40), expires_at
- delete_shared_group: project_id*, group_id*
- list_invited_groups: project_id*, search, min_access_level
- list_user_contributed / list_user_starred: user_id*, search, visibility, archived, order_by, sort, simple

Members (member_*):
- members: project_id*, query (filter name/username)
- member_get / member_inherited: project_id*, user_id*
- member_add: project_id*, user_id or username*, access_level* (10-50), expires_at, member_role_id
- member_edit: project_id*, user_id*, access_level*, expires_at, member_role_id
- member_delete: project_id*, user_id*

Webhooks (hook_*) — project webhook event booleans are push_events, tag_push_events, issues_events, confidential_issues_events, merge_requests_events, note_events, confidential_note_events, job_events, pipeline_events, wiki_page_events, deployment_events, releases_events, emoji_events, and resource_access_token_events. Do not send member_events or subgroup_events for project hooks; those are group hook fields. Omit params that are not requested, and omit null values.
- hook_list: project_id*
- hook_get / hook_delete: project_id*, hook_id*
- hook_add: project_id*, url*, name, description, token, event booleans, enable_ssl_verification, push_events_branch_filter, custom_webhook_template, branch_filter_strategy
- hook_edit: project_id*, hook_id*, same params as hook_add
- hook_test: project_id*, hook_id*, event* (e.g. push_events)
- hook_set_custom_header / hook_set_url_variable: project_id*, hook_id*, key*, value*
- hook_delete_custom_header / hook_delete_url_variable: project_id*, hook_id*, key*

Labels (label_*):
- label_list: project_id*, search, with_counts, include_ancestor_groups
- label_get / label_delete / label_subscribe / label_unsubscribe / label_promote: project_id*, label_id*
- label_create: project_id*, name*, color* (hex), description, priority
- label_update: project_id*, label_id*, new_name, color, description, priority

Project milestones (milestone_* — use gitlab_group group_milestone_* only when the prompt explicitly says group milestone or gives a group_id/group path):
- milestone_list: project_id*, state (active/closed), title, search, include_ancestors
- milestone_get / milestone_delete: project_id*, milestone_iid*
- milestone_create: project_id*, title*, description, start_date, due_date
- milestone_update: project_id*, milestone_iid*, title, description, start_date, due_date, state_event (activate/close)
- milestone_issues / milestone_merge_requests: project_id*, milestone_iid*

Badges (badge_*):
- badge_list: project_id*, name
- badge_get / badge_delete: project_id*, badge_id*
- badge_add / badge_preview: project_id*, link_url*, image_url*, name
- badge_edit: project_id*, badge_id*, link_url, image_url, name

Boards (board_*):
- board_list: project_id*
- board_get / board_delete: project_id*, board_id*
- board_create: project_id*, name*
- board_update: project_id*, board_id*, name, assignee_id, milestone_id, labels, weight, hide_backlog_list, hide_closed_list
- board_list_list: project_id*, board_id*
- board_list_get / board_list_delete: project_id*, board_id*, list_id*
- board_list_create: project_id*, board_id*, label_id
- board_list_update: project_id*, board_id*, list_id*, position

Integrations (integration_*):
- integration_list: project_id*
- integration_get / integration_delete: project_id*, slug* (e.g. jira, slack, discord, datadog, jenkins, mattermost, telegram)
- integration_set_jira: project_id*, url*, username, password, active, api_url, jira_auth_type, jira_issue_prefix, commit_events, merge_requests_events, issues_enabled, project_keys

Uploads:
- upload: project_id*, filename*, file_path or content_base64 (one required). Returns Markdown embed
- upload_list: project_id*
- upload_delete: project_id*, upload_id*

Import/Export:
- export_schedule / export_status / export_download: project_id*
- import_from_file: file_path or content_base64 (one required), namespace, name, path, overwrite
- import_status: project_id*

Pages (pages_*):
- pages_get / pages_unpublish: project_id*
- pages_update: project_id*, pages_https_only, pages_access_level
- pages_domain_list_all: (admin only)
- pages_domain_list: project_id*
- pages_domain_get / pages_domain_delete: project_id*, domain*
- pages_domain_create / pages_domain_update: project_id*, domain*, certificate, key

Avatars:
- upload_avatar: project_id*, filename*, content_base64*
- download_avatar: project_id*

Approval rules (approval_*):
- approval_config_get: project_id*
- approval_config_change: project_id*, approvals_before_merge, reset_approvals_on_push, merge_requests_author_approval, merge_requests_disable_committers_approval, require_password_to_approve
- approval_rule_list: project_id*
- approval_rule_get / approval_rule_delete: project_id*, rule_id*
- approval_rule_create: project_id*, name*, approvals_required*, rule_type, user_ids, group_ids, protected_branch_ids, usernames, applies_to_all_protected_branches
- approval_rule_update: project_id*, rule_id*, name, approvals_required, user_ids, group_ids, protected_branch_ids, usernames

Pull mirroring:
- pull_mirror_get: project_id*
- pull_mirror_configure: project_id*, enabled, url, auth_user, auth_password, mirror_branch_regex, mirror_trigger_builds, only_mirror_protected_branches
- start_mirroring: project_id*

Remote mirrors (mirror_*):
- mirror_list: project_id*
- mirror_get / mirror_delete: project_id*, mirror_id*
- mirror_get_public_key: project_id*, mirror_id*
- mirror_add: project_id*, url*, enabled, keep_divergent_refs, only_protected_branches, mirror_branch_regex, auth_method (password/ssh_public_key)
- mirror_edit: project_id*, mirror_id*, enabled, keep_divergent_refs, only_protected_branches, mirror_branch_regex, auth_method
- mirror_force_push: project_id*, mirror_id*

Maintenance:
- start_housekeeping / repository_storage_get / statistics_get: project_id*

Admin:
- create_for_user: user_id*, name*, path, namespace_id, description, visibility, initialize_with_readme, default_branch, topics

See also: gitlab_repository (files/commits), gitlab_branch, gitlab_wiki, gitlab_issue, gitlab_merge_request, gitlab_discover_project (find project ID)`

	if enterprise {
		desc += `

Push Rules (Premium+ — GITLAB_ENTERPRISE=true):
- push_rule_get / push_rule_delete: project_id*
- push_rule_add / push_rule_edit: project_id*, commit_message_regex, commit_message_negative_regex, branch_name_regex, author_email_regex, file_name_regex, max_file_size, deny_delete_tag, member_check, prevent_secrets, commit_committer_check, reject_unsigned_commits, reject_non_dco_commits

Security Settings (Ultimate — GITLAB_ENTERPRISE=true):
- security_settings_get: project_id*
- security_settings_update: project_id*, secret_push_protection_enabled*`
	}

	addMetaTool(server, "gitlab_project", desc, routes, toolutil.IconProject)
}

// registerBranchMeta registers the gitlab_branch meta-tool with actions:
// create, get, list, delete, protect, unprotect, list_protected, get_protected, and update_protected.
func registerBranchMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":           routeAction(client, branches.Create),
		"get":              routeAction(client, branches.Get),
		"list":             routeAction(client, branches.List),
		"delete":           destructiveVoidAction(client, branches.Delete),
		"delete_merged":    destructiveVoidAction(client, branches.DeleteMerged),
		"protect":          routeAction(client, branches.Protect),
		"unprotect":        destructiveAction(client, branches.Unprotect),
		"list_protected":   routeAction(client, branches.ProtectedList),
		"get_protected":    routeAction(client, branches.ProtectedGet),
		"update_protected": routeAction(client, branches.ProtectedUpdate),
		"rule_list":        routeAction(client, branchrules.List),
	}

	addMetaTool(server, "gitlab_branch", `Manage Git branches and branch protections in a project, plus aggregated branch rules (GraphQL). Delete and unprotect are destructive and irreversible.
When to use: create/list/delete branches, protect or update protection on branches, audit aggregated branch rules (push/merge access, approval rules, status checks).
NOT for: file contents on a branch (use gitlab_repository file_get/file_create/...), commit operations (use gitlab_repository commit_*), tags (use gitlab_tag), opening MRs against a branch (use gitlab_merge_request).

Returns:
- list / list_protected: array of {name, default, protected, merged, commit, ...} with pagination.
- get / get_protected / create / protect / update_protected: branch or protection object.
- delete / delete_merged / unprotect: {success: bool, message: string}.
- rule_list: GraphQL aggregated view {nodes: [{name, branch_protection, approval_rules, external_status_checks}], page_info}.
Errors: 404 not found, 403 forbidden (hint: requires Maintainer+ to protect/unprotect), 400 invalid params (hint: cannot delete default or protected branches — unprotect first).

Param conventions: * = required. All actions need project_id* (numeric or url-encoded path) except rule_list which uses project_path*. Access levels: 0 = no one, 30 = Developer, 40 = Maintainer.

- create: project_id*, branch_name*, ref* (branch/tag/SHA)
- get / delete: project_id*, branch_name*
- list: project_id*, search, page, per_page
- delete_merged: project_id* — deletes all merged branches except default/protected
- protect: project_id*, branch_name*, push_access_level (0/30/40), merge_access_level (0/30/40), allow_force_push (bool)
- unprotect: project_id*, branch_name*
- list_protected: project_id*
- get_protected: project_id*, branch_name*
- update_protected: project_id*, branch_name*, allow_force_push (bool), code_owner_approval_required (bool)
- rule_list: project_path* (e.g. my-group/my-project), first (max 100), after (cursor)

See also: gitlab_repository (file/commit operations on a branch), gitlab_merge_request (open MRs against a branch), gitlab_tag (tag CRUD/protection).`, routes, toolutil.IconBranch)
}

// registerTagMeta registers the gitlab_tag meta-tool with actions:
// create, get, list, delete, get_signature, list_protected, get_protected,
// protect, and unprotect.
func registerTagMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":         routeAction(client, tags.Create),
		"get":            routeAction(client, tags.Get),
		"list":           routeAction(client, tags.List),
		"delete":         destructiveVoidAction(client, tags.Delete),
		"get_signature":  routeAction(client, tags.GetSignature),
		"list_protected": routeAction(client, tags.ListProtectedTags),
		"get_protected":  routeAction(client, tags.GetProtectedTag),
		"protect":        routeAction(client, tags.ProtectTag),
		"unprotect":      destructiveVoidAction(client, tags.UnprotectTag),
	}

	addMetaTool(server, "gitlab_tag", `Manage Git tags and tag protections in a project, plus GPG signature inspection. Delete is destructive and also removes any release attached to the tag.
When to use: create/list/delete tags, protect or unprotect tag patterns, verify a tag's GPG/X.509 signature.
NOT for: releases (use gitlab_release — a release wraps a tag with notes/assets), branches (use gitlab_branch), repository file/commit operations (use gitlab_repository).

Returns:
- list / list_protected: array of {name, target, message, protected, ...} with pagination.
- get / create / get_protected / protect: tag or protection object.
- get_signature: {signature_type, gpg_key_id, verification_status, ...} or X.509 equivalent.
- delete / unprotect: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: requires Maintainer+ to protect/unprotect), 400 invalid params (hint: tag name must not exist for create).

Param conventions: * = required. All actions need project_id*. Access levels: 0 = no one, 30 = Developer, 40 = Maintainer.

- create: project_id*, tag_name*, ref* (branch/tag/SHA), message (annotated tag if non-empty)
- get / delete: project_id*, tag_name*
- list: project_id*, search, order_by (name/updated/version), sort (asc/desc)
- get_signature: project_id*, tag_name*
- list_protected: project_id*
- get_protected / unprotect: project_id*, tag_name*
- protect: project_id*, tag_name* (literal or wildcard e.g. 'v*'), create_access_level (0/30/40), allowed_to_create (array of {user_id|group_id|deploy_key_id|access_level})

See also: gitlab_release (releases use tags as anchors), gitlab_repository (commits referenced by tags), gitlab_branch (branches).`, routes, toolutil.IconTag)
}

// registerReleaseMeta registers the gitlab_release meta-tool with actions:
// create, get, get_latest, list, update, delete, link_create, link_create_batch,
// link_get, link_list, link_update, and link_delete.
func registerReleaseMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":            routeAction(client, releases.Create),
		"get":               routeAction(client, releases.Get),
		"get_latest":        routeAction(client, releases.GetLatest),
		"list":              routeAction(client, releases.List),
		"update":            routeAction(client, releases.Update),
		"delete":            destructiveAction(client, releases.Delete),
		"link_create":       routeAction(client, releaselinks.Create),
		"link_create_batch": routeAction(client, releaselinks.CreateBatch),
		"link_get":          routeAction(client, releaselinks.Get),
		"link_list":         routeAction(client, releaselinks.List),
		"link_update":       routeAction(client, releaselinks.Update),
		"link_delete":       destructiveAction(client, releaselinks.Delete),
	}

	addMetaTool(server, "gitlab_release", `Manage GitLab releases and their asset links (binaries, packages, runbooks). Releases wrap a Git tag with notes, milestones and downloadable assets. Delete is destructive: it removes the release but preserves the underlying tag.
When to use: publish a release for a tag, create a release and its tag from a ref, list/get/update releases, attach asset links to a release, batch-attach links after a CI build.
NOT for: uploading binaries to the package registry (use gitlab_package), milestones (use gitlab_project milestone_*).

Returns:
- list: array of releases with pagination.
- get / get_latest / create / update: release object {name, tag_name, description, released_at, assets: {sources, links}, evidences, milestones}.
- link_list: array of {id, name, url, link_type, direct_asset_path}.
- link_create / link_create_batch / link_get / link_update: link object(s).
- delete / link_delete: {success: bool, message: string}.
Errors: 404 not found (hint: verify tag_name), 403 forbidden (hint: requires Developer+ for create, Maintainer+ for update/delete), 400 invalid params (hint: link url must be absolute https://).

Param conventions: * = required. All actions need project_id*. Release actions need tag_name*. Link actions need tag_name* + link_id* (except link_create / link_create_batch / link_list).

Releases:
- create: project_id*, tag_name*, ref (branch/SHA when tag_name does not exist or the prompt says from ref), name, description (Markdown), released_at (ISO 8601), milestones ([]string), tag_message
- get: project_id*, tag_name*
- get_latest: project_id*
- list: project_id*, order_by (released_at/created_at), sort (asc/desc), page, per_page
- update: project_id*, tag_name*, name, description, released_at, milestones
- delete: project_id*, tag_name*

Asset links:
- link_create: project_id*, tag_name*, name*, url*, link_type (runbook/package/image/other), filepath, direct_asset_path
- link_create_batch: project_id*, tag_name*, links* (array of {name, url, link_type, filepath, direct_asset_path})
- link_get: project_id*, tag_name*, link_id*
- link_list: project_id*, tag_name*, page, per_page
- link_update: project_id*, tag_name*, link_id*, name, url, filepath, direct_asset_path, link_type
- link_delete: project_id*, tag_name*, link_id*

See also: gitlab_tag (standalone tag CRUD), gitlab_package (upload binaries; link_create can point at the package URL), gitlab_project (milestones referenced by releases).`, routes, toolutil.IconRelease)
}

// registerRepositoryMeta registers the gitlab_repository meta-tool with actions:
// tree, compare, contributors, merge_base, blob, raw_blob, archive, changelog,
// commit operations, file operations (including file_raw_metadata),
// update_submodule, and markdown_render.
func registerRepositoryMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"tree":                          routeAction(client, repository.Tree),
		"compare":                       routeAction(client, repository.Compare),
		"contributors":                  routeAction(client, repository.Contributors),
		"merge_base":                    routeAction(client, repository.MergeBase),
		"blob":                          routeAction(client, repository.Blob),
		"raw_blob":                      routeAction(client, repository.RawBlobContent),
		"archive":                       routeAction(client, repository.Archive),
		"changelog_add":                 routeAction(client, repository.AddChangelog),
		"changelog_generate":            routeAction(client, repository.GenerateChangelogData),
		"commit_create":                 routeAction(client, commits.Create),
		"commit_list":                   routeAction(client, commits.List),
		"commit_get":                    routeAction(client, commits.Get),
		"commit_diff":                   routeAction(client, commits.Diff),
		"commit_refs":                   routeAction(client, commits.GetRefs),
		"commit_comments":               routeAction(client, commits.GetComments),
		"commit_comment_create":         routeAction(client, commits.PostComment),
		"commit_statuses":               routeAction(client, commits.GetStatuses),
		"commit_status_set":             routeAction(client, commits.SetStatus),
		"commit_merge_requests":         routeAction(client, commits.ListMRsByCommit),
		"commit_cherry_pick":            routeAction(client, commits.CherryPick),
		"commit_revert":                 routeAction(client, commits.Revert),
		"commit_signature":              routeAction(client, commits.GetGPGSignature),
		"file_get":                      routeAction(client, files.Get),
		"file_create":                   routeAction(client, files.Create),
		"file_update":                   routeAction(client, files.Update),
		"file_delete":                   destructiveVoidAction(client, files.Delete),
		"file_blame":                    routeAction(client, files.Blame),
		"file_metadata":                 routeAction(client, files.GetMetaData),
		"file_raw":                      routeAction(client, files.GetRaw),
		"file_raw_metadata":             routeAction(client, files.GetRawFileMetaData),
		"update_submodule":              routeAction(client, repositorysubmodules.Update),
		"list_submodules":               routeAction(client, repositorysubmodules.List),
		"read_submodule_file":           routeAction(client, repositorysubmodules.Read),
		"markdown_render":               routeAction(client, markdown.Render),
		"commit_discussion_list":        routeAction(client, commitdiscussions.List),
		"commit_discussion_get":         routeAction(client, commitdiscussions.Get),
		"commit_discussion_create":      routeAction(client, commitdiscussions.Create),
		"commit_discussion_add_note":    routeAction(client, commitdiscussions.AddNote),
		"commit_discussion_update_note": routeAction(client, commitdiscussions.UpdateNote),
		"commit_discussion_delete_note": destructiveVoidAction(client, commitdiscussions.DeleteNote),
		"file_history":                  routeAction(client, commits.List),
	}

	addMetaTool(server, "gitlab_repository", `Browse and manage GitLab repository content: file tree, read/write/delete files, commits, diffs, cherry-pick, revert, blame, compare branches, contributors, archives, changelogs, submodules, render markdown, and commit discussions. File delete is destructive.
	When to use: exact file/commit operations, diffs for a known commit SHA, blame, compare, archives, submodules, markdown rendering. NOT for: full-text code search (use gitlab_search action code), MR changes/diffs by merge_request_iid (use gitlab_mr_review changes_get/raw_diffs), branch CRUD (use gitlab_branch), tag CRUD (use gitlab_tag).

Behavior:
- Idempotent reads: tree, blob, raw_blob, archive, compare, merge_base, contributors, file_get/raw/metadata/raw_metadata/blame, list_submodules, read_submodule_file, file_history, commit_list/get/diff/refs/comments/merge_requests/statuses/signature, commit_discussion_list/get, markdown_render, changelog_generate.
- file_create / file_update / file_delete / commit_create / commit_cherry_pick / commit_revert / update_submodule / changelog_add are NON-idempotent: when preconditions are satisfied each call produces a new commit SHA; otherwise they fail (e.g. 400 on conflict, 409 on stale last_commit_id). Use last_commit_id on file_update/file_delete for optimistic-concurrency safety.
- commit_status_set is idempotent per (sha, name, ref); commit_comment_create / commit_discussion_create / commit_discussion_add_note append rather than replace. commit_discussion_update_note replaces the existing note body.
- Side effects: any commit-producing action triggers webhooks, CI pipelines, and protected-branch / push-rule checks; archive returns large binary payloads (base64).
- File delete is destructive at the working-tree level but git history is preserved.

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation.
Errors: 404 (hint: confirm project_id, ref/sha, and file_path — paths are URL-encoded), 403 (hint: file_create/file_update/file_delete and commit_* require Developer+; protected branches may need Maintainer+), 400 (hint: file_update/file_delete accept last_commit_id for optimistic concurrency; commit_create needs at least one entry in actions).

Param conventions: * = required. Most actions need project_id*. List actions accept page, per_page.

Repository browsing:
- tree: project_id*, path, ref, recursive
- compare: project_id*, from*, to*, straight
- contributors: project_id*, order_by (name/email/commits), sort
- merge_base: project_id*, refs* (array of 2+ branch/tag/SHA)
- blob / raw_blob: project_id*, sha*
- archive: project_id*, sha, format (tar.gz/zip/tar.bz2), path

Changelogs:
- changelog_add: project_id*, version*, branch, config_file, file, from, to, message, trailer
- changelog_generate: project_id*, version*, config_file, from, to, trailer

Commits:
- commit_create: project_id*, branch*, commit_message*, actions* (array of {action: create/update/delete/move, file_path, content, previous_path}), start_branch, author_email, author_name
- commit_list: project_id*, ref_name, since, until, path, author, with_stats
- file_history: alias for commit_list filtered by path* — list commits modifying a specific file
- commit_get: project_id*, sha*
- commit_diff: project_id*, sha* — commit SHA only; not for merge_request_iid or MR changes.
- commit_refs: project_id*, sha*, type (branch/tag/all)
- commit_comments / commit_merge_requests: project_id*, sha*
- commit_comment_create: project_id*, sha*, note*, path, line, line_type (new/old)
- commit_statuses: project_id*, sha*, ref, stage, name, pipeline_id, all
- commit_status_set: project_id*, sha*, state* (pending/running/success/failed/canceled), ref, name, context, target_url, description, coverage, pipeline_id
- commit_cherry_pick: project_id*, sha*, branch*, dry_run, message
- commit_revert: project_id*, sha*, branch*
- commit_signature: project_id*, sha*

Files:
- file_get / file_raw / file_metadata / file_raw_metadata / file_blame: project_id*, file_path*, ref. Use only when the exact repository file_path is known; use gitlab_search/code for text search across files. Blame also accepts range_start, range_end.
  - file_metadata: HEAD-style metadata for the file content endpoint (size, encoding, content_sha256, blob_id, last_commit_id, ref).
  - file_raw_metadata: HEAD-style metadata for the raw file endpoint (size, content_type, ref) — useful to size-check before downloading via file_raw.
- file_create: project_id*, file_path*, branch*, commit_message*, content, start_branch, encoding (text/base64), author_email, author_name, execute_filemode
- file_update: project_id*, file_path*, branch*, commit_message*, content, start_branch, encoding, author_email, author_name, last_commit_id, execute_filemode
- file_delete: project_id*, file_path*, branch*, commit_message*, start_branch, author_email, author_name, last_commit_id

Submodules:
- update_submodule: project_id*, submodule* (URL-encoded path), branch*, commit_sha*, commit_message
- list_submodules: project_id*, ref
- read_submodule_file: project_id*, submodule_path*, file_path*, ref

Markdown:
- markdown_render: text*, gfm, project (path for resolving references)

Commit discussions:
- commit_discussion_list: project_id*, commit_id*
- commit_discussion_get: project_id*, commit_id*, discussion_id*
- commit_discussion_create: project_id*, commit_id*, body*, position
- commit_discussion_add_note: project_id*, commit_id*, discussion_id*, body*
- commit_discussion_update_note: project_id*, commit_id*, discussion_id*, note_id*, body*
- commit_discussion_delete_note: project_id*, commit_id*, discussion_id*, note_id*

See also: gitlab_branch, gitlab_tag, gitlab_project, gitlab_merge_request`, routes, toolutil.IconFile)
}

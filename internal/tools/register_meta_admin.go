package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appearance"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/applications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/appstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/avatar"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dbmigrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencyproxy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupepicboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupreleases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/modelregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// registerGroupMeta registers the gitlab_group meta-tool with actions:
// list, get, create, update, delete, restore, archive, unarchive, search, transfer_project, projects,
// members, subgroups, issues, hook_list, hook_get, hook_add, hook_edit, hook_delete,
// epic_list, epic_get, epic_get_links, epic_create, epic_update, epic_delete,
// epic_issue_list, epic_issue_assign, epic_issue_remove, epic_issue_update,
// epic_note_list, epic_note_get, epic_note_create, epic_note_update, epic_note_delete,
// epic_board_list, epic_board_get,
// wiki_list, wiki_get, wiki_create, wiki_edit, wiki_delete,
// protected_branch_list, protected_branch_get, protected_branch_protect,
// protected_branch_update, protected_branch_unprotect,
// protected_env_list, protected_env_get, protected_env_protect,
// protected_env_update, protected_env_unprotect,
// release_list,
// ldap_link_list, ldap_link_add, ldap_link_delete, ldap_link_delete_for_provider,
// saml_link_list, saml_link_get, saml_link_add, saml_link_delete,
// service_account_list, service_account_create, service_account_update,
// service_account_delete, service_account_pat_list, service_account_pat_create,
// service_account_pat_revoke.
func registerGroupMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := actionMap{
		"list":                           routeAction(client, groups.List),
		"get":                            routeAction(client, groups.Get),
		"create":                         routeAction(client, groups.Create),
		"update":                         routeAction(client, groups.Update),
		"delete":                         destructiveVoidAction(client, groups.Delete),
		"restore":                        routeAction(client, groups.Restore),
		"archive":                        routeVoidAction(client, groups.Archive),
		"unarchive":                      routeVoidAction(client, groups.Unarchive),
		"search":                         routeAction(client, groups.Search),
		"transfer_project":               routeAction(client, groups.TransferProject),
		"projects":                       routeAction(client, groups.ListProjects),
		"members":                        routeAction(client, groups.MembersList),
		"subgroups":                      routeAction(client, groups.SubgroupsList),
		"issues":                         routeAction(client, issues.ListGroup),
		"hook_list":                      routeAction(client, groups.ListHooks),
		"hook_get":                       routeAction(client, groups.GetHook),
		"hook_add":                       routeAction(client, groups.AddHook),
		"hook_edit":                      routeAction(client, groups.EditHook),
		"hook_delete":                    destructiveVoidAction(client, groups.DeleteHook),
		"badge_list":                     routeAction(client, badges.ListGroup),
		"badge_get":                      routeAction(client, badges.GetGroup),
		"badge_add":                      routeAction(client, badges.AddGroup),
		"badge_edit":                     routeAction(client, badges.EditGroup),
		"badge_delete":                   destructiveVoidAction(client, badges.DeleteGroup),
		"badge_preview":                  routeAction(client, badges.PreviewGroup),
		"group_member_get":               routeAction(client, groupmembers.GetMember),
		"group_member_get_inherited":     routeAction(client, groupmembers.GetInheritedMember),
		"group_member_add":               routeAction(client, groupmembers.AddMember),
		"group_member_edit":              routeAction(client, groupmembers.EditMember),
		"group_member_remove":            destructiveVoidAction(client, groupmembers.RemoveMember),
		"group_member_share":             routeAction(client, groupmembers.ShareGroup),
		"group_member_unshare":           routeVoidAction(client, groupmembers.UnshareGroup),
		"group_label_list":               routeAction(client, grouplabels.List),
		"group_label_get":                routeAction(client, grouplabels.Get),
		"group_label_create":             routeAction(client, grouplabels.Create),
		"group_label_update":             routeAction(client, grouplabels.Update),
		"group_label_delete":             destructiveVoidAction(client, grouplabels.Delete),
		"group_label_subscribe":          routeAction(client, grouplabels.Subscribe),
		"group_label_unsubscribe":        routeVoidAction(client, grouplabels.Unsubscribe),
		"group_milestone_list":           routeAction(client, groupmilestones.List),
		"group_milestone_get":            routeAction(client, groupmilestones.Get),
		"group_milestone_create":         routeAction(client, groupmilestones.Create),
		"group_milestone_update":         routeAction(client, groupmilestones.Update),
		"group_milestone_delete":         destructiveVoidAction(client, groupmilestones.Delete),
		"group_milestone_issues":         routeAction(client, groupmilestones.GetIssues),
		"group_milestone_merge_requests": routeAction(client, groupmilestones.GetMergeRequests),
		"group_milestone_burndown":       routeAction(client, groupmilestones.GetBurndownChartEvents),
		"group_board_list":               routeAction(client, groupboards.ListGroupBoards),
		"group_board_get":                routeAction(client, groupboards.GetGroupBoard),
		"group_board_create":             routeAction(client, groupboards.CreateGroupBoard),
		"group_board_update":             routeAction(client, groupboards.UpdateGroupBoard),
		"group_board_delete":             destructiveVoidAction(client, groupboards.DeleteGroupBoard),
		"group_board_list_lists":         routeAction(client, groupboards.ListGroupBoardLists),
		"group_board_get_list":           routeAction(client, groupboards.GetGroupBoardList),
		"group_board_create_list":        routeAction(client, groupboards.CreateGroupBoardList),
		"group_board_update_list":        routeAction(client, groupboards.UpdateGroupBoardList),
		"group_board_delete_list":        destructiveVoidAction(client, groupboards.DeleteGroupBoardList),
		"group_upload_list":              routeAction(client, groupmarkdownuploads.List),
		"group_upload_delete_by_id":      destructiveVoidAction(client, groupmarkdownuploads.DeleteByID),
		"group_upload_delete_by_secret":  destructiveVoidAction(client, groupmarkdownuploads.DeleteBySecretAndFilename),
		"group_relations_schedule":       routeVoidAction(client, grouprelationsexport.ScheduleExport),
		"group_relations_list_status":    routeAction(client, grouprelationsexport.ListExportStatus),
		"group_export_schedule":          routeAction(client, groupimportexport.ScheduleExport),
		"group_export_download":          routeAction(client, groupimportexport.ExportDownload),
		"group_import_file":              routeAction(client, groupimportexport.ImportFile),
		// Group releases (Free tier — verified via GitLab docs and E2E on CE)
		"release_list": routeAction(client, groupreleases.List),
	}

	if enterprise {
		// Group service accounts (EE-only — returns 404 on CE)
		routes["service_account_list"] = routeAction(client, groupserviceaccounts.List)
		routes["service_account_create"] = routeAction(client, groupserviceaccounts.Create)
		routes["service_account_update"] = routeAction(client, groupserviceaccounts.Update)
		routes["service_account_delete"] = destructiveVoidAction(client, groupserviceaccounts.Delete)
		routes["service_account_pat_list"] = routeAction(client, groupserviceaccounts.ListPATs)
		routes["service_account_pat_create"] = routeAction(client, groupserviceaccounts.CreatePAT)
		routes["service_account_pat_revoke"] = destructiveVoidAction(client, groupserviceaccounts.RevokePAT)
		routes["epic_discussion_list"] = routeAction(client, epicdiscussions.List)
		routes["epic_discussion_get"] = routeAction(client, epicdiscussions.Get)
		routes["epic_discussion_create"] = routeAction(client, epicdiscussions.Create)
		routes["epic_discussion_add_note"] = routeAction(client, epicdiscussions.AddNote)
		routes["epic_discussion_update_note"] = routeAction(client, epicdiscussions.UpdateNote)
		routes["epic_discussion_delete_note"] = destructiveVoidAction(client, epicdiscussions.DeleteNote)
		routes["epic_list"] = routeAction(client, epics.List)
		routes["epic_get"] = routeAction(client, epics.Get)
		routes["epic_get_links"] = routeAction(client, epics.GetLinks)
		routes["epic_create"] = routeAction(client, epics.Create)
		routes["epic_update"] = routeAction(client, epics.Update)
		routes["epic_delete"] = destructiveVoidAction(client, epics.Delete)
		routes["epic_issue_list"] = routeAction(client, epicissues.List)
		routes["epic_issue_assign"] = routeAction(client, epicissues.Assign)
		routes["epic_issue_remove"] = destructiveAction(client, epicissues.Remove)
		routes["epic_issue_update"] = routeAction(client, epicissues.UpdateOrder)
		routes["epic_note_list"] = routeAction(client, epicnotes.List)
		routes["epic_note_get"] = routeAction(client, epicnotes.Get)
		routes["epic_note_create"] = routeAction(client, epicnotes.Create)
		routes["epic_note_update"] = routeAction(client, epicnotes.Update)
		routes["epic_note_delete"] = destructiveVoidAction(client, epicnotes.Delete)
		routes["epic_board_list"] = routeAction(client, groupepicboards.List)
		routes["epic_board_get"] = routeAction(client, groupepicboards.Get)
		routes["wiki_list"] = routeAction(client, groupwikis.List)
		routes["wiki_get"] = routeAction(client, groupwikis.Get)
		routes["wiki_create"] = routeAction(client, groupwikis.Create)
		routes["wiki_edit"] = routeAction(client, groupwikis.Edit)
		routes["wiki_delete"] = destructiveVoidAction(client, groupwikis.Delete)
		routes["protected_branch_list"] = routeAction(client, groupprotectedbranches.List)
		routes["protected_branch_get"] = routeAction(client, groupprotectedbranches.Get)
		routes["protected_branch_protect"] = routeAction(client, groupprotectedbranches.Protect)
		routes["protected_branch_update"] = routeAction(client, groupprotectedbranches.Update)
		routes["protected_branch_unprotect"] = destructiveVoidAction(client, groupprotectedbranches.Unprotect)
		routes["protected_env_list"] = routeAction(client, groupprotectedenvs.List)
		routes["protected_env_get"] = routeAction(client, groupprotectedenvs.Get)
		routes["protected_env_protect"] = routeAction(client, groupprotectedenvs.Protect)
		routes["protected_env_update"] = routeAction(client, groupprotectedenvs.Update)
		routes["protected_env_unprotect"] = destructiveVoidAction(client, groupprotectedenvs.Unprotect)
		routes["ldap_link_list"] = routeAction(client, groupldap.List)
		routes["ldap_link_add"] = routeAction(client, groupldap.Add)
		routes["ldap_link_delete"] = destructiveVoidAction(client, groupldap.DeleteWithCNOrFilter)
		routes["ldap_link_delete_for_provider"] = destructiveVoidAction(client, groupldap.DeleteForProvider)
		routes["saml_link_list"] = routeAction(client, groupsaml.List)
		routes["saml_link_get"] = routeAction(client, groupsaml.Get)
		routes["saml_link_add"] = routeAction(client, groupsaml.Add)
		routes["saml_link_delete"] = destructiveVoidAction(client, groupsaml.Delete)
		routes["analytics_issues_count"] = routeAction(client, groupanalytics.GetIssuesCount)
		routes["analytics_mr_count"] = routeAction(client, groupanalytics.GetMRCount)
		routes["analytics_members_count"] = routeAction(client, groupanalytics.GetMembersCount)
		routes["credential_list_pats"] = routeAction(client, groupcredentials.ListPATs)
		routes["credential_list_ssh_keys"] = routeAction(client, groupcredentials.ListSSHKeys)
		routes["credential_revoke_pat"] = destructiveVoidAction(client, groupcredentials.RevokePAT)
		routes["credential_delete_ssh_key"] = destructiveVoidAction(client, groupcredentials.DeleteSSHKey)
		routes["ssh_cert_list"] = routeAction(client, groupsshcerts.List)
		routes["ssh_cert_create"] = routeAction(client, groupsshcerts.Create)
		routes["ssh_cert_delete"] = destructiveVoidAction(client, groupsshcerts.Delete)
		routes["security_settings_update"] = routeAction(client, securitysettings.UpdateGroup)
	}

	desc := `Manage GitLab groups: CRUD, subgroups, members, labels, milestones, webhooks, badges, boards, uploads, and import/export.
When to use: group-level operations (groups, subgroups, members, labels, milestones, boards, webhooks, badges, wikis, epics). NOT for: project-specific operations (use gitlab_project or gitlab_merge_request), user accounts (use gitlab_user), cross-project search (use gitlab_search).

Behavior:
- Idempotent reads: get / list / projects / members / subgroups / issues / search / *_list / *_get / hook_list / badge_list / group_label_list / group_milestone_list / group_board_list / release_list (Premium+ adds wiki_list / epic_list / protected_*_list / ldap_link_list / saml_link_list / service_account_*_list).
- update / *_update / *_edit / archive / unarchive are idempotent (same input → same state). create / *_create / hook_add are NON-idempotent (re-invocation creates a duplicate or returns 409).
- Side effects: group_member_add / group_member_share / group_member_edit may notify the invited user/group; hook_add / hook_edit trigger webhook deliveries; transfer_project moves repository data and re-permissions members; ldap/saml link mutations re-evaluate group membership on next sign-in (read-after-write may lag).
- Destructive: delete cascades to subgroups, projects, members, issues, MRs (irreversible when permanently_remove=true; restore window applies otherwise); hook_delete, badge_delete, group_label_delete, group_milestone_delete, group_board_delete, group_upload_delete_* are irreversible. Premium+ adds destructive: epic_delete, wiki_delete, protected_*_unprotect, ldap_link_delete*, saml_link_delete, service_account_delete, service_account_pat_revoke. archive is reversible via unarchive.

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation.
Errors: 404 (hint: group_id can be numeric ID or URL-encoded full path), 403 (hint: most mutations require Maintainer+; group_member_share, transfer_project and service_account_* require Owner), 400 (hint: visibility ∈ private/internal/public; permanently_remove=true requires full_path).

Param conventions: * = required. Most actions need group_id* (numeric ID or URL-encoded path like 'group/subgroup'). List actions accept page, per_page.

Group CRUD:
- list: search, owned, top_level_only (no group_id needed)
- get: group_id*
- create: name*, path, description, visibility (private/internal/public), parent_id, request_access_enabled, lfs_enabled, default_branch
- update: group_id*, name, path, description, visibility, request_access_enabled, lfs_enabled, default_branch
- delete: group_id*, permanently_remove, full_path (required when permanently_remove=true)
- restore: group_id*
- archive / unarchive: group_id* (requires Owner role)
- search: query* (no group_id needed)
- transfer_project: group_id*, project_id*

Group queries:
- projects: group_id*, search, include_subgroups (recommended for hierarchies), archived, visibility, order_by, sort, simple, owned, starred, with_shared
- members: group_id*, query (filter name/username)
- subgroups: group_id*, search
- issues: group_id*, state, labels, milestone, scope, search, assignee_username, author_username

Webhooks (hook_*):
- hook_list: group_id*
- hook_get / hook_delete: group_id*, hook_id*
- hook_add: group_id*, url*, name, description, token, event booleans (push/tag_push/merge_requests/issues/note/job/pipeline/wiki_page/deployment/releases/subgroup/member_events), enable_ssl_verification, push_events_branch_filter
- hook_edit: group_id*, hook_id*, same params as hook_add

Badges (badge_*):
- badge_list: group_id*, name
- badge_get / badge_delete: group_id*, badge_id*
- badge_add / badge_preview: group_id*, link_url*, image_url*, name
- badge_edit: group_id*, badge_id*, link_url, image_url, name

Members (group_member_*):
- group_member_get: group_id*, user_id*
- group_member_get_inherited: group_id*, user_id* (includes inherited)
- group_member_add / group_member_edit: group_id*, user_id*, access_level*, expires_at
- group_member_remove: group_id*, user_id*
- group_member_share: group_id*, shared_with_group_id*, group_access*, expires_at
- group_member_unshare: group_id*, shared_with_group_id*

Labels (group_label_*):
- group_label_list: group_id*, search, with_counts, include_ancestor_groups, include_descendant_groups
- group_label_get / group_label_delete / group_label_subscribe / group_label_unsubscribe: group_id*, label_id*
- group_label_create: group_id*, name*, color*, description
- group_label_update: group_id*, label_id*, new_name, color, description

Group milestones (group_milestone_* — group scope only; use gitlab_project milestone_* for project milestones):
- group_milestone_list: group_id*, state, title, search, include_ancestors, include_descendants
- group_milestone_get / group_milestone_delete: group_id*, milestone_iid*
- group_milestone_create: group_id*, title*, description, start_date, due_date
- group_milestone_update: group_id*, milestone_iid*, title, description, start_date, due_date, state_event
- group_milestone_issues / group_milestone_merge_requests / group_milestone_burndown: group_id*, milestone_iid*

Boards (group_board_*):
- group_board_list: group_id*
- group_board_get / group_board_delete: group_id*, board_id*
- group_board_create: group_id*, name*
- group_board_update: group_id*, board_id*, name, assignee_id, milestone_id, labels, weight
- group_board_list_lists: group_id*, board_id*
- group_board_get_list / group_board_delete_list: group_id*, board_id*, list_id*
- group_board_create_list: group_id*, board_id*, label_id
- group_board_update_list: group_id*, board_id*, list_id*, position

Uploads:
- group_upload_list: group_id*
- group_upload_delete_by_id: group_id*, upload_id*
- group_upload_delete_by_secret: group_id*, secret*, filename*

Import/Export:
- group_relations_schedule / group_relations_list_status: group_id*
- group_export_schedule / group_export_download: group_id*
- group_import_file: name*, path*, file*, parent_id (no group_id)

Releases:
- release_list: group_id*, simple`

	if enterprise {
		desc += `

Premium+ behavior notes (GITLAB_ENTERPRISE=true): service_account_pat_create returns the cleartext token only ONCE — store it immediately. service_account_delete and service_account_pat_revoke are irreversible.

Epics (Premium+ — GITLAB_ENTERPRISE=true. CRUD/notes/discussions use Work Items GraphQL API with full_path + iid. Only epic_get_links and epic boards use REST with group_id):

Epic discussions (epic_discussion_*): full_path*, epic_iid* for all. GraphQL pagination: first, after, last, before.
- epic_discussion_list / epic_discussion_get (+ discussion_id*)
- epic_discussion_create: body*
- epic_discussion_add_note: discussion_id*, body*
- epic_discussion_update_note: note_id*, body*
- epic_discussion_delete_note: note_id*

Epic CRUD (epic_*): full_path* for all.
- epic_list: state, search, author_username, label_name, confidential, sort, first, after, include_ancestors, include_descendants
- epic_get: epic_iid*
- epic_get_links: epic_iid* [REST]
- epic_create: title*, description, confidential, color, start_date, due_date, assignee_ids, label_ids, weight, health_status
- epic_update: epic_iid*, title, description, state_event, color, start_date, due_date, add_label_ids, remove_label_ids, assignee_ids, weight, health_status, status
- epic_delete: epic_iid*

Epic issues (epic_issue_*): full_path*, epic_iid* for all. GraphQL pagination: first, after, last, before.
- epic_issue_list
- epic_issue_assign / epic_issue_remove: child_project_path*, child_iid*
- epic_issue_update: child_id*, adjacent_id*, relative_position* (BEFORE/AFTER)

Epic notes (epic_note_*): full_path*, epic_iid* for all. GraphQL pagination: first, after, last, before.
- epic_note_list / epic_note_get (+ note_id*) / epic_note_delete (+ note_id*)
- epic_note_create: body*
- epic_note_update: note_id*, body*

Epic boards [Deprecated]:
- epic_board_list: group_id*
- epic_board_get: group_id*, board_id*

Group Wikis (Premium+):
- wiki_list: group_id*, with_content
- wiki_get: group_id*, slug*, render_html, version
- wiki_create: group_id*, title*, content*, format (markdown/rdoc/asciidoc/org)
- wiki_edit: group_id*, slug*, title, content, format
- wiki_delete: group_id*, slug*

Protected Branches (Premium+):
- protected_branch_list: group_id*, search
- protected_branch_get / protected_branch_unprotect: group_id*, branch*
- protected_branch_protect: group_id*, name*, push_access_level, merge_access_level, unprotect_access_level, allow_force_push, code_owner_approval_required, allowed_to_push/merge/unprotect
- protected_branch_update: group_id*, branch*, name, allow_force_push, code_owner_approval_required, allowed_to_push/merge/unprotect

Protected Environments (Premium+):
- protected_env_list: group_id*
- protected_env_get / protected_env_unprotect: group_id*, environment*
- protected_env_protect: group_id*, name*, deploy_access_levels, required_approval_count, approval_rules
- protected_env_update: group_id*, environment*, name, deploy_access_levels, required_approval_count, approval_rules

LDAP Links (Premium+):
- ldap_link_list: group_id*
- ldap_link_add: group_id*, cn*, group_access* (int), provider*
- ldap_link_delete: group_id*, cn, filter, provider
- ldap_link_delete_for_provider: group_id*, provider*, cn*

SAML Links (Premium+):
- saml_link_list: group_id*
- saml_link_get / saml_link_delete: group_id*, saml_group_name*
- saml_link_add: group_id*, saml_group_name*, access_level* (int)

Service Accounts (Premium+):
- service_account_list: group_id*, order_by, sort
- service_account_create: group_id*, name*, username*
- service_account_update: group_id*, service_account_id*, name, username
- service_account_delete: group_id*, service_account_id*, hard_delete
- service_account_pat_list: group_id*, service_account_id*
- service_account_pat_create: group_id*, service_account_id*, name*, scopes* (array), expires_at (YYYY-MM-DD)
- service_account_pat_revoke: group_id*, service_account_id*, token_id*

Analytics (Premium+):
- analytics_issues_count / analytics_mr_count / analytics_members_count: group_path* (URL-encoded)

Credentials (Ultimate):
- credential_list_pats: group_id*, search, state (active/inactive), revoked
- credential_list_ssh_keys: group_id*
- credential_revoke_pat: group_id*, token_id*
- credential_delete_ssh_key: group_id*, key_id*

SSH Certificates (Premium+):
- ssh_cert_list: group_id*
- ssh_cert_create: group_id*, key*, title*
- ssh_cert_delete: group_id*, certificate_id*

Security Settings (Ultimate):
- security_settings_update: group_id*, secret_push_protection_enabled*, projects_to_exclude (array)
`
	}

	desc += `

See also: gitlab_project (project-level), gitlab_user (user management), gitlab_search (cross-project search), gitlab_merge_request (MR workflows)`

	addMetaTool(server, "gitlab_group", desc, routes, toolutil.IconGroup)
}

// registerUserMeta registers the gitlab_user meta-tool with user,
// SSH key, email, event, notification, key, GPG key, impersonation token, and task-list management actions.
func registerUserMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := actionMap{
		"current":                     routeAction(client, users.Current),
		"list":                        routeAction(client, users.List),
		"get":                         routeAction(client, users.Get),
		"get_status":                  routeAction(client, users.GetStatus),
		"set_status":                  routeAction(client, users.SetStatus),
		"ssh_keys":                    routeAction(client, users.ListSSHKeys),
		"emails":                      routeAction(client, users.ListEmails),
		"contribution_events":         routeAction(client, users.ListContributionEvents),
		"associations_count":          routeAction(client, users.GetAssociationsCount),
		"todo_list":                   routeAction(client, todos.List),
		"todo_mark_done":              routeAction(client, todos.MarkDone),
		"todo_mark_all_done":          routeAction(client, todos.MarkAllDone),
		"event_list_project":          routeAction(client, events.ListProjectEvents),
		"event_list_contributions":    routeAction(client, events.ListCurrentUserContributionEvents),
		"notification_global_get":     routeAction(client, notifications.GetGlobalSettings),
		"notification_project_get":    routeAction(client, notifications.GetSettingsForProject),
		"notification_group_get":      routeAction(client, notifications.GetSettingsForGroup),
		"notification_global_update":  routeAction(client, notifications.UpdateGlobalSettings),
		"notification_project_update": routeAction(client, notifications.UpdateSettingsForProject),
		"notification_group_update":   routeAction(client, notifications.UpdateSettingsForGroup),
		"key_get_with_user":           routeAction(client, keys.GetKeyWithUser),
		"key_get_by_fingerprint":      routeAction(client, keys.GetKeyByFingerprint),
		"namespace_list":              routeAction(client, namespaces.List),
		"namespace_get":               routeAction(client, namespaces.Get),
		"namespace_exists":            routeAction(client, namespaces.Exists),
		"namespace_search":            routeAction(client, namespaces.Search),
		"avatar_get":                  routeAction(client, avatar.Get),
		"me":                          routeAction(client, users.Current),
		// Extended user admin actions
		"block":              destructiveAction(client, users.BlockUser),
		"unblock":            routeAction(client, users.UnblockUser),
		"ban":                destructiveAction(client, users.BanUser),
		"unban":              routeAction(client, users.UnbanUser),
		"activate":           routeAction(client, users.ActivateUser),
		"deactivate":         destructiveAction(client, users.DeactivateUser),
		"approve":            routeAction(client, users.ApproveUser),
		"reject":             destructiveAction(client, users.RejectUser),
		"disable_two_factor": destructiveAction(client, users.DisableTwoFactor),
		// User CRUD
		"create": routeAction(client, users.Create),
		"modify": routeAction(client, users.Modify),
		"delete": destructiveAction(client, users.Delete),
		// Extended SSH keys
		"ssh_keys_for_user":       routeAction(client, users.ListSSHKeysForUser),
		"get_ssh_key":             routeAction(client, users.GetSSHKey),
		"get_ssh_key_for_user":    routeAction(client, users.GetSSHKeyForUser),
		"add_ssh_key":             routeAction(client, users.AddSSHKey),
		"add_ssh_key_for_user":    routeAction(client, users.AddSSHKeyForUser),
		"delete_ssh_key":          destructiveAction(client, users.DeleteSSHKey),
		"delete_ssh_key_for_user": destructiveAction(client, users.DeleteSSHKeyForUser),
		// Misc user tools
		"current_user_status": routeAction(client, users.CurrentUserStatus),
		"activities":          routeAction(client, users.GetUserActivities),
		"memberships":         routeAction(client, users.GetUserMemberships),
		"create_runner":       routeAction(client, users.CreateUserRunner),
		"delete_identity":     destructiveAction(client, users.DeleteUserIdentity),
		// GPG keys
		"gpg_keys":                routeAction(client, usergpgkeys.List),
		"gpg_keys_for_user":       routeAction(client, usergpgkeys.ListForUser),
		"get_gpg_key":             routeAction(client, usergpgkeys.Get),
		"get_gpg_key_for_user":    routeAction(client, usergpgkeys.GetForUser),
		"add_gpg_key":             routeAction(client, usergpgkeys.Add),
		"add_gpg_key_for_user":    routeAction(client, usergpgkeys.AddForUser),
		"delete_gpg_key":          destructiveAction(client, usergpgkeys.Delete),
		"delete_gpg_key_for_user": destructiveAction(client, usergpgkeys.DeleteForUser),
		// Emails (extended)
		"emails_for_user":       routeAction(client, useremails.ListForUser),
		"get_email":             routeAction(client, useremails.Get),
		"add_email":             routeAction(client, useremails.Add),
		"add_email_for_user":    routeAction(client, useremails.AddForUser),
		"delete_email":          destructiveAction(client, useremails.Delete),
		"delete_email_for_user": destructiveAction(client, useremails.DeleteForUser),
		// Impersonation tokens
		"list_impersonation_tokens":    routeAction(client, impersonationtokens.List),
		"get_impersonation_token":      routeAction(client, impersonationtokens.Get),
		"create_impersonation_token":   routeAction(client, impersonationtokens.Create),
		"revoke_impersonation_token":   destructiveAction(client, impersonationtokens.Revoke),
		"create_personal_access_token": routeAction(client, impersonationtokens.CreatePAT),
		// Current user PAT (CE-compatible)
		"create_current_user_pat": routeAction(client, users.CreateCurrentUserPAT),
	}

	// Service accounts (EE-only — returns 404 on CE)
	if enterprise {
		routes["create_service_account"] = routeAction(client, users.CreateServiceAccount)
		routes["list_service_accounts"] = routeAction(client, users.ListServiceAccounts)
	}

	desc := `User management for GitLab: full user account CRUD plus SSH/GPG keys, emails, personal access tokens (PATs), impersonation tokens, user status, todos, contribution events, notification settings, namespaces, and avatars. This is the canonical user management tool — covers the entire /users API surface. Delete / block / ban / reject actions are destructive.
When to use: any user-management workflow — user CRUD (create / modify / delete / block / unblock / ban / unban / approve / reject / activate / deactivate), SSH/GPG key management, personal access token (PAT) management, impersonation tokens (admin), todos, contribution events, notification settings, namespaces, avatars, current-user status. NOT for: deploy tokens or project/group access tokens (use gitlab_access), instance-wide admin operations (use gitlab_admin), project/group memberships (use gitlab_project / gitlab_group).

Param conventions: * = required. User IDs are integers. List actions accept page, per_page. Actions ending in _for_user take the same params as the base action plus user_id*. Plain ssh_keys / gpg_keys / emails (no suffix) operate on the current authenticated user with no params.

Current user:
- current: returns authenticated user info. The legacy alias me is accepted and normalized to current, but current is the canonical action to emit.
- current_user_status: returns emoji, message, availability.
- set_status: emoji, message, availability (not_set/busy), clear_status_after (30_minutes/3_hours/8_hours/1_day/3_days/7_days/30_days)

User CRUD (admin):
- list: search, username, active, blocked, external, order_by, sort
- get: user_id*
- get_status: user_id*
- create: email*, name*, username*, password, reset_password, force_random_password, skip_confirmation, admin, external, bio, location, job_title, organization, projects_limit, note
- modify: user_id*, email, name, username, bio, location, job_title, organization, projects_limit, admin, external, note
- delete: user_id*
- associations_count: user_id*

User state (admin):
- block / unblock / ban / unban / activate / deactivate / approve / reject / disable_two_factor: user_id*

SSH keys:
- get_ssh_key: key_id*
- get_ssh_key_for_user: user_id*, key_id*
- add_ssh_key: title*, key*, expires_at, usage_type (auth/signing)
- delete_ssh_key: key_id*
- delete_ssh_key_for_user: user_id*, key_id*

GPG keys:
- get_gpg_key: key_id*
- get_gpg_key_for_user: user_id*, key_id*
- add_gpg_key: key* (armored GPG public key)
- delete_gpg_key: key_id*
- delete_gpg_key_for_user: user_id*, key_id*

Emails:
- get_email: email_id*
- add_email: email*, skip_confirmation
- add_email_for_user: user_id*, email*, skip_confirmation
- delete_email: email_id*
- delete_email_for_user: user_id*, email_id*

Tokens:
- list_impersonation_tokens: user_id*, state (active/inactive)
- get_impersonation_token: user_id*, token_id*
- create_impersonation_token: user_id*, name*, scopes*, expires_at
- revoke_impersonation_token: user_id*, token_id*
- create_personal_access_token: user_id*, name*, scopes*, description, expires_at
- create_current_user_pat: name*, scopes*, description, expires_at

Activity and events:
- activities: (admin) from (YYYY-MM-DD)
- memberships: user_id*, type (Project/Namespace)
- contribution_events: user_id*, action, target_type, before, after, sort
- event_list_project: project_id*, action, target_type, before, after, sort
- event_list_contributions: action, target_type, before, after, sort

Todos:
- todo_list: action, author_id, project_id, group_id, state (pending/done), type
- todo_mark_done: id*
- todo_mark_all_done: no params

Notifications:
- notification_global_get / notification_global_update: no ID needed. Update params: level, notification_email, event booleans
- notification_project_get / notification_project_update: project_id*. Update params: level, event booleans
- notification_group_get / notification_group_update: group_id*. Update params: level, event booleans

Keys and namespaces:
- key_get_with_user: key_id*. Returns SSH key with user info.
- key_get_by_fingerprint: fingerprint*
- namespace_list: search, owned_only
- namespace_get: namespace_id*
- namespace_exists: namespace*, parent_id
- namespace_search: search*
- avatar_get: email*, size

Misc:
- create_runner: runner_type*, group_id, project_id, description, paused, locked, run_untagged, tag_list, access_level, maximum_timeout, maintenance_note
- delete_identity: user_id*, provider*`

	if enterprise {
		desc += `

Service Accounts (Premium+ — GITLAB_ENTERPRISE=true):
- create_service_account: name, username, email
- list_service_accounts: order_by, sort`
	}

	desc += `

See also: gitlab_access (deploy/access tokens), gitlab_admin (instance administration)`

	addMetaTool(server, "gitlab_user", desc, routes, toolutil.IconUser)
}

// registerAdminMeta registers the gitlab_admin meta-tool with actions:
// topic_list, topic_get, topic_create, topic_update, topic_delete,
// settings_get, settings_update, appearance_get, appearance_update.
func registerAdminMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"topic_list":                     routeAction(client, topics.List),
		"topic_get":                      routeAction(client, topics.Get),
		"topic_create":                   routeAction(client, topics.Create),
		"topic_update":                   routeAction(client, topics.Update),
		"topic_delete":                   destructiveVoidAction(client, topics.Delete),
		"settings_get":                   routeAction(client, settings.Get),
		"settings_update":                routeAction(client, settings.Update),
		"appearance_get":                 routeAction(client, appearance.Get),
		"appearance_update":              routeAction(client, appearance.Update),
		"broadcast_message_list":         routeAction(client, broadcastmessages.List),
		"broadcast_message_get":          routeAction(client, broadcastmessages.Get),
		"broadcast_message_create":       routeAction(client, broadcastmessages.Create),
		"broadcast_message_update":       routeAction(client, broadcastmessages.Update),
		"broadcast_message_delete":       destructiveVoidAction(client, broadcastmessages.Delete),
		"feature_list":                   routeAction(client, features.List),
		"feature_list_definitions":       routeAction(client, features.ListDefinitions),
		"feature_set":                    features.SetRoute(client),
		"feature_delete":                 destructiveVoidAction(client, features.Delete),
		"license_get":                    routeAction(client, license.Get),
		"license_add":                    routeAction(client, license.Add),
		"license_delete":                 destructiveVoidAction(client, license.Delete),
		"system_hook_list":               routeAction(client, systemhooks.List),
		"system_hook_get":                routeAction(client, systemhooks.Get),
		"system_hook_add":                routeAction(client, systemhooks.Add),
		"system_hook_test":               routeAction(client, systemhooks.Test),
		"system_hook_delete":             destructiveVoidAction(client, systemhooks.Delete),
		"sidekiq_queue_metrics":          routeAction(client, sidekiq.GetQueueMetrics),
		"sidekiq_process_metrics":        routeAction(client, sidekiq.GetProcessMetrics),
		"sidekiq_job_stats":              routeAction(client, sidekiq.GetJobStats),
		"sidekiq_compound_metrics":       routeAction(client, sidekiq.GetCompoundMetrics),
		"plan_limits_get":                routeAction(client, planlimits.Get),
		"plan_limits_change":             routeAction(client, planlimits.Change),
		"usage_data_service_ping":        routeAction(client, usagedata.GetServicePing),
		"usage_data_non_sql_metrics":     routeAction(client, usagedata.GetNonSQLMetrics),
		"usage_data_queries":             routeAction(client, usagedata.GetQueries),
		"usage_data_metric_definitions":  routeAction(client, usagedata.GetMetricDefinitions),
		"usage_data_track_event":         routeAction(client, usagedata.TrackEvent),
		"usage_data_track_events":        routeAction(client, usagedata.TrackEvents),
		"db_migration_mark":              destructiveAction(client, dbmigrations.Mark),
		"application_list":               routeAction(client, applications.List),
		"application_create":             routeAction(client, applications.Create),
		"application_delete":             destructiveVoidAction(client, applications.Delete),
		"app_statistics_get":             routeAction(client, appstatistics.Get),
		"metadata_get":                   routeAction(client, metadata.Get),
		"custom_attr_list":               routeAction(client, customattributes.List),
		"custom_attr_get":                routeAction(client, customattributes.Get),
		"custom_attr_set":                routeAction(client, customattributes.Set),
		"custom_attr_delete":             destructiveVoidAction(client, customattributes.Delete),
		"bulk_import_start":              routeAction(client, bulkimports.StartMigration),
		"bulk_import_list":               routeAction(client, bulkimports.List),
		"bulk_import_get":                routeAction(client, bulkimports.Get),
		"bulk_import_cancel":             routeAction(client, bulkimports.Cancel),
		"bulk_import_entity_list":        routeAction(client, bulkimports.ListEntities),
		"bulk_import_entity_get":         routeAction(client, bulkimports.GetEntity),
		"bulk_import_entity_failures":    routeAction(client, bulkimports.ListEntityFailures),
		"error_tracking_list":            routeAction(client, errortracking.ListClientKeys),
		"error_tracking_create":          routeAction(client, errortracking.CreateClientKey),
		"error_tracking_delete":          destructiveVoidAction(client, errortracking.DeleteClientKey),
		"error_tracking_get_settings":    routeAction(client, errortracking.GetSettings),
		"error_tracking_update_settings": routeAction(client, errortracking.EnableDisable),
		"alert_metric_image_list":        routeAction(client, alertmanagement.ListMetricImages),
		"alert_metric_image_upload":      routeAction(client, alertmanagement.UploadMetricImage),
		"alert_metric_image_update":      routeAction(client, alertmanagement.UpdateMetricImage),
		"alert_metric_image_delete":      destructiveVoidAction(client, alertmanagement.DeleteMetricImage),
		"secure_file_list":               routeAction(client, securefiles.List),
		"secure_file_get":                routeAction(client, securefiles.Show),
		"secure_file_create":             routeAction(client, securefiles.Create),
		"secure_file_delete":             destructiveVoidAction(client, securefiles.Remove),
		"terraform_state_list":           routeAction(client, terraformstates.List),
		"terraform_state_get":            routeAction(client, terraformstates.Get),
		"terraform_state_delete":         destructiveVoidAction(client, terraformstates.Delete),
		"terraform_state_lock":           routeAction(client, terraformstates.Lock),
		"terraform_state_unlock":         destructiveAction(client, terraformstates.Unlock),
		"terraform_version_delete":       destructiveVoidAction(client, terraformstates.DeleteVersion),
		"cluster_agent_list":             routeAction(client, clusteragents.ListAgents),
		"cluster_agent_get":              routeAction(client, clusteragents.GetAgent),
		"cluster_agent_register":         routeAction(client, clusteragents.RegisterAgent),
		"cluster_agent_delete":           destructiveVoidAction(client, clusteragents.DeleteAgent),
		"cluster_agent_token_list":       routeAction(client, clusteragents.ListAgentTokens),
		"cluster_agent_token_get":        routeAction(client, clusteragents.GetAgentToken),
		"cluster_agent_token_create":     routeAction(client, clusteragents.CreateAgentToken),
		"cluster_agent_token_revoke":     destructiveVoidAction(client, clusteragents.RevokeAgentToken),
		"dependency_proxy_delete":        destructiveVoidAction(client, dependencyproxy.Purge),
		"import_github":                  routeAction(client, importservice.ImportFromGitHub),
		"import_bitbucket":               routeAction(client, importservice.ImportFromBitbucketCloud),
		"import_bitbucket_server":        routeAction(client, importservice.ImportFromBitbucketServer),
		"import_cancel_github":           routeAction(client, importservice.CancelGitHubImport),
		"import_gists":                   routeVoidAction(client, importservice.ImportGists),
	}

	addMetaTool(server, "gitlab_admin", `GitLab self-managed instance administration: settings, license, broadcast messages, system hooks, Sidekiq monitoring, plan limits, OAuth applications, secure files, Terraform states, cluster agents, dependency proxy cache, plus bulk imports (GitLab→GitLab migrations) and external imports (GitHub/Bitbucket). Most actions require admin privileges. Delete/purge/revoke actions are destructive.
When to use: instance-level admin tasks on a self-managed GitLab (settings, license, features, system hooks, Sidekiq monitoring, bulk imports between GitLab instances, external imports from GitHub/Bitbucket).
NOT for: user CRUD (use gitlab_user), group/project administration (use gitlab_group / gitlab_project), MCP server itself (use gitlab_server), runtime feature flags per project (use gitlab_feature_flags), CI variables (use gitlab_ci_variable).

Behavior:
- Idempotent reads: settings_get / appearance_get / *_list / *_get / sidekiq_* / app_statistics_get / metadata_get / usage_data_service_ping / usage_data_non_sql_metrics / usage_data_queries / usage_data_metric_definitions / plan_limits_get / feature_list / feature_list_definitions.
- settings_update / appearance_update / feature_set / plan_limits_change / custom_attr_set / error_tracking_update_settings are idempotent (same input → same state). license_add / system_hook_add / system_hook_test / broadcast_message_create / application_create / bulk_import_start / import_github / import_bitbucket / import_bitbucket_server / import_gists are NON-idempotent (re-invocation creates duplicates or new background jobs).
- Side effects: license_add / system_hook_add / broadcast_message_create / settings_update / feature_set apply instance-wide IMMEDIATELY (all sessions affected); bulk_import_* and import_* queue long-running async migrations — poll bulk_import_get / bulk_import_entity_* until status='finished'; usage_data_track_event posts to Snowplow when send_to_snowplow=true; application_create returns the OAuth secret only ONCE.
- Destructive: *_delete, license_delete, system_hook_delete, feature_delete, application_delete, broadcast_message_delete, custom_attr_delete, cluster_agent_delete, dependency_proxy_delete, secure_file_delete, terraform_state_delete / terraform_state_unlock, db_migration_mark, bulk_import_cancel and import_cancel_github are irreversible. db_migration_mark may corrupt the schema if used incorrectly.

Returns: resource object for *_get/*_create/*_update/*_set/*_add; metrics object for Sidekiq/usage_data/app_statistics/metadata; paginated array for *_list / feature_list_definitions; {success, message} for *_delete/*_revoke/*_purge/*_unlock.
Errors: 401/403 forbidden (hint: most actions require admin token), 404 not found, 400 invalid params (hint: license must be base64-encoded; system hook url must be https).

Param conventions: * = required. List actions accept page, per_page.

Topics:
- topic_list: search
- topic_get / topic_delete: topic_id*
- topic_create: name*, title, description
- topic_update: topic_id*, name, title, description

Settings & appearance:
- settings_get / appearance_get: (no params). If the task says "read current instance settings" or "get instance settings", call settings_get, not broadcast_message_list.
- settings_update: settings (map of setting_name to value)
- appearance_update: title, description, header_message, footer_message, message_background_color, message_font_color, email_header_and_footer_enabled, pwa_name, pwa_short_name, pwa_description, member_guidelines, new_project_guidelines, profile_image_guidelines

Broadcast messages:
- broadcast_message_list: (no params) lists existing broadcast messages only; it does not read instance settings.
- broadcast_message_get / broadcast_message_delete: id*
- broadcast_message_create: message*, starts_at, ends_at, broadcast_type, theme, dismissable (bool), target_path, target_access_levels
- broadcast_message_update: id*, message, starts_at, ends_at, broadcast_type, theme, dismissable

Instance feature flags:
- feature_list / feature_list_definitions: (no params)
- feature_set: name*, value*, key, feature_group, user, group, namespace, project, repository, force (bool)
- feature_delete: name*

License:
- license_get: (no params)
- license_add: license* (Base64-encoded)
- license_delete: id*

System hooks:
- system_hook_list: (no params)
- system_hook_get / system_hook_test / system_hook_delete: id*
- system_hook_add: url*, token, push_events, tag_push_events, merge_requests_events, repository_update_events, enable_ssl_verification

Sidekiq metrics: sidekiq_queue_metrics / sidekiq_process_metrics / sidekiq_job_stats / sidekiq_compound_metrics (no params).

Plan limits:
- plan_limits_get: plan_name
- plan_limits_change: plan_name*, conan_max_file_size, generic_packages_max_file_size, helm_max_file_size, maven_max_file_size, npm_max_file_size, nuget_max_file_size, pypi_max_file_size, terraform_module_max_file_size

Usage data:
- usage_data_service_ping / usage_data_non_sql_metrics / usage_data_queries / usage_data_metric_definitions: (no params)
- usage_data_track_event: event*, send_to_snowplow (bool), namespace_id, project_id
- usage_data_track_events: events* (array)

OAuth applications:
- application_list: (no params)
- application_create: name*, redirect_uri*, scopes*, confidential (bool)
- application_delete: id*

Misc:
- db_migration_mark: version*, database
- app_statistics_get / metadata_get: (no params)

Custom attributes:
- custom_attr_list: resource_type* (user/group/project), resource_id*
- custom_attr_get / custom_attr_delete: resource_type*, resource_id*, key*
- custom_attr_set: resource_type*, resource_id*, key*, value*

Bulk import:
- bulk_import_start: url*, access_token*, entities* (array of {source_type, source_full_path, destination_slug, destination_namespace, migrate_projects (bool), migrate_memberships (bool)})
- bulk_import_list: status, page, per_page
- bulk_import_get: id*
- bulk_import_cancel: id*
- bulk_import_entity_list: bulk_import_id, status, page, per_page
- bulk_import_entity_get: bulk_import_id*, entity_id*
- bulk_import_entity_failures: bulk_import_id*, entity_id*

Error tracking:
- error_tracking_list: project_id*
- error_tracking_create: project_id*
- error_tracking_delete: project_id*, key_id*
- error_tracking_get_settings: project_id*
- error_tracking_update_settings: project_id*, active (bool), integrated (bool)

Alert metric images:
- alert_metric_image_list: project_id*, alert_iid*
- alert_metric_image_upload: project_id*, alert_iid*, url*, url_text
- alert_metric_image_update: project_id*, alert_iid*, image_id*, url, url_text
- alert_metric_image_delete: project_id*, alert_iid*, image_id*

Secure files:
- secure_file_list: project_id*
- secure_file_get / secure_file_delete: project_id*, file_id*
- secure_file_create: project_id*, name*, content* (base64-encoded)

Terraform states:
- terraform_state_list: project_path*
- terraform_state_get: project_path*, name*
- terraform_state_delete / terraform_state_lock / terraform_state_unlock: project_id*, name*
- terraform_version_delete: project_id*, name*, serial*

Cluster agents:
- cluster_agent_list: project_id*
- cluster_agent_get / cluster_agent_delete: project_id*, agent_id*
- cluster_agent_register: project_id*, name*
- cluster_agent_token_list: project_id*, agent_id*
- cluster_agent_token_get / cluster_agent_token_revoke: project_id*, agent_id*, token_id*
- cluster_agent_token_create: project_id*, agent_id*, name*

Imports:
- import_github: personal_access_token*, repo_id*, target_namespace*, new_name
- import_bitbucket: bitbucket_username*, bitbucket_app_password*, repo_path*, target_namespace*, new_name
- import_bitbucket_server: bitbucket_server_url*, bitbucket_server_username*, personal_access_token*, bitbucket_server_project*, bitbucket_server_repo*, new_namespace, new_name
- import_cancel_github: project_id*
- import_gists: personal_access_token*
- dependency_proxy_delete: group_id* — purges the group's dependency proxy cache

Parameter constraints (beyond schema):
- broadcast_message_create.broadcast_type ∈ {banner, notification}; theme is a CSS hex color (e.g. '#E75E40'); target_access_levels uses GitLab numeric levels [10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner]; starts_at/ends_at are ISO 8601 timestamps and ends_at MUST be > starts_at.
- feature_set.value accepts 'true' / 'false' / a 0–100 integer (percentage of time/actors) / 'actor:<id>'; the optional key disambiguates 'percentage_of_time' vs 'percentage_of_actors'; user/group/namespace/project/repository scope the gate and are mutually-exclusive with each other.
- plan_limits_change.*_max_file_size are sizes in BYTES; 0 disables the limit. Omitted fields keep their current value (partial update).
- license_add.license is the Base64 of the raw .gitlab-license file (not the file path).
- system_hook_add.url MUST be https when enable_ssl_verification=true; token is sent as X-Gitlab-Token on every delivery.
- application_create.scopes is a SPACE-separated string of OAuth scopes (e.g. 'api read_user'); confidential=false enables PKCE for public clients. The client_secret is returned ONCE on creation and cannot be retrieved later.
- cluster_agent_token_create returns the token ONCE; revoke + re-create to rotate.
- secure_file_create.content is Base64-encoded; max size 5 MiB.
- custom_attr_set.resource_type ∈ {user, group, project} and (resource_type, resource_id, key) is a unique upsert key.
- bulk_import_start.entities[].source_type ∈ {group_entity, project_entity}; migrate_projects and migrate_memberships apply only to group_entity. destination_namespace must already exist on the target instance.
- import_bitbucket_server.bitbucket_server_project is the project KEY (usually uppercase, from the Bitbucket URL), not the display name.
- usage_data_track_event: namespace_id and project_id are mutually-exclusive context refs (provide at most one); send_to_snowplow=false keeps the event internal to GitLab.
- db_migration_mark.database ∈ {main, ci}; defaults to 'main'. Marking a non-applied migration corrupts schema_migrations — verify first via metadata_get.
- terraform_state_lock fails if the state is already locked; unlock breaks any active client session holding the lock.
- topic_create.name must be globally unique (slug); title is the display name shown in the UI.
- List actions: page defaults to 1, per_page defaults to 20 (GitLab cap is 100).

See also: gitlab_user (user CRUD), gitlab_server (MCP server health and updates), gitlab_group / gitlab_project (group/project admin), gitlab_access (tokens, deploy keys, access requests).`, routes, toolutil.IconServer)
}

// registerAccessMeta registers the gitlab_access meta-tool with actions:
// token_project_list, token_project_get, token_project_create, token_project_rotate,
// token_project_rotate_self, token_project_revoke, token_group_list, token_group_get,
// token_group_create, token_group_rotate, token_group_rotate_self, token_group_revoke,
// token_personal_list, token_personal_get, token_personal_rotate, token_personal_rotate_self,
// token_personal_revoke, token_personal_revoke_self,
// deploy_token_list_all, deploy_token_list_project, deploy_token_list_group,
// deploy_token_get_project, deploy_token_get_group, deploy_token_create_project,
// deploy_token_create_group, deploy_token_delete_project, deploy_token_delete_group,
// deploy_key_list_project, deploy_key_get, deploy_key_add, deploy_key_update, deploy_key_delete,
// deploy_key_enable, deploy_key_list_all, deploy_key_add_instance, deploy_key_list_user_project,
// request_list_project, request_list_group, request_project, request_group,
// approve_project, approve_group, deny_project, deny_group,
// invite_list_project, invite_list_group, invite_project, and invite_group.
func registerAccessMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"token_project_list":           routeAction(client, accesstokens.ProjectList),
		"token_project_get":            routeAction(client, accesstokens.ProjectGet),
		"token_project_create":         routeAction(client, accesstokens.ProjectCreate),
		"token_project_rotate":         routeAction(client, accesstokens.ProjectRotate),
		"token_project_rotate_self":    routeAction(client, accesstokens.ProjectRotateSelf),
		"token_project_revoke":         destructiveVoidAction(client, accesstokens.ProjectRevoke),
		"token_group_list":             routeAction(client, accesstokens.GroupList),
		"token_group_get":              routeAction(client, accesstokens.GroupGet),
		"token_group_create":           routeAction(client, accesstokens.GroupCreate),
		"token_group_rotate":           routeAction(client, accesstokens.GroupRotate),
		"token_group_rotate_self":      routeAction(client, accesstokens.GroupRotateSelf),
		"token_group_revoke":           destructiveVoidAction(client, accesstokens.GroupRevoke),
		"token_personal_list":          routeAction(client, accesstokens.PersonalList),
		"token_personal_get":           routeAction(client, accesstokens.PersonalGet),
		"token_personal_rotate":        routeAction(client, accesstokens.PersonalRotate),
		"token_personal_rotate_self":   routeAction(client, accesstokens.PersonalRotateSelf),
		"token_personal_revoke":        destructiveVoidAction(client, accesstokens.PersonalRevoke),
		"token_personal_revoke_self":   destructiveVoidAction(client, accesstokens.PersonalRevokeSelf),
		"deploy_token_list_all":        routeAction(client, deploytokens.ListAll),
		"deploy_token_list_project":    routeAction(client, deploytokens.ListProject),
		"deploy_token_list_group":      routeAction(client, deploytokens.ListGroup),
		"deploy_token_get_project":     routeAction(client, deploytokens.GetProject),
		"deploy_token_get_group":       routeAction(client, deploytokens.GetGroup),
		"deploy_token_create_project":  routeAction(client, deploytokens.CreateProject),
		"deploy_token_create_group":    routeAction(client, deploytokens.CreateGroup),
		"deploy_token_delete_project":  destructiveVoidAction(client, deploytokens.DeleteProject),
		"deploy_token_delete_group":    destructiveVoidAction(client, deploytokens.DeleteGroup),
		"deploy_key_list_project":      routeAction(client, deploykeys.ListProject),
		"deploy_key_get":               routeAction(client, deploykeys.Get),
		"deploy_key_add":               routeAction(client, deploykeys.Add),
		"deploy_key_update":            routeAction(client, deploykeys.Update),
		"deploy_key_delete":            destructiveVoidAction(client, deploykeys.Delete),
		"deploy_key_enable":            routeAction(client, deploykeys.Enable),
		"deploy_key_list_all":          routeAction(client, deploykeys.ListAll),
		"deploy_key_add_instance":      routeAction(client, deploykeys.AddInstance),
		"deploy_key_list_user_project": routeAction(client, deploykeys.ListUserProject),
		"request_list_project":         routeAction(client, accessrequests.ListProject),
		"request_list_group":           routeAction(client, accessrequests.ListGroup),
		"request_project":              routeAction(client, accessrequests.RequestProject),
		"request_group":                routeAction(client, accessrequests.RequestGroup),
		"approve_project":              routeAction(client, accessrequests.ApproveProject),
		"approve_group":                routeAction(client, accessrequests.ApproveGroup),
		"deny_project":                 destructiveVoidAction(client, accessrequests.DenyProject),
		"deny_group":                   destructiveVoidAction(client, accessrequests.DenyGroup),
		"invite_list_project":          routeAction(client, invites.ListPendingProjectInvitations),
		"invite_list_group":            routeAction(client, invites.ListPendingGroupInvitations),
		"invite_project":               routeAction(client, invites.ProjectInvites),
		"invite_group":                 routeAction(client, invites.GroupInvites),
	}
	addMetaTool(server, "gitlab_access", `Manage GitLab access credentials: access tokens (project/group/personal), deploy tokens, deploy keys, access requests, and invitations. Revoke/delete actions are destructive and irreversible.
When to use: provision and audit who/what can access a project or group; rotate (not revoke+create) to roll a token without invalidating CI configurations.
NOT for: SSH/GPG keys or impersonation tokens (use gitlab_user), PAT creation (use gitlab_user create_personal_access_token / create_current_user_pat — gitlab_access exposes token_personal_* for list/get/rotate/revoke only), instance admin operations (use gitlab_admin), project membership/permissions (use gitlab_project member_*), 2FA/MFA flows.

Returns:
- token_*_list / deploy_token_list_* / deploy_key_list_* / request_list_* / invite_list_*: arrays with pagination.
- token_*_get / token_*_create / token_*_rotate / deploy_token_get_* / deploy_token_create_* / deploy_key_get / deploy_key_add / deploy_key_update / deploy_key_enable / approve_* / request_*: token / key / request / invitation object. Create / rotate include the cleartext token only ONCE — store it securely; subsequent reads return only the metadata.
- token_*_revoke / deploy_token_delete_* / deploy_key_delete / deny_* : {success, message}.
Errors: 401/403 (hint: requires Maintainer+ to manage project tokens, Owner for group, admin for instance / deploy_token_list_all / deploy_key_list_all / deploy_key_add_instance), 404 (hint: token_id and deploy_key_id are scoped to the project/group), 400 (hint: scopes must be a subset of {api, read_api, read_repository, write_repository, read_registry, write_registry}; expires_at must be a future ISO date).

Param conventions: * = required. List actions accept page, per_page. Project/group token actions use project_id* or group_id*; personal token actions use user_id where documented, and self actions take no scope ID. Deploy token/key delete and token revoke are irreversible.

Access tokens (token_*) — project, group, and personal scopes. Rotate generates a new token and invalidates the old one:
- token_project_list / token_group_list: project_id* or group_id*
- token_project_get / token_group_get: project_id* or group_id*, token_id*
- token_project_create / token_group_create: project_id* or group_id*, name*, scopes*, expires_at, access_level
- token_project_rotate / token_group_rotate: project_id* or group_id*, token_id*, expires_at
- token_project_rotate_self / token_group_rotate_self: project_id* or group_id*, expires_at
- token_project_revoke / token_group_revoke: project_id* or group_id*, token_id*
- token_personal_list: user_id
- token_personal_get: token_id*
- token_personal_rotate: token_id*, expires_at
- token_personal_rotate_self: expires_at
- token_personal_revoke: token_id*
- token_personal_revoke_self: (no params)

Deploy tokens (deploy_token_*) — scoped to project or group, used for CI/CD registry access:
- deploy_token_list_all: (admin only)
- deploy_token_list_project / deploy_token_list_group: project_id* or group_id*
- deploy_token_get_project / deploy_token_get_group: project_id* or group_id*, deploy_token_id*
- deploy_token_create_project / deploy_token_create_group: project_id* or group_id*, name*, scopes*, expires_at
- deploy_token_delete_project / deploy_token_delete_group: project_id* or group_id*, deploy_token_id*

Deploy keys (deploy_key_*) — SSH keys for read/write repo access without a user account. For deploy-key wording, add/create maps to deploy_key_add, fetch/get maps to deploy_key_get, update maps to deploy_key_update, and delete/remove maps to deploy_key_delete.
- deploy_key_list_project / deploy_key_list_user_project: project_id*
- deploy_key_list_all: (admin only)
- deploy_key_get: project_id*, deploy_key_id*. If a workflow says add/create, then fetch/get, then update/delete, call deploy_key_get with the id returned by deploy_key_add before updating.
- deploy_key_add: project_id*, title*, key*, can_push
- deploy_key_update: project_id*, deploy_key_id*, title, can_push
- deploy_key_delete: project_id*, deploy_key_id*
- deploy_key_enable: project_id*, deploy_key_id*
- deploy_key_add_instance: title*, key*

Access requests (request_*, approve_*, deny_*):
- request_list_project / request_list_group: project_id* or group_id*
- request_project / request_group: project_id* or group_id*
- approve_project / approve_group: project_id* or group_id*, user_id*, access_level
- deny_project / deny_group: project_id* or group_id*, user_id*

Invitations (invite_*):
- invite_list_project / invite_list_group: project_id* or group_id*
- invite_project / invite_group: project_id* or group_id*, email*, access_level*, expires_at

See also: gitlab_user (SSH/GPG keys, user PATs), gitlab_admin (instance admin), gitlab_project (project settings)`, routes, toolutil.IconToken)
}

// registerMergeTrainMeta registers the gitlab_merge_train meta-tool with actions
// for listing, getting, and adding merge requests to merge trains.
func registerMergeTrainMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list_project": routeAction(client, mergetrains.ListProjectMergeTrains),
		"list_branch":  routeAction(client, mergetrains.ListMergeRequestInMergeTrain),
		"get":          routeAction(client, mergetrains.GetMergeRequestOnMergeTrain),
		"add":          routeAction(client, mergetrains.AddMergeRequestToMergeTrain),
	}
	addMetaTool(server, "gitlab_merge_train", `Manage GitLab merge trains (automated merge queues). List, get, and add MRs to merge trains.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

Param conventions: * = required. All actions need project_id*.

- list_project: project_id*, scope (active/complete), sort (asc/desc), page, per_page
- list_branch: project_id*, target_branch*, scope, sort, page, per_page
- get: project_id*, merge_request_iid*
- add: project_id*, merge_request_iid*, auto_merge (bool), sha, squash (bool)

See also: gitlab_merge_request, gitlab_pipeline`, routes, toolutil.IconMR)
}

// registerAuditEventMeta registers the gitlab_audit_event meta-tool with actions
// for listing and getting audit events at instance, group, and project levels.
func registerAuditEventMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list_instance": routeAction(client, auditevents.ListInstance),
		"get_instance":  routeAction(client, auditevents.GetInstance),
		"list_group":    routeAction(client, auditevents.ListGroup),
		"get_group":     routeAction(client, auditevents.GetGroup),
		"list_project":  routeAction(client, auditevents.ListProject),
		"get_project":   routeAction(client, auditevents.GetProject),
	}
	addReadOnlyMetaTool(server, "gitlab_audit_event", `List and get GitLab audit events at instance, group, and project levels for compliance tracking.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

Common optional params: created_after (YYYY-MM-DD), created_before, page, per_page.

- list_instance: (admin only) created_after, created_before
- get_instance: event_id*
- list_group: group_id*, created_after, created_before
- get_group: group_id*, event_id*
- list_project: project_id*, created_after, created_before
- get_project: project_id*, event_id*

See also: gitlab_admin`, routes, toolutil.IconEvent)
}

// registerDORAMetricsMeta registers the gitlab_dora_metrics meta-tool with actions
// for retrieving DORA metrics at project and group levels.
func registerDORAMetricsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"project": routeAction(client, dorametrics.GetProjectMetrics),
		"group":   routeAction(client, dorametrics.GetGroupMetrics),
	}
	addReadOnlyMetaTool(server, "gitlab_dora_metrics", `Get DORA metrics: deployment frequency, lead time, MTTR, change failure rate.
	Returns: JSON with metric data. Errors: 404 not found, 403 forbidden — with actionable hints.

Common params: metric* (deployment_frequency|lead_time_for_changes|time_to_restore_service|change_failure_rate), start_date (YYYY-MM-DD), end_date, interval (daily/monthly/all), environment_tiers (array).

- project: project_id*, metric*, start_date, end_date, interval, environment_tiers
- group: group_id*, metric*, start_date, end_date, interval, environment_tiers

See also: gitlab_environment, gitlab_pipeline`, routes, toolutil.IconAnalytics)
}

// registerDependencyMeta registers the gitlab_dependency meta-tool with actions
// for listing project dependencies and managing dependency list exports (SBOM).
func registerDependencyMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":            routeAction(client, dependencies.ListDeps),
		"export_create":   routeAction(client, dependencies.CreateExport),
		"export_get":      routeAction(client, dependencies.GetExport),
		"export_download": routeAction(client, dependencies.DownloadExport),
	}
	addMetaTool(server, "gitlab_dependency", `List project dependencies and create/download SBOM exports (CycloneDX).
Returns: JSON with resource data. The list action includes pagination (page, per_page, total, next_page). export_create/export_get return export metadata, and export_download returns the CycloneDX payload. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

- list: project_id*, package_manager, page, per_page
- export_create: pipeline_id*, export_type (default: sbom)
- export_get: export_id*
- export_download: export_id*. CycloneDX JSON, max 1MB.

See also: gitlab_project, gitlab_vulnerability`, routes, toolutil.IconPackage)
}

// registerExternalStatusCheckMeta registers the gitlab_external_status_check meta-tool with actions
// for managing external status checks on merge requests and projects.
func registerExternalStatusCheckMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list_project_checks":    routeAction(client, externalstatuschecks.ListProjectStatusChecks),
		"list_project_mr_checks": routeAction(client, externalstatuschecks.ListProjectMRExternalStatusChecks),
		"list_project":           routeAction(client, externalstatuschecks.ListProjectExternalStatusChecks),
		"create_project":         routeAction(client, externalstatuschecks.CreateProjectExternalStatusCheck),
		"delete_project":         destructiveVoidAction(client, externalstatuschecks.DeleteProjectExternalStatusCheck),
		"update_project":         routeAction(client, externalstatuschecks.UpdateProjectExternalStatusCheck),
		"retry_project":          routeVoidAction(client, externalstatuschecks.RetryFailedExternalStatusCheckForProjectMR),
		"set_project_mr_status":  routeVoidAction(client, externalstatuschecks.SetProjectMRExternalStatusCheckStatus),
	}
	addMetaTool(server, "gitlab_external_status_check", `Manage external status checks for MRs and projects. CRUD checks and set/retry status.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Param conventions: * = required.

- list_project_checks: project_id*, page, per_page
- list_project_mr_checks: project_id*, merge_request_iid*, page, per_page
- list_project: project_id*, page, per_page
- create_project: project_id*, name*, external_url*, shared_secret, protected_branch_ids
- delete_project: project_id*, check_id*
- update_project: project_id*, check_id*, name, external_url, shared_secret, protected_branch_ids
- retry_project: project_id*, merge_request_iid*, check_id*
- set_project_mr_status: project_id*, merge_request_iid*, sha*, external_status_check_id*, status*

See also: gitlab_merge_request`, routes, toolutil.IconSecurity)
}

// registerGroupSCIMMeta registers the gitlab_group_scim meta-tool with actions
// for managing SCIM identities in a group.
func registerGroupSCIMMeta(server *mcp.Server, client *gitlabclient.Client) {
	updateAction := func(ctx context.Context, client *gitlabclient.Client, input groupscim.UpdateInput) (groupscim.UpdateOutput, error) {
		if err := groupscim.Update(ctx, client, input); err != nil {
			return groupscim.UpdateOutput{}, err
		}
		return groupscim.UpdateOutput{Updated: true, Message: "SCIM identity updated successfully."}, nil
	}

	routes := actionMap{
		"list":   routeAction(client, groupscim.List),
		"get":    routeAction(client, groupscim.Get),
		"update": routeAction(client, updateAction),
		"delete": destructiveVoidAction(client, groupscim.Delete),
	}
	addMetaTool(server, "gitlab_group_scim", `Manage SCIM identities for GitLab group provisioning.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

All actions need group_id*.

- list: group_id*
- get / delete: group_id*, uid*
- update: group_id*, uid*, extern_uid*

See also: gitlab_group, gitlab_user`, routes, toolutil.IconSecurity)
}

// registerMemberRoleMeta registers the gitlab_member_role meta-tool with actions
// for managing custom member roles at instance and group levels.
func registerMemberRoleMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list_instance":   routeAction(client, memberroles.ListInstance),
		"create_instance": routeAction(client, memberroles.CreateInstance),
		"delete_instance": destructiveVoidAction(client, memberroles.DeleteInstance),
		"list_group":      routeAction(client, memberroles.ListGroup),
		"create_group":    routeAction(client, memberroles.CreateGroup),
		"delete_group":    destructiveVoidAction(client, memberroles.DeleteGroup),
	}
	addMetaTool(server, "gitlab_member_role", `Manage custom member roles at instance or group level. Fine-grained permissions beyond standard access levels.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Instance-level:
- list_instance: no params
- create_instance: name*, base_access_level* (10/20/30/40/50), description, permissions (object)
- delete_instance: member_role_id*

Group-level:
- list_group: group_id*
- create_group: group_id*, name*, base_access_level*, description, permissions
- delete_group: group_id*, member_role_id*

See also: gitlab_group, gitlab_user`, routes, toolutil.IconSecurity)
}

// registerEnterpriseUserMeta registers the gitlab_enterprise_user meta-tool with actions
// for listing, getting, disabling 2FA, and deleting enterprise users.
func registerEnterpriseUserMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":        routeAction(client, enterpriseusers.List),
		"get":         routeAction(client, enterpriseusers.Get),
		"disable_2fa": destructiveVoidAction(client, enterpriseusers.Disable2FA),
		"delete":      destructiveVoidAction(client, enterpriseusers.Delete),
	}
	addMetaTool(server, "gitlab_enterprise_user", `Manage enterprise users for a GitLab group: list, get, disable 2FA, delete.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

All actions need group_id*.

- list: group_id*, username, search, active, blocked, created_after, created_before, two_factor, page, per_page
- get: group_id*, user_id*
- disable_2fa: group_id*, user_id*
- delete: group_id*, user_id*, hard_delete

See also: gitlab_group, gitlab_user`, routes, toolutil.IconUser)
}

// registerAttestationMeta registers the gitlab_attestation meta-tool with actions
// for listing and downloading build attestations.
func registerAttestationMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":     routeAction(client, attestations.List),
		"download": routeAction(client, attestations.Download),
	}
	addReadOnlyMetaTool(server, "gitlab_attestation", `List and download build attestations (SLSA provenance) for project artifacts.
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

- list: project_id*, subject_digest*
- download: project_id*, attestation_iid*

See also: gitlab_pipeline, gitlab_package`, routes, toolutil.IconSecurity)
}

// registerCompliancePolicyMeta registers the gitlab_compliance_policy meta-tool with actions:
// get, update.
func registerCompliancePolicyMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"get":    routeAction(client, compliancepolicy.Get),
		"update": routeAction(client, compliancepolicy.Update),
	}
	addMetaTool(server, "gitlab_compliance_policy", `Get and update admin compliance policy settings (CSP namespace configuration).
Returns: JSON with resource data for get/update actions. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

- get: no params
- update: csp_namespace_id (int64)

See also: gitlab_admin`, routes, toolutil.IconSecurity)
}

// registerProjectAliasMeta registers the gitlab_project_alias meta-tool with actions:
// list, get, create, delete.
func registerProjectAliasMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":   routeAction(client, projectaliases.List),
		"get":    routeAction(client, projectaliases.Get),
		"create": routeAction(client, projectaliases.Create),
		"delete": destructiveVoidAction(client, projectaliases.Delete),
	}
	addMetaTool(server, "gitlab_project_alias", `CRUD project aliases: short names that redirect to projects (admin, Premium/Ultimate).
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

- list: no params
- get / delete: name*
- create: name*, project_id* (int64)

See also: gitlab_project`, routes, toolutil.IconProject)
}

// registerGeoMeta registers the gitlab_geo enterprise meta-tool that provides
// Geo replication site management (create, list, get, edit, delete, repair, status).
func registerGeoMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":      routeAction(client, geo.Create),
		"list":        routeAction(client, geo.List),
		"get":         routeAction(client, geo.Get),
		"edit":        routeAction(client, geo.Edit),
		"delete":      destructiveVoidAction(client, geo.Delete),
		"repair":      routeAction(client, geo.Repair),
		"list_status": routeAction(client, geo.ListStatus),
		"get_status":  routeAction(client, geo.GetStatus),
	}
	addMetaTool(server, "gitlab_geo", `Manage Geo replication sites: CRUD, repair OAuth, and check replication status (admin, Premium/Ultimate).
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Param conventions: * = required.

- list / list_status: page, per_page
- get / get_status / delete / repair: id*
- create: name, url, primary, enabled, internal_url, files_max_capacity, repos_max_capacity, verification_max_capacity, container_repositories_max_capacity, sync_object_storage, selective_sync_type, selective_sync_shards, selective_sync_namespace_ids, minimum_reverification_interval
- edit: id*, plus create params (except primary, sync_object_storage)

See also: gitlab_admin`, routes, toolutil.IconServer)
}

// registerModelRegistryMeta registers the gitlab_model_registry enterprise meta-tool
// that provides ML model registry operations (download model package files).
func registerModelRegistryMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"download": routeAction(client, modelregistry.Download),
	}
	addReadOnlyMetaTool(server, "gitlab_model_registry", `Download ML model package files from the GitLab Model Registry. Read-only — cannot publish or delete model versions through this tool. The underlying GitLab API requires a Premium/Ultimate plan on the target instance (server enforces it with 403); the tool itself is always registered and is not gated by GITLAB_ENTERPRISE.
When to use: pull a model artifact (.pkl, .onnx, .safetensors, .bin, .gguf, etc.) attached to a registered model version, e.g. for inference, evaluation or vendoring into a build pipeline.
NOT for: generic packages (use gitlab_package), container images (use gitlab_package registry_*), release attachments (use gitlab_release link_*), training jobs or experiment tracking, model publishing or versioning (not yet exposed through MCP).

Returns:
- download: {file_name, model_version_id, size, content_base64} — binary content is base64-encoded; large models can produce very large responses.
Errors: 404 (hint: project_id, model_version_id and path are model-registry-scoped; verify in the GitLab UI under Deploy → Model registry), 403 (hint: requires Reporter+ on the project and a Premium/Ultimate plan), 400 (hint: filename must match an asset attached to the version).

- download: project_id*, model_version_id*, path*, filename*. Returns base64-encoded file content.
  - project_id (string | int, required) — numeric ID or URL-encoded full path of the project that owns the registered model.
  - model_version_id (int, required) — registered model version ID; visible in the GitLab UI under Deploy → Model registry → <model> → Versions.
  - path (string, required) — package-relative directory of the asset (use '/' for the package root, otherwise e.g. 'artifacts/' or 'weights/').
  - filename (string, required) — exact asset filename within the package, including extension (e.g. 'model.safetensors', 'config.json').
  - Any unrecognized parameter name is rejected by the meta-tool router (strict unknown-field validation); only the reserved meta key 'confirm' is stripped before unmarshalling.

See also: gitlab_package (generic / npm / maven / conan / pypi / nuget / container packages), gitlab_release (asset links per release), gitlab_repository (raw files in the repo).`, routes, toolutil.IconPackage)
}

// registerStorageMoveMeta registers the gitlab_storage_move enterprise meta-tool
// that provides repository storage move operations for projects, groups, and snippets.
func registerStorageMoveMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"retrieve_all_project":    routeAction(client, projectstoragemoves.RetrieveAll),
		"retrieve_project":        routeAction(client, projectstoragemoves.RetrieveForProject),
		"get_project":             routeAction(client, projectstoragemoves.Get),
		"get_project_for_project": routeAction(client, projectstoragemoves.GetForProject),
		"schedule_project":        routeAction(client, projectstoragemoves.Schedule),
		"schedule_all_project":    routeAction(client, projectstoragemoves.ScheduleAll),
		"retrieve_all_group":      routeAction(client, groupstoragemoves.RetrieveAll),
		"retrieve_group":          routeAction(client, groupstoragemoves.RetrieveForGroup),
		"get_group":               routeAction(client, groupstoragemoves.Get),
		"get_group_for_group":     routeAction(client, groupstoragemoves.GetForGroup),
		"schedule_group":          routeAction(client, groupstoragemoves.Schedule),
		"schedule_all_group":      routeAction(client, groupstoragemoves.ScheduleAll),
		"retrieve_all_snippet":    routeAction(client, snippetstoragemoves.RetrieveAll),
		"retrieve_snippet":        routeAction(client, snippetstoragemoves.RetrieveForSnippet),
		"get_snippet":             routeAction(client, snippetstoragemoves.Get),
		"get_snippet_for_snippet": routeAction(client, snippetstoragemoves.GetForSnippet),
		"schedule_snippet":        routeAction(client, snippetstoragemoves.Schedule),
		"schedule_all_snippet":    routeAction(client, snippetstoragemoves.ScheduleAll),
	}
	addMetaTool(server, "gitlab_storage_move", `Manage repository storage moves for projects, groups, and snippets (admin only).
Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

Param conventions: * = required. retrieve_all/list actions accept page, per_page. Each resource type (project/group/snippet) has the same action pattern.

Project storage moves:
- retrieve_all_project: page, per_page
- retrieve_project: project_id*, page, per_page
- get_project: id*
- get_project_for_project: project_id*, id*
- schedule_project: project_id*, destination_storage_name
- schedule_all_project: source_storage_name, destination_storage_name

Group storage moves:
- retrieve_all_group: page, per_page
- retrieve_group: group_id*, page, per_page
- get_group: id*
- get_group_for_group: group_id*, id*
- schedule_group: group_id*, destination_storage_name
- schedule_all_group: source_storage_name, destination_storage_name

Snippet storage moves:
- retrieve_all_snippet: page, per_page
- retrieve_snippet: snippet_id*, page, per_page
- get_snippet: id*
- get_snippet_for_snippet: snippet_id*, id*
- schedule_snippet: snippet_id*, destination_storage_name
- schedule_all_snippet: source_storage_name, destination_storage_name

See also: gitlab_admin`, routes, toolutil.IconServer)
}

// registerVulnerabilityMeta registers the gitlab_vulnerability meta-tool with actions:
// list, get, dismiss, confirm, resolve, revert, severity_count, pipeline_security_summary.
func registerVulnerabilityMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                      routeAction(client, vulnerabilities.List),
		"get":                       routeAction(client, vulnerabilities.Get),
		"dismiss":                   routeAction(client, vulnerabilities.Dismiss),
		"confirm":                   routeAction(client, vulnerabilities.Confirm),
		"resolve":                   routeAction(client, vulnerabilities.Resolve),
		"revert":                    routeAction(client, vulnerabilities.Revert),
		"severity_count":            routeAction(client, vulnerabilities.SeverityCount),
		"pipeline_security_summary": routeAction(client, vulnerabilities.PipelineSecuritySummary),
	}
	addMetaTool(server, "gitlab_vulnerability", `List, triage, and summarize project vulnerabilities (Premium/Ultimate, GraphQL). Actions: list, get, dismiss, confirm, resolve, revert, severity_count, pipeline_security_summary.
Returns: JSON with resource data. The list action accepts first/after cursor pagination and returns action-specific pagination metadata. Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Param conventions: * = required. GID format: gid://gitlab/Vulnerability/42.

- list: project_path*, severity, state, scanner, report_type (arrays), has_issues, has_resolution, sort, first, after
- get / confirm / resolve / revert: id* (GID)
- dismiss: id* (GID), comment, dismissal_reason (ACCEPTABLE_RISK/FALSE_POSITIVE/MITIGATING_CONTROL/USED_IN_TESTS/NOT_APPLICABLE)
- severity_count: project_path*
- pipeline_security_summary: project_path*, pipeline_iid*

See also: gitlab_security_finding, gitlab_pipeline`, routes, toolutil.IconSecurity)
}

// registerSecurityFindingsMeta registers the gitlab_security_finding meta-tool with actions: list.
func registerSecurityFindingsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list": routeAction(client, securityfindings.List),
	}
	addReadOnlyMetaTool(server, "gitlab_security_finding", `List pipeline security report findings via GraphQL (Premium/Ultimate). Replaces deprecated REST vulnerability_findings endpoint.
Returns: JSON with resource data. The list action accepts first/after cursor pagination and returns action-specific pagination metadata. Errors: 404 not found, 403 forbidden — with actionable hints.

- list: project_path*, pipeline_iid*, severity, confidence, scanner, report_type (arrays), first, after

See also: gitlab_vulnerability, gitlab_pipeline`, routes, toolutil.IconSecurity)
}

// registerCICatalogMeta registers the gitlab_ci_catalog meta-tool with actions: list, get.
func registerCICatalogMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list": routeAction(client, cicatalog.List),
		"get":  routeAction(client, cicatalog.Get),
	}
	addReadOnlyMetaTool(server, "gitlab_ci_catalog", `Discover and inspect CI/CD Catalog resources (reusable pipeline components and templates published by groups for import into .gitlab-ci.yml). Read-only; GraphQL endpoint. The underlying GitLab API requires a Premium/Ultimate plan on the target instance (server enforces it with 403); the tool itself is always registered and is not gated by GITLAB_ENTERPRISE.
When to use: browse the Catalog to find reusable components, inspect a component's versions before pinning it in `+"`include:component`"+`, or audit which Catalog resources a publisher group exposes.
NOT for: running pipelines or pipeline definitions (use gitlab_pipeline), built-in GitLab templates such as gitignore/Dockerfile/license (use gitlab_template), CI YAML linting (use gitlab_template action=lint).

Returns:
- list: {nodes: [{id, full_path, name, description, latest_version, star_count}], page_info: {end_cursor, has_next_page}}.
- get: {id, full_path, name, description, latest_version, star_count, versions: [{version, released_at, tag_name}]}.
Errors: 404 not found (hint: check full_path or id), 403 forbidden (hint: requires Premium/Ultimate or Catalog read access), 400 invalid params (hint: provide id OR full_path).

Param conventions: * = required. id format = GID (gid://gitlab/Ci::Catalog::Resource/123). full_path = namespace/project (e.g. mygroup/components/docker-build).

- list: search, scope (ALL/NAMESPACED), sort (NAME_ASC/NAME_DESC/LATEST_RELEASED_AT_ASC/LATEST_RELEASED_AT_DESC/STAR_COUNT_ASC/STAR_COUNT_DESC), first (max 100), after (cursor)
- get: exactly one of id* or full_path* (mutually exclusive)

See also: gitlab_template (built-in templates and CI lint), gitlab_pipeline (run pipelines using catalog components), gitlab_project (publisher project metadata).`, routes, toolutil.IconPackage)
}

// registerCustomEmojiMeta registers the gitlab_custom_emoji meta-tool with actions: list, create, delete.
func registerCustomEmojiMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":   routeAction(client, customemoji.List),
		"create": routeAction(client, customemoji.Create),
		"delete": destructiveVoidAction(client, customemoji.Delete),
	}
	addMetaTool(server, "gitlab_custom_emoji", `Manage group-level custom emoji via GraphQL. Delete is destructive: existing reactions using the emoji remain in the database but render as :name: text. The underlying GitLab API requires a Premium/Ultimate plan on the target instance (server enforces it with 403); the tool itself is always registered and is not gated by GITLAB_ENTERPRISE.
When to use: list, add, or remove the custom emoji available to a group's projects (e.g. company logos, team mascots) used as reactions on issues/MRs/notes.
NOT for: posting or removing a reaction on an issue/MR/snippet/commit/note (use the `+"`emoji_issue_*`"+` / `+"`emoji_mr_*`"+` / `+"`emoji_snippet_*`"+` actions on gitlab_issue, gitlab_merge_request, or gitlab_snippet), Unicode emoji (built-in, no action required), instance-level emoji (not supported by GitLab).

Returns:
- list: {nodes: [{id, name, url, external (bool), created_at, user_permissions: {delete}}], page_info: {end_cursor, has_next_page}}.
- create: the created node {id, name, url, external, created_at}.
- delete: {success: bool, message: string}.
Errors: 404 not found (hint: check group_path or id GID), 403 forbidden (hint: requires Maintainer+ on the group and Premium/Ultimate), 400 invalid params (hint: name must not contain colons; url must be a publicly reachable image).

Param conventions: * = required. id format = GID (gid://gitlab/CustomEmoji/123). group_path = full namespace path (e.g. mygroup or mygroup/subgroup).

- list: group_path*, first (max 100), after (cursor)
- create: group_path*, name* (no colons), url* (HTTPS image URL)
- delete: id*

See also: gitlab_group (group settings and membership), gitlab_issue / gitlab_merge_request / gitlab_snippet (post reactions using the emoji).`, routes, toolutil.IconLabel)
}

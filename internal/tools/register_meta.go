// register_meta.go wires domain-scoped meta-tools to the MCP server.
// Meta-tools consolidate multiple related operations behind a single tool
// with an "action" parameter, reducing the number of tools exposed to the
// LLM and lowering token usage.

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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/broadcastmessages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/clusteragents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/customemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dbmigrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencyproxy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploymentmergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ffuserlists"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupepicboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupiterations"
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/instancevariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/metadata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/modelregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovalsettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/planlimits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectmirrors"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projecttemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedpackages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourcegroups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securitysettings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/sidekiq"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/systemhooks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterAllMeta wires meta-tools to the MCP server.
// Base: 28 meta-tools (24 inline + 3 delegated + 1 standalone).
// Enterprise: +15 inline = 43 meta-tools total.
// Each meta-tool dispatches to the underlying handler based on
// the "action" parameter. This reduces token usage for LLMs while preserving full functionality.
func RegisterAllMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	// Core domain meta-tools (inline handlers — enterprise routes injected when enabled)
	registerProjectMeta(server, client, enterprise)
	registerBranchMeta(server, client)
	registerTagMeta(server, client)
	registerReleaseMeta(server, client)
	registerMergeRequestMeta(server, client)
	registerMRReviewMeta(server, client)
	registerRepositoryMeta(server, client)
	registerGroupMeta(server, client, enterprise)
	registerIssueMeta(server, client, enterprise)
	registerPipelineMeta(server, client)
	registerJobMeta(server, client)
	registerUserMeta(server, client, enterprise)
	registerWikiMeta(server, client)
	registerEnvironmentMeta(server, client)
	registerCIVariableMeta(server, client)
	registerTemplateMeta(server, client)
	registerAdminMeta(server, client)

	// Consolidated domain meta-tools (inline handlers)
	registerAccessMeta(server, client)
	registerPackageMeta(server, client)
	registerSnippetMeta(server, client)
	registerFeatureFlagsMeta(server, client)

	// Free-tier meta-tools (available on CE — GraphQL/REST based)
	registerModelRegistryMeta(server, client)
	registerCICatalogMeta(server, client)
	registerCustomEmojiMeta(server, client)

	// Enterprise meta-tools (Premium/Ultimate — gated by GITLAB_ENTERPRISE)
	if enterprise {
		registerMergeTrainMeta(server, client)
		registerAuditEventMeta(server, client)
		registerDORAMetricsMeta(server, client)
		registerDependencyMeta(server, client)
		registerExternalStatusCheckMeta(server, client)
		registerGroupSCIMMeta(server, client)
		registerMemberRoleMeta(server, client)
		registerEnterpriseUserMeta(server, client)
		registerAttestationMeta(server, client)
		registerCompliancePolicyMeta(server, client)
		registerProjectAliasMeta(server, client)
		registerGeoMeta(server, client)
		registerStorageMoveMeta(server, client)
		registerVulnerabilityMeta(server, client)
		registerSecurityFindingsMeta(server, client)
	}

	// Delegated meta-tools (sub-package RegisterMeta)
	search.RegisterMeta(server, client)
	runners.RegisterMeta(server, client)
	samplingtools.RegisterMeta(server, client)

	// Standalone utility tools (not consolidated into meta-tools)
	projectdiscovery.RegisterTools(server, client)
}

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
		"mirror_force_push":     routeVoidAction(client, projectmirrors.ForcePushUpdate),
	}

	if enterprise {
		routes["push_rule_get"] = routeAction(client, projects.GetPushRules)
		routes["push_rule_add"] = routeAction(client, projects.AddPushRule)
		routes["push_rule_edit"] = routeAction(client, projects.EditPushRule)
		routes["push_rule_delete"] = destructiveVoidAction(client, projects.DeletePushRule)
		routes["security_settings_get"] = routeAction(client, securitysettings.GetProject)
		routes["security_settings_update"] = routeAction(client, securitysettings.UpdateProject)
	}

	desc := `Manage GitLab projects: CRUD, settings, members, labels, milestones, webhooks, badges, boards, integrations, uploads, Pages, avatars, approval rules, mirrors, and import/export. Delete and unpublish actions are destructive.
Valid actions: ` + validActionsString(routes) + `

When to use: project-level CRUD, settings, members, labels, milestones, webhooks, boards, integrations, Pages, mirrors, approval rules. NOT for: file content/commits (use gitlab_repository), branches (use gitlab_branch), wiki pages (use gitlab_wiki), issues (use gitlab_issue), MRs (use gitlab_merge_request).

Returns:
- list / list_* / *_list / members / list_forks / list_starrers / list_groups / hook_list / label_list / milestone_list / badge_list / board_list / board_list_list / integration_list / pages_domain_list / pages_domain_list_all / approval_rule_list / mirror_list / list_invited_groups / upload_list: arrays with pagination {page, per_page, total, next_page}.
- get / create / update / fork / transfer / star / unstar / archive / unarchive / restore / member_get / member_inherited / member_add / member_edit / hook_get / hook_add / hook_edit / hook_test / label_* / milestone_* (non-list) / badge_get / badge_add / badge_edit / badge_preview / board_get / board_create / board_update / board_list_get / board_list_create / board_list_update / integration_get / integration_set_jira / pages_get / pages_update / pages_domain_get / pages_domain_create / pages_domain_update / approval_config_get / approval_config_change / approval_rule_get / approval_rule_create / approval_rule_update / pull_mirror_get / pull_mirror_configure / mirror_get / mirror_get_public_key / mirror_add / mirror_edit / repository_storage_get / statistics_get / languages / share_with_group / upload / upload_avatar / download_avatar / create_for_user / create_fork_relation / export_status / export_download / import_from_file / import_status / export_schedule: resource object.
- delete / hook_delete / hook_set_*/hook_delete_* / label_delete / label_subscribe / label_unsubscribe / label_promote / milestone_delete / badge_delete / board_delete / board_list_delete / integration_delete / pages_unpublish / pages_domain_delete / approval_rule_delete / mirror_delete / mirror_force_push / delete_shared_group / delete_fork_relation / start_mirroring / start_housekeeping / upload_delete: {success, message}.
Errors: 404 (hint: project_id may be a numeric ID or URL-encoded path like 'group%2Frepo'), 403 (hint: most mutations require Maintainer+, settings/transfers require Owner, instance-level actions require admin), 400 (hint: visibility ∈ private/internal/public; merge_method ∈ merge/rebase_merge/ff; namespace_id must be writable by the caller).

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

Webhooks (hook_*) — event booleans: push/tag_push/issues/merge_requests/note/job/pipeline/wiki_page/deployment/releases/emoji:
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

Milestones (milestone_*):
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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

When to use: publish a release for a tag, list/get/update releases, attach asset links to a release, batch-attach links after a CI build.
NOT for: creating tags (use gitlab_tag create first — release_create requires an existing tag_name), uploading binaries to the package registry (use gitlab_package), milestones (use gitlab_project milestone_*).

Returns:
- list: array of releases with pagination.
- get / get_latest / create / update: release object {name, tag_name, description, released_at, assets: {sources, links}, evidences, milestones}.
- link_list: array of {id, name, url, link_type, direct_asset_path}.
- link_create / link_create_batch / link_get / link_update: link object(s).
- delete / link_delete: {success: bool, message: string}.
Errors: 404 not found (hint: tag_name must exist), 403 forbidden (hint: requires Developer+ for create, Maintainer+ for update/delete), 400 invalid params (hint: link url must be absolute https://).

Param conventions: * = required. All actions need project_id*. Release actions need tag_name*. Link actions need tag_name* + link_id* (except link_create / link_create_batch / link_list).

Releases:
- create: project_id*, tag_name* (must exist), name, description (Markdown), released_at (ISO 8601), milestones ([]string)
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

See also: gitlab_tag (create the tag before the release), gitlab_package (upload binaries; link_create can point at the package URL), gitlab_project (milestones referenced by releases).`, routes, toolutil.IconRelease)
}

// registerMergeRequestMeta registers the gitlab_merge_request meta-tool with actions:
// create, get, list, list_global, list_group, update, merge, approve, unapprove,
// commits, pipelines, delete, rebase, participants, reviewers, create_pipeline,
// issues_closed, cancel_auto_merge, approval_state, approval_rules, approval_config,
// approval_reset, approval_rule_create, approval_rule_update, approval_rule_delete,
// approval_settings_group_get, approval_settings_group_update,
// approval_settings_project_get, approval_settings_project_update,
// subscribe, unsubscribe, time_estimate_set, time_estimate_reset, spent_time_add,
// spent_time_reset, time_stats, context_commits_list, context_commits_create,
// context_commits_delete.
func registerMergeRequestMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"create":                           routeAction(client, mergerequests.Create),
		"get":                              routeAction(client, mergerequests.Get),
		"list":                             routeAction(client, mergerequests.List),
		"list_global":                      routeAction(client, mergerequests.ListGlobal),
		"list_group":                       routeAction(client, mergerequests.ListGroup),
		"update":                           routeAction(client, mergerequests.Update),
		"merge":                            destructiveAction(client, mergerequests.Merge),
		"approve":                          routeAction(client, mergerequests.Approve),
		"unapprove":                        destructiveVoidAction(client, mergerequests.Unapprove),
		"commits":                          routeAction(client, mergerequests.Commits),
		"pipelines":                        routeAction(client, mergerequests.Pipelines),
		"delete":                           destructiveVoidAction(client, mergerequests.Delete),
		"rebase":                           routeAction(client, mergerequests.Rebase),
		"participants":                     routeAction(client, mergerequests.Participants),
		"reviewers":                        routeAction(client, mergerequests.Reviewers),
		"create_pipeline":                  routeAction(client, mergerequests.CreatePipeline),
		"issues_closed":                    routeAction(client, mergerequests.IssuesClosed),
		"cancel_auto_merge":                routeAction(client, mergerequests.CancelAutoMerge),
		"approval_state":                   routeAction(client, mrapprovals.State),
		"approval_rules":                   routeAction(client, mrapprovals.Rules),
		"approval_config":                  routeAction(client, mrapprovals.Config),
		"approval_reset":                   destructiveVoidAction(client, mrapprovals.Reset),
		"approval_rule_create":             routeAction(client, mrapprovals.CreateRule),
		"approval_rule_update":             routeAction(client, mrapprovals.UpdateRule),
		"approval_rule_delete":             destructiveVoidAction(client, mrapprovals.DeleteRule),
		"approval_settings_group_get":      routeAction(client, mrapprovalsettings.GetGroupSettings),
		"approval_settings_group_update":   routeAction(client, mrapprovalsettings.UpdateGroupSettings),
		"approval_settings_project_get":    routeAction(client, mrapprovalsettings.GetProjectSettings),
		"approval_settings_project_update": routeAction(client, mrapprovalsettings.UpdateProjectSettings),
		"subscribe":                        routeAction(client, mergerequests.Subscribe),
		"unsubscribe":                      routeAction(client, mergerequests.Unsubscribe),
		"time_estimate_set":                routeAction(client, mergerequests.SetTimeEstimate),
		"time_estimate_reset":              routeAction(client, mergerequests.ResetTimeEstimate),
		"spent_time_add":                   routeAction(client, mergerequests.AddSpentTime),
		"spent_time_reset":                 routeAction(client, mergerequests.ResetSpentTime),
		"time_stats":                       routeAction(client, mergerequests.GetTimeStats),
		"context_commits_list":             routeAction(client, mrcontextcommits.List),
		"context_commits_create":           routeAction(client, mrcontextcommits.Create),
		"context_commits_delete":           destructiveVoidAction(client, mrcontextcommits.Delete),
		"emoji_mr_list":                    routeAction(client, awardemoji.ListMRAwardEmoji),
		"emoji_mr_get":                     routeAction(client, awardemoji.GetMRAwardEmoji),
		"emoji_mr_create":                  routeAction(client, awardemoji.CreateMRAwardEmoji),
		"emoji_mr_delete":                  destructiveVoidAction(client, awardemoji.DeleteMRAwardEmoji),
		"emoji_mr_note_list":               routeAction(client, awardemoji.ListMRNoteAwardEmoji),
		"emoji_mr_note_get":                routeAction(client, awardemoji.GetMRNoteAwardEmoji),
		"emoji_mr_note_create":             routeAction(client, awardemoji.CreateMRNoteAwardEmoji),
		"emoji_mr_note_delete":             destructiveVoidAction(client, awardemoji.DeleteMRNoteAwardEmoji),
		"event_mr_label_list":              routeAction(client, resourceevents.ListMRLabelEvents),
		"event_mr_label_get":               routeAction(client, resourceevents.GetMRLabelEvent),
		"event_mr_milestone_list":          routeAction(client, resourceevents.ListMRMilestoneEvents),
		"event_mr_milestone_get":           routeAction(client, resourceevents.GetMRMilestoneEvent),
		"event_mr_state_list":              routeAction(client, resourceevents.ListMRStateEvents),
		"event_mr_state_get":               routeAction(client, resourceevents.GetMRStateEvent),
	}

	addMetaTool(server, "gitlab_merge_request", `Manage GitLab merge requests: create, list, get, update, merge, approve, rebase, delete. Also manages approval rules/settings, time tracking, subscriptions, context commits, award emoji, and resource events. Delete permanently removes an MR.
Valid actions: `+validActionsString(routes)+`

When to use: MR lifecycle (open/list/update/merge/close/delete/rebase), approvals at MR/group/project level, time tracking, subscriptions, context commits, award emoji, MR resource events.
NOT for: comments, discussions, diffs, draft notes (use gitlab_mr_review), CI pipelines (use gitlab_pipeline; use action 'pipelines' here only to LIST MR pipelines), branches/tags (use gitlab_branch / gitlab_tag), commits in the repo (use gitlab_repository).

Returns:
- list / list_global / list_group / commits / pipelines / participants / reviewers / issues_closed / approval_rules / context_commits_list / event_*_list / emoji_*_list: arrays with pagination {page, per_page, total, next_page}.
- get / create / update / approve / merge / rebase / approval_state / approval_config / approval_rule_create / approval_rule_update / approval_settings_*: MR or settings object.
- time_estimate_set / spent_time_add / time_stats / time_estimate_reset / spent_time_reset: {time_estimate, total_time_spent, human_time_estimate, human_total_time_spent}.
- subscribe / unsubscribe / cancel_auto_merge / create_pipeline: updated MR or pipeline object.
- delete / unapprove / approval_reset / approval_rule_delete / context_commits_delete / emoji_*_delete: {success, message}.
Errors: 404 (hint: confirm project_id and mr_iid — mr_iid is project-scoped, not the global ID), 403 (hint: requires Reporter+ to comment, Developer+ to merge, configured approvers to approve), 405/409 on merge (hint: WIP/draft, unresolved threads, failing pipelines or pending approvals — see approval_state).

Param conventions: * = required. Most actions need project_id*, mr_iid*. List actions accept page, per_page.

IMPORTANT for create: target_branch* — if user doesn't specify, retrieve project's default_branch via gitlab_project get; do NOT assume 'main'.
IMPORTANT for merge: auto-detects project squash/delete-branch settings — do NOT set squash or should_remove_source_branch unless user explicitly asks.

MR lifecycle:
- create: project_id*, source_branch*, target_branch*, title*, description, assignee_id, assignee_ids, reviewer_ids, labels (comma-separated), milestone_id, remove_source_branch, squash, allow_collaboration, target_project_id (forks)
- get: project_id*, mr_iid*
- list: project_id*, state (opened/closed/merged/all), labels, not_labels, milestone, scope, search, source_branch, target_branch, author_username, draft, iids, created_after/before, updated_after/before, order_by, sort
- list_global / list_group: same filters as list. list_group needs group_id* instead of project_id.
- update: project_id*, mr_iid*, title, description, target_branch, assignee_id, assignee_ids, reviewer_ids, labels, add_labels, remove_labels, milestone_id, remove_source_branch, squash, discussion_locked, allow_collaboration, state_event (close/reopen)
- merge: project_id*, mr_iid*, merge_commit_message, squash, should_remove_source_branch, auto_merge, sha, squash_commit_message
- approve / unapprove / rebase / delete / participants / reviewers / create_pipeline / cancel_auto_merge: project_id*, mr_iid*
- rebase also accepts: skip_ci
- commits / pipelines / issues_closed: project_id*, mr_iid*
- subscribe / unsubscribe: project_id*, mr_iid*

Approvals:
- approval_state / approval_rules / approval_config / approval_reset: project_id*, mr_iid*
- approval_rule_create: project_id*, mr_iid*, name*, approvals_required*, approval_project_rule_id, user_ids, group_ids
- approval_rule_update: project_id*, mr_iid*, approval_rule_id*, name, approvals_required, user_ids, group_ids
- approval_rule_delete: project_id*, mr_iid*, approval_rule_id*
- approval_settings_group_get / approval_settings_group_update: group_id*. Update params: allow_author_approval, allow_committer_approval, allow_overrides_approver_list_per_mr, retain_approvals_on_push, require_reauthentication_to_approve
- approval_settings_project_get / approval_settings_project_update: project_id*. Same params + selective_code_owner_removals.

Time tracking:
- time_estimate_set / spent_time_add: project_id*, mr_iid*, duration* (e.g. '3h30m', '1w2d'). spent_time_add also accepts summary.
- time_estimate_reset / spent_time_reset / time_stats: project_id*, mr_iid*

Context commits:
- context_commits_list / context_commits_create / context_commits_delete: project_id*, mr_iid*. create/delete need commits ([]string)*.

Award emoji:
- emoji_mr_list / emoji_mr_create / emoji_mr_delete: project_id*, iid*, name* (create), award_id* (get/delete)
- emoji_mr_get: project_id*, iid*, award_id*
- emoji_mr_note_list / emoji_mr_note_create / emoji_mr_note_delete: project_id*, iid*, note_id*, name* (create), award_id* (get/delete)
- emoji_mr_note_get: project_id*, iid*, note_id*, award_id*

Resource events:
- event_mr_label_list / event_mr_label_get: project_id*, mr_iid*, label_event_id* (get)
- event_mr_milestone_list / event_mr_milestone_get: project_id*, mr_iid*, milestone_event_id* (get)
- event_mr_state_list / event_mr_state_get: project_id*, mr_iid*, state_event_id* (get)

See also: gitlab_mr_review (comments, discussions, diffs, draft notes), gitlab_pipeline, gitlab_branch`, routes, toolutil.IconMR)
}

// registerMRReviewMeta registers the gitlab_mr_review meta-tool with actions:
// note_create, note_list, note_update, note_delete, discussion_create,
// discussion_list, discussion_get, discussion_reply, discussion_resolve,
// discussion_note_update, discussion_note_delete, changes_get,
// draft_note_list, draft_note_get, draft_note_create, draft_note_update,
// draft_note_delete, draft_note_publish, draft_note_publish_all,
// diff_versions_list, diff_version_get.
func registerMRReviewMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"note_create":            routeAction(client, mrnotes.Create),
		"note_list":              routeAction(client, mrnotes.List),
		"note_get":               routeAction(client, mrnotes.GetNote),
		"note_update":            routeAction(client, mrnotes.Update),
		"note_delete":            destructiveVoidAction(client, mrnotes.Delete),
		"discussion_create":      routeAction(client, mrdiscussions.Create),
		"discussion_list":        routeAction(client, mrdiscussions.List),
		"discussion_get":         routeAction(client, mrdiscussions.Get),
		"discussion_reply":       routeAction(client, mrdiscussions.Reply),
		"discussion_resolve":     routeAction(client, mrdiscussions.Resolve),
		"discussion_note_update": routeAction(client, mrdiscussions.UpdateNote),
		"discussion_note_delete": destructiveVoidAction(client, mrdiscussions.DeleteNote),
		"changes_get":            routeAction(client, mrchanges.Get),
		"draft_note_list":        routeAction(client, mrdraftnotes.List),
		"draft_note_get":         routeAction(client, mrdraftnotes.Get),
		"draft_note_create":      routeAction(client, mrdraftnotes.Create),
		"draft_note_update":      routeAction(client, mrdraftnotes.Update),
		"draft_note_delete":      destructiveVoidAction(client, mrdraftnotes.Delete),
		"draft_note_publish":     routeVoidAction(client, mrdraftnotes.Publish),
		"draft_note_publish_all": routeVoidAction(client, mrdraftnotes.PublishAll),
		"diff_versions_list":     routeAction(client, mrchanges.ListDiffVersions),
		"diff_version_get":       routeAction(client, mrchanges.GetDiffVersion),
	}

	addMetaTool(server, "gitlab_mr_review", `Review and comment on GitLab merge requests: notes, threaded discussions (inline + general), code diffs, draft notes (batch review), diff versions, and the per-version diff payload.
Valid actions: `+validActionsString(routes)+`

When to use: post review comments, open or resolve discussion threads, fetch the diff to comment inline, queue draft notes during a session and publish them as a single review.
NOT for: MR lifecycle — create/update/merge/approve/rebase/delete (use gitlab_merge_request), reactions on MR notes (use gitlab_merge_request emoji_mr_note_*), CI pipelines on the MR (use gitlab_pipeline or gitlab_merge_request pipelines).

IMPORTANT — batch review pattern: call draft_note_create once per comment (with `+"`position`"+` for inline comments, or `+"`in_reply_to_discussion_id`"+` for replies), then draft_note_publish_all ONCE to send a single notification. Use discussion_create only for standalone questions that need immediate visibility.

Returns:
- *_list: array with pagination (page, per_page, total, next_page).
- note_*, discussion_*, draft_note_*, diff_*: resource object(s) with id, body/note, author, position (when inline).
- changes_get: {changes: [{old_path, new_path, diff, ...}], truncated_files} — if truncated, use diff_versions_list + diff_version_get.
- *_delete / *_publish: {success: bool, message: string}.
Errors: 404 not found (hint: check note_id/discussion_id and mr_iid), 403 forbidden (hint: requires Reporter+ to comment), 400 invalid params (hint: position requires base_sha + start_sha + head_sha + new_path/old_path + new_line/old_line).

Param conventions: * = required. All actions need project_id*, mr_iid*. List actions accept page, per_page. position object: {base_sha, start_sha, head_sha, new_path, old_path, new_line (added/modified), old_line (removed), both lines for unchanged context}.

Notes (general comments):
- note_list: order_by (created_at/updated_at), sort
- note_get / note_delete: note_id*
- note_create: body*
- note_update: note_id*, body*

Discussions (threaded, can be inline via position):
- discussion_list
- discussion_get: discussion_id*
- discussion_create: body*, position (inline)
- discussion_reply: discussion_id*, body*
- discussion_resolve: discussion_id*, resolved* (bool)
- discussion_note_update: discussion_id*, note_id*, body, resolved
- discussion_note_delete: discussion_id*, note_id*

Changes and diff versions:
- changes_get: returns file diffs; check truncated_files
- diff_versions_list: list MR diff revisions
- diff_version_get: version_id*, unidiff (bool)

Draft notes (batch review):
- draft_note_list: order_by, sort
- draft_note_get: note_id*
- draft_note_create: note*, commit_id, in_reply_to_discussion_id, resolve_discussion (bool), position
- draft_note_update: note_id*, note, position
- draft_note_delete / draft_note_publish: note_id*
- draft_note_publish_all: publishes ALL pending drafts as a single review notification

See also: gitlab_merge_request (MR lifecycle, approvals, merge, time tracking, reactions), gitlab_pipeline (MR pipelines), gitlab_repository (file blame for context).`, routes, toolutil.IconDiscussion)
}

// registerRepositoryMeta registers the gitlab_repository meta-tool with actions:
// tree, compare, contributors, merge_base, blob, raw_blob, archive, changelog,
// commit operations, file operations, update_submodule, and markdown_render.
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
Valid actions: `+validActionsString(routes)+`

When to use: file/commit operations, diffs, blame, compare, archives, submodules, markdown rendering. NOT for: branch CRUD (use gitlab_branch), tag CRUD (use gitlab_tag).

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

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
- commit_diff: project_id*, sha*
- commit_refs: project_id*, sha*, type (branch/tag/all)
- commit_comments / commit_merge_requests: project_id*, sha*
- commit_comment_create: project_id*, sha*, note*, path, line, line_type (new/old)
- commit_statuses: project_id*, sha*, ref, stage, name, pipeline_id, all
- commit_status_set: project_id*, sha*, state* (pending/running/success/failed/canceled), ref, name, context, target_url, description, coverage, pipeline_id
- commit_cherry_pick: project_id*, sha*, branch*, dry_run, message
- commit_revert: project_id*, sha*, branch*
- commit_signature: project_id*, sha*

Files:
- file_get / file_raw / file_metadata / file_blame: project_id*, file_path*, ref. Blame also accepts range_start, range_end.
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
Valid actions: ` + validActionsString(routes) + `

When to use: group-level operations (groups, subgroups, members, labels, milestones, boards, webhooks, badges, wikis, epics). NOT for: project-specific operations (use gitlab_project or gitlab_merge_request), user accounts (use gitlab_user), cross-project search (use gitlab_search).

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

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

Milestones (group_milestone_*):
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
- release_list: group_id*, simple

See also: gitlab_project (project-level), gitlab_user (user management), gitlab_search (cross-project search), gitlab_merge_request (MR workflows)`

	if enterprise {
		desc += `

Epics (Premium+ — GITLAB_ENTERPRISE=true. CRUD/notes/discussions use Work Items GraphQL API with full_path + iid. Only epic_get_links and epic boards use REST with group_id):

Epic discussions (epic_discussion_*): full_path*, iid* for all. GraphQL pagination: first, after, last, before.
- epic_discussion_list / epic_discussion_get (+ discussion_id*)
- epic_discussion_create: body*
- epic_discussion_add_note: discussion_id*, body*
- epic_discussion_update_note: note_id*, body*
- epic_discussion_delete_note: note_id*

Epic CRUD (epic_*): full_path* for all.
- epic_list: state, search, author_username, label_name, confidential, sort, first, after, include_ancestors, include_descendants
- epic_get: iid*
- epic_get_links: iid* [REST]
- epic_create: title*, description, confidential, color, start_date, due_date, assignee_ids, label_ids, weight, health_status
- epic_update: iid*, title, description, state_event, color, start_date, due_date, add_label_ids, remove_label_ids, assignee_ids, weight, health_status, status
- epic_delete: iid*

Epic issues (epic_issue_*): full_path*, iid* for all. GraphQL pagination: first, after, last, before.
- epic_issue_list
- epic_issue_assign / epic_issue_remove: child_project_path*, child_iid*
- epic_issue_update: child_id*, adjacent_id*, relative_position* (BEFORE/AFTER)

Epic notes (epic_note_*): full_path*, iid* for all. GraphQL pagination: first, after, last, before.
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

See also: gitlab_project (project-level), gitlab_user (user management), gitlab_search (cross-project search), gitlab_merge_request (MR workflows)`
	}

	addMetaTool(server, "gitlab_group", desc, routes, toolutil.IconGroup)
}

// registerIssueMeta registers the gitlab_issue meta-tool with actions:
// create, get, list, update, delete, note_create, note_list, note_get,
// note_update, note_delete, list_group, link_list, link_get, link_create, link_delete,
// work_item_get, work_item_list, work_item_create, work_item_update, work_item_delete.
func registerIssueMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := actionMap{
		"create":                     routeAction(client, issues.Create),
		"get":                        routeAction(client, issues.Get),
		"get_by_id":                  routeAction(client, issues.GetByID),
		"list":                       routeAction(client, issues.List),
		"list_all":                   routeAction(client, issues.ListAll),
		"update":                     routeAction(client, issues.Update),
		"delete":                     destructiveVoidAction(client, issues.Delete),
		"list_group":                 routeAction(client, issues.ListGroup),
		"reorder":                    routeAction(client, issues.Reorder),
		"move":                       routeAction(client, issues.Move),
		"subscribe":                  routeAction(client, issues.Subscribe),
		"unsubscribe":                routeAction(client, issues.Unsubscribe),
		"create_todo":                routeAction(client, issues.CreateTodo),
		"note_create":                routeAction(client, issuenotes.Create),
		"note_list":                  routeAction(client, issuenotes.List),
		"note_get":                   routeAction(client, issuenotes.GetNote),
		"note_update":                routeAction(client, issuenotes.Update),
		"note_delete":                destructiveVoidAction(client, issuenotes.Delete),
		"link_list":                  routeAction(client, issuelinks.List),
		"link_get":                   routeAction(client, issuelinks.Get),
		"link_create":                routeAction(client, issuelinks.Create),
		"link_delete":                destructiveVoidAction(client, issuelinks.Delete),
		"time_estimate_set":          routeAction(client, issues.SetTimeEstimate),
		"time_estimate_reset":        routeAction(client, issues.ResetTimeEstimate),
		"spent_time_add":             routeAction(client, issues.AddSpentTime),
		"spent_time_reset":           routeAction(client, issues.ResetSpentTime),
		"time_stats_get":             routeAction(client, issues.GetTimeStats),
		"participants":               routeAction(client, issues.GetParticipants),
		"mrs_closing":                routeAction(client, issues.ListMRsClosing),
		"mrs_related":                routeAction(client, issues.ListMRsRelated),
		"work_item_get":              routeAction(client, workitems.Get),
		"work_item_list":             routeAction(client, workitems.List),
		"work_item_create":           routeAction(client, workitems.Create),
		"work_item_update":           routeAction(client, workitems.Update),
		"work_item_delete":           destructiveVoidAction(client, workitems.Delete),
		"discussion_list":            routeAction(client, issuediscussions.List),
		"discussion_get":             routeAction(client, issuediscussions.Get),
		"discussion_create":          routeAction(client, issuediscussions.Create),
		"discussion_add_note":        routeAction(client, issuediscussions.AddNote),
		"discussion_update_note":     routeAction(client, issuediscussions.UpdateNote),
		"discussion_delete_note":     destructiveVoidAction(client, issuediscussions.DeleteNote),
		"statistics_get":             routeAction(client, issuestatistics.Get),
		"statistics_get_group":       routeAction(client, issuestatistics.GetGroup),
		"statistics_get_project":     routeAction(client, issuestatistics.GetProject),
		"emoji_issue_list":           routeAction(client, awardemoji.ListIssueAwardEmoji),
		"emoji_issue_get":            routeAction(client, awardemoji.GetIssueAwardEmoji),
		"emoji_issue_create":         routeAction(client, awardemoji.CreateIssueAwardEmoji),
		"emoji_issue_delete":         destructiveVoidAction(client, awardemoji.DeleteIssueAwardEmoji),
		"emoji_issue_note_list":      routeAction(client, awardemoji.ListIssueNoteAwardEmoji),
		"emoji_issue_note_get":       routeAction(client, awardemoji.GetIssueNoteAwardEmoji),
		"emoji_issue_note_create":    routeAction(client, awardemoji.CreateIssueNoteAwardEmoji),
		"emoji_issue_note_delete":    destructiveVoidAction(client, awardemoji.DeleteIssueNoteAwardEmoji),
		"event_issue_label_list":     routeAction(client, resourceevents.ListIssueLabelEvents),
		"event_issue_label_get":      routeAction(client, resourceevents.GetIssueLabelEvent),
		"event_issue_milestone_list": routeAction(client, resourceevents.ListIssueMilestoneEvents),
		"event_issue_milestone_get":  routeAction(client, resourceevents.GetIssueMilestoneEvent),
		"event_issue_state_list":     routeAction(client, resourceevents.ListIssueStateEvents),
		"event_issue_state_get":      routeAction(client, resourceevents.GetIssueStateEvent),
		"event_issue_iteration_list": routeAction(client, resourceevents.ListIssueIterationEvents),
		"event_issue_iteration_get":  routeAction(client, resourceevents.GetIssueIterationEvent),
		"event_issue_weight_list":    routeAction(client, resourceevents.ListIssueWeightEvents),
	}

	if enterprise {
		routes["iteration_list_project"] = routeAction(client, projectiterations.List)
		routes["iteration_list_group"] = routeAction(client, groupiterations.List)
	}

	desc := `Manage GitLab issues: CRUD, notes, discussions, links, time tracking, work items, award emoji, statistics, and resource events.
Valid actions: ` + validActionsString(routes) + `

When to use: issue lifecycle — creating, updating, closing, moving, commenting, linking issues, time tracking, and work item management (including Epics via Work Items API). NOT for: merge request operations (use gitlab_merge_request), project settings (use gitlab_project), CI/CD pipelines (use gitlab_pipeline).

Side effects: delete/move are irreversible. move transfers the issue to another project (changes URL, IID). Time tracking uses dedicated actions (time_estimate_set, spent_time_add) — do NOT pass time params to update.

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions (delete) return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Param conventions: * = required. Most actions need project_id* + issue_iid*. List actions accept page, per_page. Work item actions use full_path* + iid* (GraphQL).

Issue CRUD:
- create: project_id*, title*, description, assignee_id, assignee_ids ([]int), labels (comma-separated), milestone_id, due_date (YYYY-MM-DD), confidential, issue_type (issue/incident/test_case/task), weight, epic_id
- get: project_id*, issue_iid*
- get_by_id: issue_id* (global ID, no project_id needed)
- list: project_id*, state (opened/closed/all), labels, not_labels, milestone, scope (created_by_me/assigned_to_me/all), search, assignee_username, author_username, iids ([]int), issue_type, confidential, created_after/before, updated_after/before (ISO 8601), order_by (created_at/updated_at/priority/due_date), sort (asc/desc)
- list_all: global issue list (no project_id). Same filters as list.
- list_group: group_id*, state, labels, milestone, scope, search, order_by, sort
- update: project_id*, issue_iid*, title, description, state_event (close/reopen), assignee_id, assignee_ids, labels, add_labels, remove_labels, milestone_id, due_date, confidential, issue_type, weight, epic_id, discussion_locked
- delete: project_id*, issue_iid* (permanent, irreversible)
- reorder: project_id*, issue_iid*, move_after_id, move_before_id
- move: project_id*, issue_iid*, to_project_id* (moves to another project)
- subscribe / unsubscribe: project_id*, issue_iid*
- create_todo: project_id*, issue_iid*

Time tracking:
- time_estimate_set: project_id*, issue_iid*, duration* (e.g. 3h30m)
- time_estimate_reset / spent_time_reset: project_id*, issue_iid*
- spent_time_add: project_id*, issue_iid*, duration*, summary
- time_stats_get: project_id*, issue_iid*

Participants & related MRs:
- participants: project_id*, issue_iid*
- mrs_closing / mrs_related: project_id*, issue_iid*

Notes (note_*): project_id*, issue_iid* for all.
- note_list: order_by, sort
- note_get / note_delete: note_id*
- note_create: body*, internal
- note_update: note_id*, body*

Issue links (link_*): project_id*, issue_iid* for all.
- link_list
- link_get / link_delete: issue_link_id*
- link_create: target_project_id*, target_issue_iid*, link_type

Discussions (discussion_*): project_id*, issue_iid* for all.
- discussion_list
- discussion_get: discussion_id*
- discussion_create: body*
- discussion_add_note: discussion_id*, body*
- discussion_update_note: discussion_id*, note_id*, body*
- discussion_delete_note: discussion_id*, note_id*

Work Items (work_item_*): full_path* for all. Use types=["Epic"] to list epics (replaces deprecated epic_list).
- work_item_list: state, search, types, author_username, label_name, confidential, sort, first, after
- work_item_get: iid*
- work_item_create: work_item_type_id*, title*, description, confidential, assignee_ids, milestone_id, label_ids, weight, health_status, color, due_date, start_date, linked_items {work_item_ids, link_type}
- work_item_update: iid*, title, state_event (CLOSE/REOPEN), description, assignee_ids, milestone_id, crm_contact_ids, parent_id, add_label_ids, remove_label_ids, start_date, due_date, weight, health_status, iteration_id, color, status (TODO/IN_PROGRESS/DONE/WONT_DO/DUPLICATE)
- work_item_delete: iid* (permanent)

Statistics:
- statistics_get: global issue stats (optional filters same as list)
- statistics_get_group: group_id*
- statistics_get_project: project_id*

Award emoji (emoji_issue_*): project_id*, iid* for all.
- emoji_issue_list / emoji_issue_get (+ award_id*) / emoji_issue_delete (+ award_id*)
- emoji_issue_create: name*
- emoji_issue_note_list / emoji_issue_note_get: note_id*, (+ award_id* for get)
- emoji_issue_note_create: note_id*, name*
- emoji_issue_note_delete: note_id*, award_id*

Resource events (event_issue_*): project_id*, issue_iid* for all.
- event_issue_label_list / event_issue_label_get (+ label_event_id*)
- event_issue_milestone_list / event_issue_milestone_get (+ milestone_event_id*)
- event_issue_state_list / event_issue_state_get (+ state_event_id*)
- event_issue_iteration_list / event_issue_iteration_get (+ iteration_event_id*)
- event_issue_weight_list`

	if enterprise {
		desc += `

Iterations (Premium+ — requires GITLAB_ENTERPRISE=true):
- iteration_list_project: project_id*, state (1=opened, 2=upcoming, 3=current, 4=closed), search, include_ancestors
- iteration_list_group: group_id*, state, search, include_ancestors`
	}

	desc += `

See also: gitlab_merge_request (MR lifecycle), gitlab_project (project settings, labels, milestones), gitlab_pipeline (CI/CD)

Returns:
- Paginated list actions ({page, per_page, total, next_page}): list / list_all / list_group / participants / mrs_closing / mrs_related / link_list / discussion_list / note_list / emoji_issue_list / emoji_issue_note_list / event_issue_label_list / event_issue_milestone_list / event_issue_state_list / event_issue_iteration_list / event_issue_weight_list.
- Cursor-paginated GraphQL list actions ({nodes, page_info: {end_cursor, has_next_page}}): work_item_list.
- Single-object actions: get / get_by_id / create / update / move / reorder / subscribe / unsubscribe / create_todo / link_get / link_create / discussion_get / discussion_create / discussion_add_note / discussion_update_note / note_get / note_create / note_update / time_estimate_set / time_estimate_reset / spent_time_add / spent_time_reset / time_stats_get / work_item_get / work_item_create / work_item_update / emoji_issue_get / emoji_issue_create / emoji_issue_note_get / emoji_issue_note_create / event_issue_label_get / event_issue_milestone_get / event_issue_state_get / event_issue_iteration_get / statistics_get / statistics_get_group / statistics_get_project: issue or sub-resource object.
- Void actions ({success, message}): delete / link_delete / note_delete / discussion_delete_note / work_item_delete / emoji_issue_delete / emoji_issue_note_delete.`

	if enterprise {
		desc += `
- Premium+ iteration list actions (paginated, only when GITLAB_ENTERPRISE=true): iteration_list_project / iteration_list_group.`
	}

	desc += `
Errors: 404 (hint: issue_iid is project-scoped — supply project_id; for list_all use scope/iids), 403 (hint: requires Reporter+ to comment, Developer+ to edit/move, configured permissions to set confidential), 400 (hint: state_event ∈ close/reopen; due_date / created_after must be ISO 8601; weight is integer 0–9 — Premium+).`

	addMetaTool(server, "gitlab_issue", desc, routes, toolutil.IconIssue)
}

// registerPipelineMeta registers the gitlab_pipeline meta-tool with actions:
// list, get, cancel, retry, and delete.
func registerPipelineMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                              routeAction(client, pipelines.List),
		"get":                               routeAction(client, pipelines.Get),
		"cancel":                            routeAction(client, pipelines.Cancel),
		"retry":                             routeAction(client, pipelines.Retry),
		"delete":                            destructiveVoidAction(client, pipelines.Delete),
		"variables":                         routeAction(client, pipelines.GetVariables),
		"test_report":                       routeAction(client, pipelines.GetTestReport),
		"test_report_summary":               routeAction(client, pipelines.GetTestReportSummary),
		"latest":                            routeAction(client, pipelines.GetLatest),
		"create":                            routeAction(client, pipelines.Create),
		"update_metadata":                   routeAction(client, pipelines.UpdateMetadata),
		"wait":                              routeActionWithRequest(client, pipelines.Wait),
		"trigger_list":                      routeAction(client, pipelinetriggers.ListTriggers),
		"trigger_get":                       routeAction(client, pipelinetriggers.GetTrigger),
		"trigger_create":                    routeAction(client, pipelinetriggers.CreateTrigger),
		"trigger_update":                    routeAction(client, pipelinetriggers.UpdateTrigger),
		"trigger_delete":                    destructiveVoidAction(client, pipelinetriggers.DeleteTrigger),
		"trigger_run":                       routeAction(client, pipelinetriggers.RunTrigger),
		"resource_group_list":               routeAction(client, resourcegroups.ListAll),
		"resource_group_get":                routeAction(client, resourcegroups.Get),
		"resource_group_edit":               routeAction(client, resourcegroups.Edit),
		"resource_group_upcoming_jobs":      routeAction(client, resourcegroups.ListUpcomingJobs),
		"schedule_list":                     routeAction(client, pipelineschedules.List),
		"schedule_get":                      routeAction(client, pipelineschedules.Get),
		"schedule_create":                   routeAction(client, pipelineschedules.Create),
		"schedule_update":                   routeAction(client, pipelineschedules.Update),
		"schedule_delete":                   destructiveVoidAction(client, pipelineschedules.Delete),
		"schedule_run":                      routeAction(client, pipelineschedules.Run),
		"schedule_take_ownership":           routeAction(client, pipelineschedules.TakeOwnership),
		"schedule_create_variable":          routeAction(client, pipelineschedules.CreateVariable),
		"schedule_edit_variable":            routeAction(client, pipelineschedules.EditVariable),
		"schedule_delete_variable":          destructiveVoidAction(client, pipelineschedules.DeleteVariable),
		"schedule_list_triggered_pipelines": routeAction(client, pipelineschedules.ListTriggeredPipelines),
	}

	addMetaTool(server, "gitlab_pipeline", `Manage GitLab CI/CD pipelines: list, get, create, retry, cancel, delete, and wait for completion. Also manages trigger tokens, resource groups, test reports, and pipeline schedules. Delete permanently removes a pipeline and all its jobs.
Valid actions: `+validActionsString(routes)+`

When to use: pipeline CRUD on a project, retry/cancel a run, fetch CI variables and JUnit test reports, manage trigger tokens, resource groups (mutual-exclusion locks), scheduled pipelines and their variables.
NOT for: jobs, logs, artifacts, manual play actions (use gitlab_job), MR-specific pipelines (use gitlab_merge_request 'pipelines' / 'create_pipeline'), CI lint or includes (use gitlab_template).

Returns:
- list / latest / variables / test_report / test_report_summary / trigger_list / resource_group_list / resource_group_upcoming_jobs / schedule_list / schedule_list_triggered_pipelines: array(s) or aggregated payloads with pagination where applicable.
- get / create / cancel / retry / update_metadata / wait / trigger_get / trigger_create / trigger_update / trigger_run / resource_group_get / resource_group_edit / schedule_get / schedule_create / schedule_update / schedule_run / schedule_take_ownership / schedule_create_variable / schedule_edit_variable: pipeline / trigger / resource group / schedule object.
- delete / trigger_delete / schedule_delete / schedule_delete_variable: {success, message}.
Errors: 404 (hint: pipeline_id and trigger/schedule IDs are project-scoped), 403 (hint: requires Maintainer+ to delete pipelines or manage triggers/schedules), 400 (hint: cron expressions must use 5 fields; cron_timezone must be a valid TZ name; create requires 'ref').

Param conventions: * = required. All pipeline actions need project_id*. List actions accept page, per_page.

Pipelines:
- list: project_id*, status (success/failed/running/pending/canceled), scope, source, ref, sha, username
- get / cancel / retry / variables / test_report / test_report_summary: project_id*, pipeline_id*
- delete: project_id*, pipeline_id*. PERMANENTLY removes pipeline and jobs.
- latest: project_id*, ref
- create: project_id*, ref*, variables (array of {key, value, variable_type})
- update_metadata: project_id*, pipeline_id*, name*
- wait: project_id*, pipeline_id*, interval_seconds (5-60, default 10), timeout_seconds (1-3600, default 300), fail_on_error (default true)

Triggers:
- trigger_list: project_id*
- trigger_get / trigger_delete: project_id*, trigger_id*
- trigger_create: project_id*, description*
- trigger_update: project_id*, trigger_id*, description
- trigger_run: project_id*, ref*, token*, variables (map)

Resource groups:
- resource_group_list: project_id*
- resource_group_get / resource_group_edit: project_id*, key*. Edit params: process_mode.
- resource_group_upcoming_jobs: project_id*, key*

Schedules:
- schedule_list: project_id*, scope (active/inactive)
- schedule_get / schedule_delete / schedule_run / schedule_take_ownership: project_id*, schedule_id*
- schedule_create: project_id*, description*, ref*, cron*, cron_timezone, active
- schedule_update: project_id*, schedule_id*, description, ref, cron, cron_timezone, active
- schedule_create_variable: project_id*, schedule_id*, key*, value*, variable_type (env_var/file)
- schedule_edit_variable: project_id*, schedule_id*, key*, value*, variable_type
- schedule_delete_variable: project_id*, schedule_id*, key*
- schedule_list_triggered_pipelines: project_id*, schedule_id*

See also: gitlab_job (job details/logs/artifacts), gitlab_merge_request, gitlab_ci_variable`, routes, toolutil.IconPipeline)
}

// registerJobMeta registers the gitlab_job meta-tool with actions:
// list, list_project, get, trace, cancel, retry, wait, list_bridges, artifacts, download_artifacts,
// download_single_artifact, download_single_artifact_by_ref, erase, keep_artifacts, play,
// delete_artifacts, delete_project_artifacts.
func registerJobMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                            routeAction(client, jobs.List),
		"list_project":                    routeAction(client, jobs.ListProject),
		"get":                             routeAction(client, jobs.Get),
		"trace":                           routeAction(client, jobs.Trace),
		"cancel":                          routeAction(client, jobs.Cancel),
		"retry":                           routeAction(client, jobs.Retry),
		"list_bridges":                    routeAction(client, jobs.ListBridges),
		"artifacts":                       routeAction(client, jobs.GetArtifacts),
		"download_artifacts":              routeAction(client, jobs.DownloadArtifacts),
		"download_single_artifact":        routeAction(client, jobs.DownloadSingleArtifact),
		"download_single_artifact_by_ref": routeAction(client, jobs.DownloadSingleArtifactByRef),
		"erase":                           destructiveAction(client, jobs.Erase),
		"keep_artifacts":                  routeAction(client, jobs.KeepArtifacts),
		"play":                            routeAction(client, jobs.Play),
		"delete_artifacts":                destructiveVoidAction(client, jobs.DeleteArtifacts),
		"delete_project_artifacts":        destructiveVoidAction(client, jobs.DeleteProjectArtifacts),
		"wait":                            routeActionWithRequest(client, jobs.Wait),
		"token_scope_get":                 routeAction(client, jobtokenscope.GetAccessSettings),
		"token_scope_patch":               routeAction(client, jobtokenscope.PatchAccessSettings),
		"token_scope_list_inbound":        routeAction(client, jobtokenscope.ListInboundAllowlist),
		"token_scope_add_project":         routeAction(client, jobtokenscope.AddProjectAllowlist),
		"token_scope_remove_project":      destructiveVoidAction(client, jobtokenscope.RemoveProjectAllowlist),
		"token_scope_list_groups":         routeAction(client, jobtokenscope.ListGroupAllowlist),
		"token_scope_add_group":           routeAction(client, jobtokenscope.AddGroupAllowlist),
		"token_scope_remove_group":        destructiveVoidAction(client, jobtokenscope.RemoveGroupAllowlist),
	}

	addMetaTool(server, "gitlab_job", `Manage GitLab CI/CD jobs: list, get, retry, cancel, erase, play manual jobs, wait for completion, download artifacts/logs, and manage CI/CD job token scope. Erase/delete actions are destructive.
Valid actions: `+validActionsString(routes)+`

When to use: job details, logs, artifacts, retry/cancel jobs, job token scope. NOT for: pipeline-level operations (use gitlab_pipeline).

Param conventions: * = required. All job actions need project_id*. List actions accept page, per_page.

Jobs:
- list: project_id*, pipeline_id*, scope
- list_project: project_id*, scope, include_retried
- get: project_id*, job_id*
- trace: project_id*, job_id*. Returns job log (truncated to 100KB).
- cancel / retry / erase / keep_artifacts: project_id*, job_id*
- play: project_id*, job_id*, variables (array of {key, value, variable_type})
- wait: project_id*, job_id*, interval_seconds (5-60, default 10), timeout_seconds (1-3600, default 300), fail_on_error (default true)
- list_bridges: project_id*, pipeline_id*, scope
- delete_artifacts: project_id*, job_id*
- delete_project_artifacts: project_id*. Deletes ALL artifacts across project.

Artifact downloads (base64, max 1MB):
- artifacts: project_id*, job_id*
- download_artifacts: project_id*, ref_name*, job
- download_single_artifact: project_id*, job_id*, artifact_path*
- download_single_artifact_by_ref: project_id*, ref_name*, artifact_path*, job

Job token scope:
- token_scope_get / token_scope_patch: project_id*. Patch params: enabled.
- token_scope_list_inbound: project_id*
- token_scope_add_project / token_scope_remove_project: project_id*, target_project_id*
- token_scope_list_groups: project_id*
- token_scope_add_group / token_scope_remove_group: project_id*, target_group_id*

See also: gitlab_pipeline, gitlab_repository`, routes, toolutil.IconJob)
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

	desc := `Manage GitLab users: CRUD, SSH/GPG keys, emails, PATs, impersonation tokens, status, todos, events, notifications, namespaces, and avatars. Delete/block/ban/reject actions are destructive.
Valid actions: ` + validActionsString(routes) + `

When to use: user CRUD, SSH/GPG key management, PATs, todos, events, notifications, namespaces. NOT for: deploy tokens or project/group access tokens (use gitlab_access), instance admin (use gitlab_admin).

Param conventions: * = required. User IDs are integers. List actions accept page, per_page.

Current user:
- current / me: no params. Returns authenticated user info.
- current_user_status: no params. Returns emoji, message, availability.
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
- ssh_keys: (current user, no params)
- ssh_keys_for_user: user_id*
- get_ssh_key: key_id*
- get_ssh_key_for_user: user_id*, key_id*
- add_ssh_key: title*, key*, expires_at, usage_type (auth/signing)
- add_ssh_key_for_user: user_id*, title*, key*, expires_at, usage_type
- delete_ssh_key: key_id*
- delete_ssh_key_for_user: user_id*, key_id*

GPG keys:
- gpg_keys: (current user, no params)
- gpg_keys_for_user: user_id*
- get_gpg_key: key_id*
- get_gpg_key_for_user: user_id*, key_id*
- add_gpg_key: key* (armored GPG public key)
- add_gpg_key_for_user: user_id*, key*
- delete_gpg_key: key_id*
- delete_gpg_key_for_user: user_id*, key_id*

Emails:
- emails: (current user, no params)
- emails_for_user: user_id*
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

// registerWikiMeta registers the gitlab_wiki meta-tool with actions:
// list, get, create, update, delete, upload_attachment.
func registerWikiMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":              routeAction(client, wikis.List),
		"get":               routeAction(client, wikis.Get),
		"create":            routeAction(client, wikis.Create),
		"update":            routeAction(client, wikis.Update),
		"delete":            destructiveVoidAction(client, wikis.Delete),
		"upload_attachment": routeAction(client, wikis.UploadAttachment),
	}

	addMetaTool(server, "gitlab_wiki", `CRUD project wiki pages and upload attachments to wikis. Delete is destructive and irreversible.
Valid actions: `+validActionsString(routes)+`

When to use: read, write, or delete wiki pages of a project; attach images or files referenced from wiki content.
NOT for: repository files or commits (use gitlab_repository), code snippets (use gitlab_snippet), group-level wikis (Enterprise/Premium — use gitlab_group when GITLAB_ENTERPRISE=true), issues or MR descriptions (use gitlab_issue / gitlab_merge_request).

Returns:
- get / create / update: {slug, title, format, content, encoding}.
- list: array of {slug, title, format} (content omitted unless with_content=true).
- delete: {success: bool, message: string}.
- upload_attachment: {file_name, url, alt, markdown} — embed `+"`markdown`"+` directly in a page.
Errors: 404 not found (hint: check slug or project_id), 403 forbidden (hint: wiki disabled or insufficient role), 400 invalid params (hint: title/content required, slug must be URL-encoded).

Param conventions: * = required. All actions need project_id* (numeric ID or url-encoded path). slug is the URL-encoded page path (e.g. `+"`docs%2Fsetup`"+`). format default = markdown. content max ~1 MB.

- list: project_id*, with_content (bool)
- get: project_id*, slug*, render_html (bool), version (commit SHA)
- create: project_id*, title*, content*, format (markdown/rdoc/asciidoc/org)
- update: project_id*, slug*, title, content, format
- delete: project_id*, slug*
- upload_attachment: project_id*, filename*, content_base64 OR file_path (exactly one), branch

See also: gitlab_project (settings/membership), gitlab_repository (file commits), gitlab_snippet (standalone code snippets).`, routes, toolutil.IconWiki)
}

// registerEnvironmentMeta registers the gitlab_environment meta-tool with actions:
// list, get, create, update, delete, stop.
func registerEnvironmentMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                         routeAction(client, environments.List),
		"get":                          routeAction(client, environments.Get),
		"create":                       routeAction(client, environments.Create),
		"update":                       routeAction(client, environments.Update),
		"delete":                       destructiveVoidAction(client, environments.Delete),
		"stop":                         destructiveAction(client, environments.Stop),
		"protected_list":               routeAction(client, protectedenvs.List),
		"protected_get":                routeAction(client, protectedenvs.Get),
		"protected_protect":            routeAction(client, protectedenvs.Protect),
		"protected_update":             routeAction(client, protectedenvs.Update),
		"protected_unprotect":          destructiveVoidAction(client, protectedenvs.Unprotect),
		"freeze_list":                  routeAction(client, freezeperiods.List),
		"freeze_get":                   routeAction(client, freezeperiods.Get),
		"freeze_create":                routeAction(client, freezeperiods.Create),
		"freeze_update":                routeAction(client, freezeperiods.Update),
		"freeze_delete":                destructiveVoidAction(client, freezeperiods.Delete),
		"deployment_list":              routeAction(client, deployments.List),
		"deployment_get":               routeAction(client, deployments.Get),
		"deployment_create":            routeAction(client, deployments.Create),
		"deployment_update":            routeAction(client, deployments.Update),
		"deployment_delete":            destructiveVoidAction(client, deployments.Delete),
		"deployment_approve_or_reject": routeAction(client, deployments.ApproveOrReject),
		"deployment_merge_requests":    routeAction(client, deploymentmergerequests.List),
	}

	addMetaTool(server, "gitlab_environment", `Manage GitLab deployment environments, protected environments, freeze (deploy block) periods, and the deployment record audit trail. Delete and stop are destructive (stop terminates the running env; force=true skips on-stop jobs).
Valid actions: `+validActionsString(routes)+`

When to use: define/update environments (production, staging, review/*), restrict who can deploy via protected environments, schedule deploy freezes, audit deployment history, approve/reject deployments awaiting manual gate.
NOT for: CI/CD variables scoped to environments (use gitlab_ci_variable), pipelines/jobs (use gitlab_pipeline / gitlab_job), feature flag rollout strategies (use gitlab_feature_flags).

Returns:
- *_list: array with pagination.
- *_get / *_create / *_update / *_protect: environment / protection / freeze / deployment object.
- deployment_approve_or_reject: updated deployment with approval state.
- deployment_merge_requests: MRs included in a given deployment.
- *_delete / *_unprotect / stop: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: protect/unprotect require Maintainer+), 400 invalid params (hint: tier ∈ production/staging/testing/development/other; freeze cron timezone must be valid TZ name).

Param conventions: * = required. All actions need project_id*. environment_id is the numeric ID returned by list/create.

Environments:
- list: project_id*, name, search, states (available/stopped/stopping), page, per_page
- get / delete: project_id*, environment_id*
- create: project_id*, name*, description, external_url, tier (production/staging/testing/development/other)
- update: project_id*, environment_id*, name, description, external_url, tier
- stop: project_id*, environment_id*, force (bool) — force skips on-stop jobs

Protected environments:
- protected_list: project_id*, page, per_page
- protected_get / protected_unprotect: project_id*, name*
- protected_protect / protected_update: project_id*, name*, deploy_access_levels, approval_rules

Freeze periods (cron expressions):
- freeze_list: project_id*, page, per_page
- freeze_get / freeze_delete: project_id*, freeze_period_id*
- freeze_create: project_id*, freeze_start* (cron, e.g. '0 23 * * 5'), freeze_end* (cron), cron_timezone
- freeze_update: project_id*, freeze_period_id*, freeze_start, freeze_end, cron_timezone

Deployments (immutable history records):
- deployment_list: project_id*, order_by, sort, environment, status, page, per_page
- deployment_get / deployment_delete: project_id*, deployment_id*
- deployment_create: project_id*, environment*, ref*, sha*, tag (bool), status (created/running/success/failed/canceled)
- deployment_update: project_id*, deployment_id*, status*
- deployment_approve_or_reject: project_id*, deployment_id*, status* (approved/rejected), comment
- deployment_merge_requests: project_id*, deployment_id*, state, order_by, sort, page, per_page

See also: gitlab_pipeline / gitlab_job (CI runs deploying to environments), gitlab_ci_variable (env-scoped variables), gitlab_feature_flags (env-scoped strategies).`, routes, toolutil.IconEnvironment)
}

// registerCIVariableMeta registers the gitlab_ci_variable meta-tool with actions:
// list, get, create, update, delete.
func registerCIVariableMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":            routeAction(client, civariables.List),
		"get":             routeAction(client, civariables.Get),
		"create":          routeAction(client, civariables.Create),
		"update":          routeAction(client, civariables.Update),
		"delete":          destructiveVoidAction(client, civariables.Delete),
		"group_list":      routeAction(client, groupvariables.List),
		"group_get":       routeAction(client, groupvariables.Get),
		"group_create":    routeAction(client, groupvariables.Create),
		"group_update":    routeAction(client, groupvariables.Update),
		"group_delete":    destructiveVoidAction(client, groupvariables.Delete),
		"instance_list":   routeAction(client, instancevariables.List),
		"instance_get":    routeAction(client, instancevariables.Get),
		"instance_create": routeAction(client, instancevariables.Create),
		"instance_update": routeAction(client, instancevariables.Update),
		"instance_delete": destructiveVoidAction(client, instancevariables.Delete),
	}

	addMetaTool(server, "gitlab_ci_variable", `Manage GitLab CI/CD variables at instance, group, and project scope. Delete actions are irreversible.
Valid actions: `+validActionsString(routes)+`

When to use: define / rotate / unmask / scope CI/CD variables at project, group, or instance level, both regular and secret (masked / masked_and_hidden), with environment scoping for per-env values.
NOT for: linting CI YAML or browsing CI templates (use gitlab_template), pipeline runs or schedules (use gitlab_pipeline), feature flags (use gitlab_feature_flags), per-deployment env metadata (use gitlab_environment), GitLab instance settings (use gitlab_admin).

Returns:
- list / group_list / instance_list: arrays of variable objects {key, value (or hidden), variable_type, protected, masked, raw, environment_scope, description} with pagination.
- get / create / update / group_get / group_create / group_update / instance_get / instance_create / instance_update: single variable object.
- delete / group_delete / instance_delete: {success, message}.
Errors: 404 (hint: a (key, environment_scope) pair must exist for get/update/delete — supply environment_scope when the variable is env-scoped), 403 (hint: project requires Maintainer+, group requires Owner, instance requires admin), 400 (hint: variable_type ∈ env_var/file; masked requires single-line non-empty value matching GitLab's masking rules).

Param conventions: * = required. Project-scoped actions need project_id*, group-scoped need group_id*, instance-scoped need no ID. Common optional params: variable_type, protected, masked, raw, environment_scope.

Project variables:
- list: project_id*
- get / delete: project_id*, key*, environment_scope
- create: project_id*, key*, value*, description, variable_type, protected, masked, masked_and_hidden, raw, environment_scope
- update: project_id*, key*, value, description, variable_type, protected, masked, raw, environment_scope

Group variables (group_*):
- group_list: group_id*
- group_get / group_delete: group_id*, key*
- group_create: group_id*, key*, value*, description, variable_type, protected, masked, raw, environment_scope
- group_update: group_id*, key*, value, description, variable_type, protected, masked, raw, environment_scope

Instance variables (instance_*):
- instance_list: (no params)
- instance_get / instance_delete: key*
- instance_create: key*, value*, description, variable_type, protected, masked, raw
- instance_update: key*, value, description, variable_type, protected, masked, raw

See also: gitlab_pipeline (pipeline operations), gitlab_template (CI lint)`, routes, toolutil.IconVariable)
}

// registerTemplateMeta registers the gitlab_template meta-tool with actions:
// lint, lint_project, ci_yml_list, ci_yml_get, dockerfile_list, dockerfile_get,
// gitignore_list, gitignore_get, license_list, license_get, project_template_list, project_template_get.
func registerTemplateMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"lint":                  routeAction(client, cilint.LintContent),
		"lint_project":          routeAction(client, cilint.LintProject),
		"ci_yml_list":           routeAction(client, ciyamltemplates.List),
		"ci_yml_get":            routeAction(client, ciyamltemplates.Get),
		"dockerfile_list":       routeAction(client, dockerfiletemplates.List),
		"dockerfile_get":        routeAction(client, dockerfiletemplates.Get),
		"gitignore_list":        routeAction(client, gitignoretemplates.List),
		"gitignore_get":         routeAction(client, gitignoretemplates.Get),
		"license_list":          routeAction(client, licensetemplates.List),
		"license_get":           routeAction(client, licensetemplates.Get),
		"project_template_list": routeAction(client, projecttemplates.List),
		"project_template_get":  routeAction(client, projecttemplates.Get),
	}

	addReadOnlyMetaTool(server, "gitlab_template", `Browse GitLab built-in templates (gitignore, CI/CD YAML, Dockerfile, license, project scaffolding) and lint CI configuration. Read-only; ci_lint may resolve `+"`include:`"+` directives that fetch remote URLs.
Valid actions: `+validActionsString(routes)+`

When to use: discover available built-in templates, fetch a template body to commit into a project, validate a .gitlab-ci.yml before pushing, or list project scaffolds.
NOT for: reusable Catalog components published by groups (use gitlab_ci_catalog), running pipelines (use gitlab_pipeline), CI/CD variables (use gitlab_ci_variable), repository files (use gitlab_repository).

Returns:
- *_list: [{key, name}] with pagination (page, per_page, total, next_page).
- *_get: {name, content} — paste `+"`content`"+` into the target file.
- lint / lint_project: {valid (bool), errors: [string], warnings: [string], merged_yaml (string), jobs: [...] when include_jobs=true}.
Errors: 404 not found (hint: check key or template_type), 403 forbidden, 400 invalid params (hint: content required for lint, project_id required for project_template_*).

Param conventions: * = required. template_type ∈ {dockerfiles, gitignores, gitlab_ci_ymls, licenses}.

CI lint:
- lint: project_id*, content*, dry_run (bool), include_jobs (bool), ref
- lint_project: project_id*, content_ref, dry_run (bool), dry_run_ref, include_jobs (bool), ref

Global templates:
- ci_yml_list / dockerfile_list / gitignore_list: page, per_page
- ci_yml_get / dockerfile_get / gitignore_get: key*
- license_list: page, per_page, popular (bool)
- license_get: key*, project, fullname

Project templates:
- project_template_list: project_id*, template_type*, page, per_page
- project_template_get: project_id*, template_type*, key*

See also: gitlab_ci_catalog (reusable Catalog components), gitlab_pipeline (run pipelines), gitlab_project (project membership/settings).`, routes, toolutil.IconTemplate)
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
		"feature_set":                    routeAction(client, features.Set),
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
		"db_migration_mark":              routeAction(client, dbmigrations.Mark),
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
		"terraform_state_unlock":         routeAction(client, terraformstates.Unlock),
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

	addMetaTool(server, "gitlab_admin", `GitLab self-managed instance administration: topics, settings, appearance, broadcast messages, instance feature flags, license, system hooks, Sidekiq metrics, plan limits, usage data, DB migrations, OAuth applications, custom attributes, error tracking, alert metric images, secure files, Terraform states, cluster agents, dependency proxy cache, and external imports. Most actions require admin privileges. Delete/purge/revoke actions are destructive.
Valid actions: `+validActionsString(routes)+`

When to use: instance-level admin tasks on a self-managed GitLab (settings, license, features, system hooks, Sidekiq monitoring, bulk imports from GitHub/Bitbucket).
NOT for: user CRUD (use gitlab_user), group/project administration (use gitlab_group / gitlab_project), MCP server itself (use gitlab_server), runtime feature flags per project (use gitlab_feature_flags), CI variables (use gitlab_ci_variable).

Returns:
- *_list: array with pagination.
- *_get / *_create / *_update / *_set / *_add: resource object.
- Sidekiq / usage_data / app_statistics / metadata: metrics objects.
- *_delete / *_revoke / *_purge / *_unlock: {success: bool, message: string}.
Errors: 401/403 forbidden (hint: most actions require admin token), 404 not found, 400 invalid params (hint: license must be base64-encoded; system hook url must be https).

Param conventions: * = required. List actions accept page, per_page.

Topics:
- topic_list: search
- topic_get / topic_delete: topic_id*
- topic_create: name*, title, description
- topic_update: topic_id*, name, title, description

Settings & appearance:
- settings_get / appearance_get: (no params)
- settings_update: settings (map of setting_name to value)
- appearance_update: title, description, header_message, footer_message, message_background_color, message_font_color, email_header_and_footer_enabled, pwa_name, pwa_short_name, pwa_description, member_guidelines, new_project_guidelines, profile_image_guidelines

Broadcast messages:
- broadcast_message_list: (no params)
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
Valid actions: `+validActionsString(routes)+`

When to use: manage deploy keys, deploy tokens, access tokens (project / group / personal), access requests, and invitations.
NOT for: SSH/GPG keys or user-scoped PATs created from the user profile (use gitlab_user), instance admin tokens or impersonation tokens (use gitlab_admin), project membership/permissions (use gitlab_project member_*), 2FA / MFA flows.

Returns:
- token_*_list / deploy_token_list_* / deploy_key_list_* / request_list_* / invite_list_*: arrays with pagination.
- token_*_get / token_*_create / token_*_rotate / deploy_token_get_* / deploy_token_create_* / deploy_key_get / deploy_key_add / deploy_key_update / deploy_key_enable / approve_* / request_*: token / key / request / invitation object. Create / rotate include the cleartext token only ONCE — store it securely; subsequent reads return only the metadata.
- token_*_revoke / deploy_token_delete_* / deploy_key_delete / deny_* : {success, message}.
Errors: 401/403 (hint: requires Maintainer+ to manage project tokens, Owner for group, admin for instance / deploy_token_list_all / deploy_key_list_all / deploy_key_add_instance), 404 (hint: token_id and deploy_key_id are scoped to the project/group), 400 (hint: scopes must be a subset of {api, read_api, read_repository, write_repository, read_registry, write_registry}; expires_at must be a future ISO date).

Param conventions: * = required. List actions accept page, per_page. Token actions scope to project_id* or group_id*. Deploy token/key delete and token revoke are irreversible.

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

Deploy keys (deploy_key_*) — SSH keys for read/write repo access without a user account:
- deploy_key_list_project / deploy_key_list_user_project: project_id*
- deploy_key_list_all: (admin only)
- deploy_key_get: project_id*, deploy_key_id*
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

// registerPackageMeta registers the gitlab_package meta-tool with actions from
// packages (publish, download, list, file_list, delete, file_delete, publish_and_link,
// publish_directory), container registry (registry_list_project, registry_list_group,
// registry_get, registry_delete, registry_tag_list, registry_tag_get, registry_tag_delete,
// registry_tag_delete_bulk, registry_rule_list, registry_rule_create, registry_rule_update,
// registry_rule_delete), and package protection rules (protection_rule_list, protection_rule_create,
// protection_rule_update, protection_rule_delete).
func registerPackageMeta(server *mcp.Server, client *gitlabclient.Client) {
	publishAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.PublishInput](params)
		if err != nil {
			return nil, err
		}
		return packages.Publish(ctx, nil, client, input)
	}
	downloadAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.DownloadInput](params)
		if err != nil {
			return nil, err
		}
		return packages.Download(ctx, nil, client, input)
	}
	deleteAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.DeleteInput](params)
		if err != nil {
			return nil, err
		}
		return nil, packages.Delete(ctx, nil, client, input)
	}
	fileDeleteAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.FileDeleteInput](params)
		if err != nil {
			return nil, err
		}
		return nil, packages.FileDelete(ctx, nil, client, input)
	}
	publishAndLinkAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.PublishAndLinkInput](params)
		if err != nil {
			return nil, err
		}
		return packages.PublishAndLink(ctx, nil, client, input)
	}
	publishDirAction := func(ctx context.Context, params map[string]any) (any, error) {
		input, err := unmarshalParams[packages.PublishDirInput](params)
		if err != nil {
			return nil, err
		}
		return packages.PublishDirectory(ctx, nil, client, input)
	}

	routes := actionMap{
		"publish":                  route(publishAction),
		"download":                 route(downloadAction),
		"list":                     routeAction(client, packages.List),
		"file_list":                routeAction(client, packages.FileList),
		"delete":                   destructiveRoute(deleteAction),
		"file_delete":              destructiveRoute(fileDeleteAction),
		"publish_and_link":         route(publishAndLinkAction),
		"publish_directory":        route(publishDirAction),
		"registry_list_project":    routeAction(client, containerregistry.ListProject),
		"registry_list_group":      routeAction(client, containerregistry.ListGroup),
		"registry_get":             routeAction(client, containerregistry.GetRepository),
		"registry_delete":          destructiveVoidAction(client, containerregistry.DeleteRepository),
		"registry_tag_list":        routeAction(client, containerregistry.ListTags),
		"registry_tag_get":         routeAction(client, containerregistry.GetTag),
		"registry_tag_delete":      destructiveVoidAction(client, containerregistry.DeleteTag),
		"registry_tag_delete_bulk": destructiveVoidAction(client, containerregistry.DeleteTagsBulk),
		"registry_rule_list":       routeAction(client, containerregistry.ListProtectionRules),
		"registry_rule_create":     routeAction(client, containerregistry.CreateProtectionRule),
		"registry_rule_update":     routeAction(client, containerregistry.UpdateProtectionRule),
		"registry_rule_delete":     destructiveVoidAction(client, containerregistry.DeleteProtectionRule),
		"protection_rule_list":     routeAction(client, protectedpackages.List),
		"protection_rule_create":   routeAction(client, protectedpackages.Create),
		"protection_rule_update":   routeAction(client, protectedpackages.Update),
		"protection_rule_delete":   destructiveVoidAction(client, protectedpackages.Delete),
	}

	addMetaTool(server, "gitlab_package", `Manage GitLab package registry, container registry, and protection rules. Upload/download generic packages, list/delete packages, browse container images/tags, and configure access policies. Delete actions are destructive.
Valid actions: `+validActionsString(routes)+`

When to use: publish / download / list / delete generic packages, browse npm/maven/conan/nuget/pypi/etc. metadata, browse and prune container images and tags, manage container and package protection rules.
NOT for: release asset links — these are managed by gitlab_release link_*; secure files (use gitlab_admin secure_file_*); ML model registry artifacts (use gitlab_model_registry); upload general project attachments (use gitlab_project upload).

Returns:
- list / file_list / registry_list_project / registry_list_group / registry_tag_list / registry_rule_list / protection_rule_list: arrays with pagination.
- publish / publish_and_link / publish_directory / registry_get / registry_tag_get / registry_rule_create / registry_rule_update / protection_rule_create / protection_rule_update: package / image / rule object. publish_and_link also returns the created release link.
- download: {file_name, content_base64 (or saved_to)} — large files are streamed to disk when output_path is set.
- delete / file_delete / registry_delete / registry_tag_delete / registry_tag_delete_bulk / registry_rule_delete / protection_rule_delete: {success, message}.
Errors: 404 (hint: package_id, repository_id and tag_name are project-scoped), 403 (hint: requires Maintainer+ to delete; protection rules may block delete with a 'forbidden by protection rule' message), 400 (hint: file_path must exist locally; content_base64 must be valid base64; package_type must be one of GitLab's supported types).

Param conventions: * = required. Most actions need project_id*. List actions accept page, per_page.

Packages:
- publish: project_id*, package_name*, package_version*, file_name*, file_path or content_base64 (one required), status (default/hidden)
- download: project_id*, package_name*, package_version*, file_name*, output_path*
- list: project_id*, package_name, package_version, package_type (generic/npm/maven/etc.), order_by, sort
- file_list: project_id*, package_id*
- delete: project_id*, package_id*. Deletes package and all files.
- file_delete: project_id*, package_id*, package_file_id*
- publish_and_link: publish + create release link. project_id*, package_name*, package_version*, file_name*, file_path or content_base64 (one required), tag_name*, link_name, link_type
- publish_directory: project_id*, package_name*, package_version*, directory_path*, include_pattern (glob), status

Container registry:
- registry_list_project: project_id*, tags, tags_count
- registry_list_group: group_id*
- registry_get: repository_id*, tags, tags_count
- registry_delete: project_id*, repository_id*
- registry_tag_list / registry_tag_get / registry_tag_delete: project_id*, repository_id*, tag_name* (for get/delete)
- registry_tag_delete_bulk: project_id*, repository_id*, name_regex_delete, name_regex_keep, keep_n, older_than

Container registry protection rules:
- registry_rule_list: project_id*
- registry_rule_create: project_id*, repository_path_pattern*, minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_update: project_id*, rule_id*, repository_path_pattern, minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_delete: project_id*, rule_id*

Package protection rules:
- protection_rule_list: project_id*
- protection_rule_create: project_id*, package_name_pattern*, package_type*, minimum_access_level_for_push, minimum_access_level_for_delete
- protection_rule_update: project_id*, rule_id*, package_name_pattern, package_type, minimum_access_level_for_push, minimum_access_level_for_delete
- protection_rule_delete: project_id*, rule_id*

See also: gitlab_release (release asset links), gitlab_project`, routes, toolutil.IconPackage)
}

// registerSnippetMeta registers the gitlab_snippet meta-tool with actions:
// list, list_all, get, content, file_content, create, update, delete, explore,
// project_list, project_get, project_content, project_create, project_update, project_delete,
// discussion_list, discussion_get, discussion_create, discussion_add_note,
// discussion_update_note, discussion_delete_note, note_list, note_get, note_create,
// note_update, and note_delete.
func registerSnippetMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":                      routeAction(client, snippets.List),
		"list_all":                  routeAction(client, snippets.ListAll),
		"get":                       routeAction(client, snippets.Get),
		"content":                   routeAction(client, snippets.Content),
		"file_content":              routeAction(client, snippets.FileContent),
		"create":                    routeAction(client, snippets.Create),
		"update":                    routeAction(client, snippets.Update),
		"delete":                    destructiveVoidAction(client, snippets.Delete),
		"explore":                   routeAction(client, snippets.Explore),
		"project_list":              routeAction(client, snippets.ProjectList),
		"project_get":               routeAction(client, snippets.ProjectGet),
		"project_content":           routeAction(client, snippets.ProjectContent),
		"project_create":            routeAction(client, snippets.ProjectCreate),
		"project_update":            routeAction(client, snippets.ProjectUpdate),
		"project_delete":            destructiveVoidAction(client, snippets.ProjectDelete),
		"discussion_list":           routeAction(client, snippetdiscussions.List),
		"discussion_get":            routeAction(client, snippetdiscussions.Get),
		"discussion_create":         routeAction(client, snippetdiscussions.Create),
		"discussion_add_note":       routeAction(client, snippetdiscussions.AddNote),
		"discussion_update_note":    routeAction(client, snippetdiscussions.UpdateNote),
		"discussion_delete_note":    destructiveVoidAction(client, snippetdiscussions.DeleteNote),
		"note_list":                 routeAction(client, snippetnotes.List),
		"note_get":                  routeAction(client, snippetnotes.Get),
		"note_create":               routeAction(client, snippetnotes.Create),
		"note_update":               routeAction(client, snippetnotes.Update),
		"note_delete":               destructiveVoidAction(client, snippetnotes.Delete),
		"emoji_snippet_list":        routeAction(client, awardemoji.ListSnippetAwardEmoji),
		"emoji_snippet_get":         routeAction(client, awardemoji.GetSnippetAwardEmoji),
		"emoji_snippet_create":      routeAction(client, awardemoji.CreateSnippetAwardEmoji),
		"emoji_snippet_delete":      destructiveVoidAction(client, awardemoji.DeleteSnippetAwardEmoji),
		"emoji_snippet_note_list":   routeAction(client, awardemoji.ListSnippetNoteAwardEmoji),
		"emoji_snippet_note_get":    routeAction(client, awardemoji.GetSnippetNoteAwardEmoji),
		"emoji_snippet_note_create": routeAction(client, awardemoji.CreateSnippetNoteAwardEmoji),
		"emoji_snippet_note_delete": destructiveVoidAction(client, awardemoji.DeleteSnippetNoteAwardEmoji),
	}
	addMetaTool(server, "gitlab_snippet", `Manage GitLab snippets (personal, project-scoped, and explore feed): CRUD snippet metadata and content, threaded discussions, notes (project snippets only), and award emoji on snippets and snippet notes. Delete is destructive and irreversible.
Valid actions: `+validActionsString(routes)+`

When to use: store/share standalone code or text outside a repo, comment on existing snippets, react with emoji on a snippet or snippet note, browse public snippets via explore.
NOT for: files in a repository (use gitlab_repository), wiki pages (use gitlab_wiki), MR/issue notes (use gitlab_mr_review or gitlab_issue), custom group emoji (use gitlab_custom_emoji — enterprise).

Returns:
- *_list / list_all / explore: array with pagination.
- *_get / *_create / *_update: snippet object {id, title, file_name, files, visibility, author, web_url, raw_url}.
- content / project_content: raw snippet body as text.
- file_content: raw content of a single file in a multi-file snippet.
- discussion_* / note_*: discussion or note object.
- emoji_*: award emoji object.
- *_delete: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: requires Reporter+ for project snippets; private snippets require ownership), 400 invalid params (hint: visibility ∈ private/internal/public).

Param conventions: * = required. List actions accept page, per_page. visibility ∈ private/internal/public.

Personal snippets:
- list / list_all / explore: (no required params)
- get / content: snippet_id*
- file_content: snippet_id*, file_path*
- create: title*, file_name*, content*, visibility, description
- update: snippet_id*, title, file_name, content, visibility, description
- delete: snippet_id*

Project snippets:
- project_list: project_id*
- project_get / project_content: project_id*, snippet_id*
- project_create: project_id*, title*, file_name*, content*, visibility
- project_update: project_id*, snippet_id*, title, file_name, content, visibility
- project_delete: project_id*, snippet_id*

Discussions (threaded):
- discussion_list: snippet_id*
- discussion_get: snippet_id*, discussion_id*
- discussion_create: snippet_id*, body*
- discussion_add_note: snippet_id*, discussion_id*, body*
- discussion_update_note: snippet_id*, discussion_id*, note_id*, body*
- discussion_delete_note: snippet_id*, discussion_id*, note_id*

Notes (project snippets only):
- note_list: project_id*, snippet_id*, order_by, sort
- note_get / note_delete: project_id*, snippet_id*, note_id*
- note_create: project_id*, snippet_id*, body*
- note_update: project_id*, snippet_id*, note_id*, body*

Award emoji:
- emoji_snippet_list / emoji_snippet_create / emoji_snippet_delete: project_id*, iid*, name* (create), award_id* (delete)
- emoji_snippet_get: project_id*, iid*, award_id*
- emoji_snippet_note_list / emoji_snippet_note_create / emoji_snippet_note_delete: project_id*, iid*, note_id*, name* (create), award_id* (delete)
- emoji_snippet_note_get: project_id*, iid*, note_id*, award_id*

See also: gitlab_repository (project files and commits), gitlab_wiki (long-form project docs), gitlab_project (project membership and visibility), gitlab_custom_emoji (define group-level custom emoji).`, routes, toolutil.IconSnippet)
}

// registerFeatureFlagsMeta registers the gitlab_feature_flags meta-tool with actions:
// feature_flag_list, feature_flag_get, feature_flag_create, feature_flag_update, feature_flag_delete,
// ff_user_list_list, ff_user_list_get, ff_user_list_create, ff_user_list_update, and ff_user_list_delete.
func registerFeatureFlagsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"feature_flag_list":   routeAction(client, featureflags.ListFeatureFlags),
		"feature_flag_get":    routeAction(client, featureflags.GetFeatureFlag),
		"feature_flag_create": routeAction(client, featureflags.CreateFeatureFlag),
		"feature_flag_update": routeAction(client, featureflags.UpdateFeatureFlag),
		"feature_flag_delete": destructiveVoidAction(client, featureflags.DeleteFeatureFlag),
		"ff_user_list_list":   routeAction(client, ffuserlists.ListUserLists),
		"ff_user_list_get":    routeAction(client, ffuserlists.GetUserList),
		"ff_user_list_create": routeAction(client, ffuserlists.CreateUserList),
		"ff_user_list_update": routeAction(client, ffuserlists.UpdateUserList),
		"ff_user_list_delete": destructiveVoidAction(client, ffuserlists.DeleteUserList),
	}
	addMetaTool(server, "gitlab_feature_flags", `Manage project feature flags and feature-flag user lists for gradual rollouts. Delete is destructive; setting active=false disables the flag but preserves history.
Valid actions: `+validActionsString(routes)+`

When to use: define rollout strategies (percentage, user-targeted, environment-scoped) for a project's feature flags, and manage the user lists referenced by `+"`gitlabUserList`"+` strategies.
NOT for: GitLab instance-level feature flags (admin only — use gitlab_admin), environment definitions or protection (use gitlab_environment), code branching (use gitlab_branch), CI/CD variables (use gitlab_ci_variable).

Returns:
- *_list: array with pagination (page, per_page, total, next_page).
- *_get / *_create / *_update: the resource object (flag includes strategies and scopes; user list includes user_xids).
- *_delete: {success: bool, message: string}.
Errors: 404 not found, 403 forbidden (hint: requires Developer+ role), 400 invalid params (hint: strategies/scopes JSON shape).

Param conventions: * = required. All actions need project_id*. version = `+"`new_version_flag`"+` (legacy `+"`legacy_flag`"+` deprecated).

strategies shape: [{name, parameters, scopes: [{environment_scope}]}] where name ∈ {default, gradualRolloutUserId, userWithId, flexibleRollout, gitlabUserList}. parameters per strategy: gradualRolloutUserId={groupId, percentage}; userWithId={userIds}; flexibleRollout={groupId, rollout, stickiness}; gitlabUserList={userListId}.

Feature flags (feature_flag_*):
- feature_flag_list: project_id*, scope (enabled/disabled), page, per_page
- feature_flag_get / feature_flag_delete: project_id*, name*
- feature_flag_create: project_id*, name*, version*, description, active (bool), strategies
- feature_flag_update: project_id*, name*, description, active (bool), strategies

User lists (ff_user_list_*) — named sets of user IDs referenced by gitlabUserList strategies:
- ff_user_list_list: project_id*, page, per_page
- ff_user_list_get / ff_user_list_delete: project_id*, iid*
- ff_user_list_create: project_id*, name*, user_xids* (comma-separated user IDs)
- ff_user_list_update: project_id*, iid*, name, user_xids

See also: gitlab_environment (environment scopes referenced by strategies), gitlab_admin (instance-level feature flags), gitlab_project (project membership and settings).`, routes, toolutil.IconConfig)
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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

Param conventions: * = required. All actions need project_id*.

- list_project: project_id*, scope (active/complete), sort (asc/desc), page, per_page
- list_branch: project_id*, target_branch*, scope, sort, page, per_page
- get: project_id*, merge_request_id*
- add: project_id*, merge_request_id*, auto_merge (bool), sha, squash (bool)

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
	addMetaTool(server, "gitlab_audit_event", `List and get GitLab audit events at instance, group, and project levels for compliance tracking.
Valid actions: `+validActionsString(routes)+`

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
	addMetaTool(server, "gitlab_dora_metrics", `Get DORA metrics: deployment frequency, lead time, MTTR, change failure rate.
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

Param conventions: * = required.

- list_project_checks: project_id*, page, per_page
- list_project_mr_checks: project_id*, mr_iid*, page, per_page
- list_project: project_id*, page, per_page
- create_project: project_id*, name*, external_url*, shared_secret, protected_branch_ids
- delete_project: project_id*, check_id*
- update_project: project_id*, check_id*, name, external_url, shared_secret, protected_branch_ids
- retry_project: project_id*, mr_iid*, check_id*
- set_project_mr_status: project_id*, mr_iid*, sha*, external_status_check_id*, status*

See also: gitlab_merge_request`, routes, toolutil.IconSecurity)
}

// registerGroupSCIMMeta registers the gitlab_group_scim meta-tool with actions
// for managing SCIM identities in a group.
func registerGroupSCIMMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := actionMap{
		"list":   routeAction(client, groupscim.List),
		"get":    routeAction(client, groupscim.Get),
		"update": routeVoidAction(client, groupscim.Update),
		"delete": destructiveVoidAction(client, groupscim.Delete),
	}
	addMetaTool(server, "gitlab_group_scim", `Manage SCIM identities for GitLab group provisioning.
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

When to use: pull a model artifact (.pkl, .onnx, .safetensors, .bin, .gguf, etc.) attached to a registered model version, e.g. for inference, evaluation or vendoring into a build pipeline.
NOT for: generic packages (use gitlab_package), container images (use gitlab_package registry_*), release attachments (use gitlab_release link_*), training jobs or experiment tracking, model publishing or versioning (not yet exposed through MCP).

Returns:
- download: {file_name, model_version_id, size, content_base64} — binary content is base64-encoded; large models can produce very large responses.
Errors: 404 (hint: project_id, model_version_id and path are model-registry-scoped; verify in the GitLab UI under Deploy → Model registry), 403 (hint: requires Reporter+ on the project and a Premium/Ultimate plan), 400 (hint: filename must match an asset attached to the version).

- download: project_id*, model_version_id*, path*, filename*. Returns base64-encoded file content.

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
Valid actions: `+validActionsString(routes)+`

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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Void actions return confirmation. Errors: 404 not found, 403 forbidden, 400 invalid params — with actionable hints.

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
Valid actions: `+validActionsString(routes)+`

Returns: JSON with resource data. Lists include pagination (page, per_page, total, next_page). Errors: 404 not found, 403 forbidden — with actionable hints.

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
Valid actions: `+validActionsString(routes)+`

When to use: browse the Catalog to find reusable components, inspect a component's versions before pinning it in `+"`include:component`"+`, or audit which Catalog resources a publisher group exposes.
NOT for: running pipelines or pipeline definitions (use gitlab_pipeline), built-in GitLab templates such as gitignore/Dockerfile/license (use gitlab_template), CI YAML linting (use gitlab_template action=lint).

Returns:
- list: {nodes: [{id, full_path, name, description, latest_version, star_count}], page_info: {end_cursor, has_next_page}}.
- get: {id, full_path, name, description, latest_version, star_count, versions: [{version, released_at, tag_name}]}.
Errors: 404 not found (hint: check full_path or id), 403 forbidden (hint: requires Premium/Ultimate or Catalog read access), 400 invalid params (hint: provide id OR full_path).

Param conventions: * = required. id format = GID (gid://gitlab/Ci::Catalog::Resource/123). full_path = namespace/project (e.g. mygroup/components/docker-build).

- list: search, scope (ALL/NAMESPACED), sort (NAME_ASC/NAME_DESC/LATEST_RELEASED_AT_ASC/LATEST_RELEASED_AT_DESC/STAR_COUNT_ASC/STAR_COUNT_DESC), first (max 100), after (cursor)
- get: id OR full_path* (exactly one)

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
Valid actions: `+validActionsString(routes)+`

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

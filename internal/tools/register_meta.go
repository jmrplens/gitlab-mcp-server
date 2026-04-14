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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollertokens"
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
// Base: 40 meta-tools (23 inline + 5 delegated + 11 sampling + 1 standalone).
// Enterprise: +19 inline = 59 meta-tools total.
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
	registerUserMeta(server, client)
	registerWikiMeta(server, client)
	registerEnvironmentMeta(server, client)
	registerDeploymentMeta(server, client)
	registerPipelineScheduleMeta(server, client)
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
	registerBranchRulesMeta(server, client)
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
	runnercontrollers.RegisterMeta(server, client)
	runnercontrollertokens.RegisterMeta(server, client)
	runnercontrollerscopes.RegisterMeta(server, client)
	samplingtools.RegisterTools(server, client)

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
	routes := map[string]actionFunc{
		"create":                   wrapAction(client, projects.Create),
		"get":                      wrapAction(client, projects.Get),
		"list":                     wrapAction(client, projects.List),
		"update":                   wrapAction(client, projects.Update),
		"delete":                   wrapAction(client, projects.Delete),
		"restore":                  wrapAction(client, projects.Restore),
		"fork":                     wrapAction(client, projects.Fork),
		"star":                     wrapAction(client, projects.Star),
		"unstar":                   wrapAction(client, projects.Unstar),
		"archive":                  wrapAction(client, projects.Archive),
		"unarchive":                wrapAction(client, projects.Unarchive),
		"transfer":                 wrapAction(client, projects.Transfer),
		"list_forks":               wrapAction(client, projects.ListForks),
		"languages":                wrapAction(client, projects.GetLanguages),
		"hook_list":                wrapAction(client, projects.ListHooks),
		"hook_get":                 wrapAction(client, projects.GetHook),
		"hook_add":                 wrapAction(client, projects.AddHook),
		"hook_edit":                wrapAction(client, projects.EditHook),
		"hook_delete":              wrapVoidAction(client, projects.DeleteHook),
		"hook_test":                wrapAction(client, projects.TriggerTestHook),
		"list_user_projects":       wrapAction(client, projects.ListUserProjects),
		"list_users":               wrapAction(client, projects.ListProjectUsers),
		"list_groups":              wrapAction(client, projects.ListProjectGroups),
		"list_starrers":            wrapAction(client, projects.ListProjectStarrers),
		"share_with_group":         wrapAction(client, projects.ShareProjectWithGroup),
		"delete_shared_group":      wrapVoidAction(client, projects.DeleteSharedProjectFromGroup),
		"list_invited_groups":      wrapAction(client, projects.ListInvitedGroups),
		"list_user_contributed":    wrapAction(client, projects.ListUserContributedProjects),
		"list_user_starred":        wrapAction(client, projects.ListUserStarredProjects),
		"members":                  wrapAction(client, members.List),
		"member_get":               wrapAction(client, members.Get),
		"member_inherited":         wrapAction(client, members.GetInherited),
		"member_add":               wrapAction(client, members.Add),
		"member_edit":              wrapAction(client, members.Edit),
		"member_delete":            wrapVoidAction(client, members.Delete),
		"upload":                   wrapActionWithRequest(client, uploads.Upload),
		"upload_list":              wrapAction(client, uploads.List),
		"upload_delete":            wrapVoidAction(client, uploads.Delete),
		"label_list":               wrapAction(client, labels.List),
		"label_get":                wrapAction(client, labels.Get),
		"label_create":             wrapAction(client, labels.Create),
		"label_update":             wrapAction(client, labels.Update),
		"label_delete":             wrapVoidAction(client, labels.Delete),
		"label_subscribe":          wrapAction(client, labels.Subscribe),
		"label_unsubscribe":        wrapVoidAction(client, labels.Unsubscribe),
		"label_promote":            wrapVoidAction(client, labels.Promote),
		"milestone_list":           wrapAction(client, milestones.List),
		"milestone_get":            wrapAction(client, milestones.Get),
		"milestone_create":         wrapAction(client, milestones.Create),
		"milestone_update":         wrapAction(client, milestones.Update),
		"milestone_delete":         wrapVoidAction(client, milestones.Delete),
		"milestone_issues":         wrapAction(client, milestones.GetIssues),
		"milestone_merge_requests": wrapAction(client, milestones.GetMergeRequests),
		"integration_list":         wrapAction(client, integrations.List),
		"integration_get":          wrapAction(client, integrations.Get),
		"integration_delete":       wrapVoidAction(client, integrations.Delete),
		"integration_set_jira":     wrapAction(client, integrations.SetJira),
		"badge_list":               wrapAction(client, badges.ListProject),
		"badge_get":                wrapAction(client, badges.GetProject),
		"badge_add":                wrapAction(client, badges.AddProject),
		"badge_edit":               wrapAction(client, badges.EditProject),
		"badge_delete":             wrapVoidAction(client, badges.DeleteProject),
		"badge_preview":            wrapAction(client, badges.PreviewProject),
		"board_list":               wrapAction(client, boards.ListBoards),
		"board_get":                wrapAction(client, boards.GetBoard),
		"board_create":             wrapAction(client, boards.CreateBoard),
		"board_update":             wrapAction(client, boards.UpdateBoard),
		"board_delete":             wrapVoidAction(client, boards.DeleteBoard),
		"board_list_list":          wrapAction(client, boards.ListBoardLists),
		"board_list_get":           wrapAction(client, boards.GetBoardList),
		"board_list_create":        wrapAction(client, boards.CreateBoardList),
		"board_list_update":        wrapAction(client, boards.UpdateBoardList),
		"board_list_delete":        wrapVoidAction(client, boards.DeleteBoardList),
		"export_schedule":          wrapAction(client, projectimportexport.ScheduleExport),
		"export_status":            wrapAction(client, projectimportexport.GetExportStatus),
		"export_download":          wrapAction(client, projectimportexport.ExportDownload),
		"import_from_file":         wrapAction(client, projectimportexport.ImportFromFile),
		"import_status":            wrapAction(client, projectimportexport.GetImportStatus),
		"statistics_get":           wrapAction(client, projectstatistics.Get),
		"pages_get":                wrapAction(client, pages.GetPages),
		"pages_update":             wrapAction(client, pages.UpdatePages),
		"pages_unpublish":          wrapVoidAction(client, pages.UnpublishPages),
		"pages_domain_list_all":    wrapAction(client, pages.ListAllDomains),
		"pages_domain_list":        wrapAction(client, pages.ListDomains),
		"pages_domain_get":         wrapAction(client, pages.GetDomain),
		"pages_domain_create":      wrapAction(client, pages.CreateDomain),
		"pages_domain_update":      wrapAction(client, pages.UpdateDomain),
		"pages_domain_delete":      wrapVoidAction(client, pages.DeleteDomain),

		// Extended project operations
		"hook_set_custom_header":    wrapVoidAction(client, projects.SetCustomHeader),
		"hook_delete_custom_header": wrapVoidAction(client, projects.DeleteCustomHeader),
		"hook_set_url_variable":     wrapVoidAction(client, projects.SetWebhookURLVariable),
		"hook_delete_url_variable":  wrapVoidAction(client, projects.DeleteWebhookURLVariable),
		"create_fork_relation":      wrapAction(client, projects.CreateForkRelation),
		"delete_fork_relation":      wrapVoidAction(client, projects.DeleteForkRelation),
		"upload_avatar":             wrapAction(client, projects.UploadAvatar),
		"download_avatar":           wrapAction(client, projects.DownloadAvatar),
		"approval_config_get":       wrapAction(client, projects.GetApprovalConfig),
		"approval_config_change":    wrapAction(client, projects.ChangeApprovalConfig),
		"approval_rule_list":        wrapAction(client, projects.ListApprovalRules),
		"approval_rule_get":         wrapAction(client, projects.GetApprovalRule),
		"approval_rule_create":      wrapAction(client, projects.CreateApprovalRule),
		"approval_rule_update":      wrapAction(client, projects.UpdateApprovalRule),
		"approval_rule_delete":      wrapVoidAction(client, projects.DeleteApprovalRule),
		"pull_mirror_get":           wrapAction(client, projects.GetPullMirror),
		"pull_mirror_configure":     wrapAction(client, projects.ConfigurePullMirror),
		"start_mirroring":           wrapVoidAction(client, projects.StartMirroring),
		"start_housekeeping":        wrapVoidAction(client, projects.StartHousekeeping),
		"repository_storage_get":    wrapAction(client, projects.GetRepositoryStorage),
		"create_for_user":           wrapAction(client, projects.CreateForUser),
	}

	if enterprise {
		routes["push_rule_get"] = wrapAction(client, projects.GetPushRules)
		routes["push_rule_add"] = wrapAction(client, projects.AddPushRule)
		routes["push_rule_edit"] = wrapAction(client, projects.EditPushRule)
		routes["push_rule_delete"] = wrapVoidAction(client, projects.DeletePushRule)
		routes["mirror_list"] = wrapAction(client, projectmirrors.List)
		routes["mirror_get"] = wrapAction(client, projectmirrors.Get)
		routes["mirror_get_public_key"] = wrapAction(client, projectmirrors.GetPublicKey)
		routes["mirror_add"] = wrapAction(client, projectmirrors.Add)
		routes["mirror_edit"] = wrapAction(client, projectmirrors.Edit)
		routes["mirror_delete"] = wrapVoidAction(client, projectmirrors.Delete)
		routes["mirror_force_push"] = wrapVoidAction(client, projectmirrors.ForcePushUpdate)
		routes["security_settings_get"] = wrapAction(client, securitysettings.GetProject)
		routes["security_settings_update"] = wrapAction(client, securitysettings.UpdateProject)
	}

	desc := `Manage GitLab projects, members, labels, milestones, webhooks, badges, boards, integrations, and Pages. Use 'action' to specify the operation and 'params' for action-specific parameters.

Project CRUD:
- create: Create a new project. Params: name (required), namespace_id, description, visibility (private/internal/public), initialize_with_readme, default_branch, path, topics ([]string), merge_method (merge/rebase_merge/ff), squash_option (never/always/default_on/default_off), only_allow_merge_if_pipeline_succeeds, only_allow_merge_if_all_discussions_are_resolved, issues_enabled (bool), merge_requests_enabled (bool), wiki_enabled (bool), jobs_enabled (bool), lfs_enabled (bool), request_access_enabled (bool), ci_config_path, allow_merge_on_skipped_pipeline (bool), remove_source_branch_after_merge (bool), autoclose_referenced_issues (bool)
- get: Get project details. Params: project_id (required, numeric ID or URL-encoded path like 'group/repo')
- list: List accessible projects. Params: owned (bool), search, visibility, archived (bool), order_by, sort, topic, simple (bool), min_access_level (int), last_activity_after (ISO 8601), last_activity_before (ISO 8601), starred (bool), membership (bool), with_issues_enabled (bool), with_merge_requests_enabled (bool), search_namespaces (bool), statistics (bool), include_pending_delete (bool, include projects marked for deletion), include_hidden (bool), page, per_page
- update: Update project settings. Params: project_id (required), name, description, visibility, default_branch, merge_method, topics, squash_option, only_allow_merge_if_pipeline_succeeds (bool), only_allow_merge_if_all_discussions_are_resolved (bool), issues_enabled (bool), merge_requests_enabled (bool), wiki_enabled (bool), jobs_enabled (bool), ci_config_path, allow_merge_on_skipped_pipeline (bool), remove_source_branch_after_merge (bool), autoclose_referenced_issues (bool), merge_commit_template, squash_commit_template, merge_pipelines_enabled (bool), merge_trains_enabled (bool), resolve_outdated_diff_discussions (bool), approvals_before_merge (int)
- delete: Delete a project. On instances with delayed deletion, the project is marked for deletion rather than removed immediately. Set permanently_remove=true with full_path to bypass delayed deletion. Params: project_id (required), permanently_remove (bool), full_path (string, required when permanently_remove=true)
- restore: Restore a project that was marked/scheduled for deletion. Params: project_id (required)

Project actions:
- fork: Fork a project. Params: project_id (required), name, path, namespace_id, namespace_path, description, visibility, branches, mr_default_target_self (bool)
- star: Star a project. Params: project_id (required)
- unstar: Unstar a project. Params: project_id (required)
- archive: Archive a project (read-only). Params: project_id (required)
- unarchive: Unarchive a project. Params: project_id (required)
- transfer: Transfer project to another namespace. Params: project_id (required), namespace (required, ID or path)
- list_forks: List project forks. Params: project_id (required), owned (bool), search, visibility, order_by, sort, page, per_page
- create_fork_relation: Create a fork relation between two projects. Params: project_id (required), forked_from_id (required)
- delete_fork_relation: Remove the fork relation from a project. Params: project_id (required)
- languages: List programming languages with percentages. Params: project_id (required)

Webhooks:
- hook_list: List project webhooks. Params: project_id (required), page, per_page
- hook_get: Get project webhook details. Params: project_id (required), hook_id (required)
- hook_add: Add a webhook to a project. Params: project_id (required), url (required), name, description, token, push_events (bool), push_events_branch_filter, issues_events (bool), confidential_issues_events (bool), merge_requests_events (bool), tag_push_events (bool), note_events (bool), confidential_note_events (bool), job_events (bool), pipeline_events (bool), wiki_page_events (bool), deployment_events (bool), releases_events (bool), emoji_events (bool), resource_access_token_events (bool), enable_ssl_verification (bool), custom_webhook_template, branch_filter_strategy
- hook_edit: Edit a project webhook. Params: project_id (required), hook_id (required), url, name, description, token, push_events (bool), and all event booleans from hook_add
- hook_delete: Delete a project webhook. Params: project_id (required), hook_id (required)
- hook_test: Trigger a test event for a webhook. Params: project_id (required), hook_id (required), event (required, e.g. push_events)
- hook_set_custom_header: Set a custom header on a webhook. Params: project_id (required), hook_id (required), key (required), value (required)
- hook_delete_custom_header: Delete a custom header from a webhook. Params: project_id (required), hook_id (required), key (required)
- hook_set_url_variable: Set a URL variable on a webhook. Params: project_id (required), hook_id (required), key (required), value (required)
- hook_delete_url_variable: Delete a URL variable from a webhook. Params: project_id (required), hook_id (required), key (required)

Users and groups:
- list_user_projects: List projects owned by a specific user. Params: user_id (required, ID or username), search, visibility, archived (bool), order_by, sort, simple (bool), page, per_page
- list_users: List users who are members of a project. Params: project_id (required), search (name or username), page, per_page
- list_groups: List ancestor groups of a project. Params: project_id (required), search, with_shared (bool), shared_visible_only (bool), skip_groups ([]int64), shared_min_access_level (int), page, per_page
- list_starrers: List users who starred a project. Params: project_id (required), search (name or username), page, per_page
- share_with_group: Share a project with a group. Params: project_id (required), group_id (required), group_access (required, 10=Guest/20=Reporter/30=Developer/40=Maintainer), expires_at (YYYY-MM-DD)
- delete_shared_group: Remove a shared group from a project. Params: project_id (required), group_id (required)
- list_invited_groups: List groups invited to a project. Params: project_id (required), search, min_access_level (int), page, per_page
- list_user_contributed: List projects a user has contributed to. Params: user_id (required), search, visibility, archived (bool), order_by, sort, simple (bool), page, per_page
- list_user_starred: List projects a user has starred. Params: user_id (required), search, visibility, archived (bool), order_by, sort, simple (bool), page, per_page

Members:
- members: List all project members (including inherited). Params: project_id (required), query (filter by name/username), page, per_page
- member_get: Get a specific project member by user ID. Params: project_id (required), user_id (required)
- member_inherited: Get a project member including inherited membership. Params: project_id (required), user_id (required)
- member_add: Add a project member. Params: project_id (required), user_id or username (required), access_level (required, 10=Guest/20=Reporter/30=Developer/40=Maintainer/50=Owner), expires_at (YYYY-MM-DD), member_role_id
- member_edit: Edit a project member. Params: project_id (required), user_id (required), access_level (required), expires_at, member_role_id
- member_delete: Remove a project member. Params: project_id (required), user_id (required)

Uploads:
- upload: Upload a file to the project. Returns a Markdown embed string. Provide either file_path (absolute local path) or content_base64 (base64-encoded), not both. Params: project_id (required), filename (required), file_path or content_base64 (one required)
- upload_list: List all markdown uploads for a project. Params: project_id (required)
- upload_delete: Delete a markdown upload by ID. Params: project_id (required), upload_id (required)

Labels:
- label_list: List all project labels. Params: project_id (required), search, with_counts (bool), include_ancestor_groups (bool), page, per_page
- label_get: Get label details. Params: project_id (required), label_id (required, ID or name)
- label_create: Create a label. Params: project_id (required), name (required), color (required, hex), description, priority (int)
- label_update: Update a label. Params: project_id (required), label_id (required), new_name, color, description, priority
- label_delete: Delete a label. Params: project_id (required), label_id (required)
- label_subscribe: Subscribe to a label. Params: project_id (required), label_id (required)
- label_unsubscribe: Unsubscribe from a label. Params: project_id (required), label_id (required)
- label_promote: Promote a project label to group label. Params: project_id (required), label_id (required)

Milestones:
- milestone_list: List project milestones. Params: project_id (required), state (active/closed), title, search, include_ancestors (bool), page, per_page
- milestone_get: Get a milestone by IID. Params: project_id (required), milestone_iid (required)
- milestone_create: Create a milestone. Params: project_id (required), title (required), description, start_date (YYYY-MM-DD), due_date (YYYY-MM-DD)
- milestone_update: Update a milestone. Params: project_id (required), milestone_iid (required), title, description, start_date (YYYY-MM-DD), due_date (YYYY-MM-DD), state_event (activate/close)
- milestone_delete: Delete a milestone. Params: project_id (required), milestone_iid (required)
- milestone_issues: List issues assigned to a milestone. Params: project_id (required), milestone_iid (required), page, per_page
- milestone_merge_requests: List merge requests assigned to a milestone. Params: project_id (required), milestone_iid (required), page, per_page

Integrations:
- integration_list: List all project integrations (services). Params: project_id (required)
- integration_get: Get a specific integration by slug. Params: project_id (required), slug (required, e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, datadog, jenkins, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, drone-ci, github, harbor, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands)
- integration_delete: Delete/disable a project integration. Params: project_id (required), slug (required)
- integration_set_jira: Configure Jira integration. Params: project_id (required), url (required), username, password, active (bool), api_url, jira_auth_type, jira_issue_prefix, jira_issue_regex, jira_issue_transition_automatic (bool), jira_issue_transition_id, commit_events (bool), merge_requests_events (bool), comment_on_event_enabled (bool), issues_enabled (bool), project_keys ([]string), use_inherited_settings (bool)

Badges:
- badge_list: List project badges. Params: project_id (required), name, page, per_page
- badge_get: Get a project badge. Params: project_id (required), badge_id (required)
- badge_add: Add a badge to a project. Params: project_id (required), link_url (required), image_url (required), name
- badge_edit: Edit a project badge. Params: project_id (required), badge_id (required), link_url, image_url, name
- badge_delete: Delete a project badge. Params: project_id (required), badge_id (required)
- badge_preview: Preview how a project badge renders. Params: project_id (required), link_url (required), image_url (required)

Boards:
- board_list: List project issue boards. Params: project_id (required), page, per_page
- board_get: Get a project issue board. Params: project_id (required), board_id (required)
- board_create: Create a project issue board. Params: project_id (required), name (required)
- board_update: Update a project issue board. Params: project_id (required), board_id (required), name, assignee_id, milestone_id, labels (comma-separated string), weight, hide_backlog_list (bool), hide_closed_list (bool)
- board_delete: Delete a project issue board. Params: project_id (required), board_id (required)
- board_list_list: List columns (lists) within a board. Params: project_id (required), board_id (required), page, per_page
- board_list_get: Get a single board column (list). Params: project_id (required), board_id (required), list_id (required)
- board_list_create: Create a board column (list). Params: project_id (required), board_id (required), label_id
- board_list_update: Update a board column (list) position. Params: project_id (required), board_id (required), list_id (required), position
- board_list_delete: Delete a board column (list). Params: project_id (required), board_id (required), list_id (required)

Import/Export:
- export_schedule: Schedule a project export. Params: project_id (required)
- export_status: Get export status. Params: project_id (required)
- export_download: Download project export. Params: project_id (required)
- import_from_file: Import project from archive. Params: file_path or content_base64 (one required), namespace, name, path, overwrite
- import_status: Get import status. Params: project_id (required)

Statistics and Pages:
- statistics_get: Get project statistics. Params: project_id (required)
- pages_get: Get Pages settings. Params: project_id (required)
- pages_update: Update Pages settings. Params: project_id (required), pages_https_only, pages_access_level
- pages_unpublish: Unpublish Pages. Params: project_id (required)
- pages_domain_list_all: List all Pages domains (admin). Params: page, per_page
- pages_domain_list: List Pages domains for a project. Params: project_id (required), page, per_page
- pages_domain_get: Get a Pages domain. Params: project_id (required), domain (required)
- pages_domain_create: Create a Pages domain. Params: project_id (required), domain (required), certificate, key
- pages_domain_update: Update a Pages domain. Params: project_id (required), domain (required), certificate, key
- pages_domain_delete: Delete a Pages domain. Params: project_id (required), domain (required)

Avatars:
- upload_avatar: Upload or replace the project avatar. Params: project_id (required), filename (required), content_base64 (required, base64-encoded image)
- download_avatar: Download project avatar as base64. Params: project_id (required)

Approval configuration:
- approval_config_get: Get project approval settings. Params: project_id (required)
- approval_config_change: Update project approval settings. Params: project_id (required), approvals_before_merge (int), reset_approvals_on_push (bool), disable_overriding_approvers_per_merge_request (bool), merge_requests_author_approval (bool), merge_requests_disable_committers_approval (bool), require_password_to_approve (bool), selective_code_owner_removals (bool)
- approval_rule_list: List project approval rules. Params: project_id (required), page, per_page
- approval_rule_get: Get an approval rule. Params: project_id (required), rule_id (required)
- approval_rule_create: Create an approval rule. Params: project_id (required), name (required), approvals_required (required), rule_type, user_ids ([]int64), group_ids ([]int64), protected_branch_ids ([]int64), usernames ([]string), applies_to_all_protected_branches (bool)
- approval_rule_update: Update an approval rule. Params: project_id (required), rule_id (required), name, approvals_required (int), user_ids ([]int64), group_ids ([]int64), protected_branch_ids ([]int64), usernames ([]string), applies_to_all_protected_branches (bool)
- approval_rule_delete: Delete an approval rule. Params: project_id (required), rule_id (required)

Pull mirroring:
- pull_mirror_get: Get pull mirror configuration. Params: project_id (required)
- pull_mirror_configure: Configure pull mirroring. Params: project_id (required), enabled (bool), url, auth_user, auth_password, mirror_branch_regex, mirror_trigger_builds (bool), only_mirror_protected_branches (bool), mirror_overwrites_diverged_branches (bool)
- start_mirroring: Trigger an immediate mirror pull. Params: project_id (required)

Maintenance:
- start_housekeeping: Run git gc/repack optimization. Params: project_id (required)
- repository_storage_get: Get repository storage info. Params: project_id (required)

Admin:
- create_for_user: Create a project for another user (admin). Params: user_id (required), name (required), path, namespace_id, description, visibility, initialize_with_readme (bool), default_branch, topics ([]string), issues_enabled (bool), merge_requests_enabled (bool), wiki_enabled (bool), jobs_enabled (bool)`

	if enterprise {
		desc += `

Push Rules (Premium+ — requires GITLAB_ENTERPRISE=true):
- push_rule_get: Get push rule configuration. Params: project_id (required)
- push_rule_add: Add push rules. Params: project_id (required), commit_message_regex, commit_message_negative_regex, branch_name_regex, author_email_regex, file_name_regex, max_file_size (int), deny_delete_tag (bool), member_check (bool), prevent_secrets (bool), commit_committer_check (bool), commit_committer_name_check (bool), reject_unsigned_commits (bool), reject_non_dco_commits (bool)
- push_rule_edit: Edit push rules. Params: project_id (required), same fields as push_rule_add (all optional)
- push_rule_delete: Delete push rules from a project. Params: project_id (required)

Push Mirrors (Premium+ — requires GITLAB_ENTERPRISE=true):
- mirror_list: List all remote push mirrors. Params: project_id (required), page, per_page
- mirror_get: Get a single remote mirror. Params: project_id (required), mirror_id (required)
- mirror_get_public_key: Get SSH public key for a mirror. Params: project_id (required), mirror_id (required)
- mirror_add: Create a new remote mirror. Params: project_id (required), url (required), enabled (bool), keep_divergent_refs (bool), only_protected_branches (bool), mirror_branch_regex, auth_method (password/ssh_public_key)
- mirror_edit: Update a remote mirror. Params: project_id (required), mirror_id (required), enabled (bool), keep_divergent_refs (bool), only_protected_branches (bool), mirror_branch_regex, auth_method
- mirror_delete: Delete a remote mirror. Params: project_id (required), mirror_id (required)
- mirror_force_push: Trigger an immediate mirror update. Params: project_id (required), mirror_id (required)

Security Settings (Ultimate — requires GITLAB_ENTERPRISE=true):
- security_settings_get: Get project security settings. Params: project_id (required)
- security_settings_update: Update project secret push protection. Params: project_id (required), secret_push_protection_enabled (required)`
	}

	addMetaTool(server, "gitlab_project", desc, routes, metaAnnotations, toolutil.IconProject)
}

// registerBranchMeta registers the gitlab_branch meta-tool with actions:
// create, get, list, delete, protect, unprotect, list_protected, get_protected, and update_protected.
func registerBranchMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"create":           wrapAction(client, branches.Create),
		"get":              wrapAction(client, branches.Get),
		"list":             wrapAction(client, branches.List),
		"delete":           wrapVoidAction(client, branches.Delete),
		"delete_merged":    wrapVoidAction(client, branches.DeleteMerged),
		"protect":          wrapAction(client, branches.Protect),
		"unprotect":        wrapAction(client, branches.Unprotect),
		"list_protected":   wrapAction(client, branches.ProtectedList),
		"get_protected":    wrapAction(client, branches.ProtectedGet),
		"update_protected": wrapAction(client, branches.ProtectedUpdate),
	}

	addMetaTool(server, "gitlab_branch", `Manage Git branches in GitLab projects. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- create: Create a new branch from a ref. Params: project_id (required), branch_name (required), ref (required, branch/tag/SHA)
- get: Get branch details. Params: project_id (required), branch_name (required)
- list: List branches with optional search. Params: project_id (required), search, page, per_page
- delete: Delete a branch. Cannot delete default or protected branches. Params: project_id (required), branch_name (required)
- delete_merged: Delete all branches merged into the default branch. Default and protected branches are never deleted. Params: project_id (required)
- protect: Protect a branch with access levels. Params: project_id (required), branch_name (required), push_access_level (0/30/40), merge_access_level (0/30/40), allow_force_push
- unprotect: Remove branch protection. Params: project_id (required), branch_name (required)
- list_protected: List all protected branches. Params: project_id (required), page, per_page
- get_protected: Get details of a single protected branch. Params: project_id (required), branch_name (required)
- update_protected: Update protected branch settings. Params: project_id (required), branch_name (required), allow_force_push, code_owner_approval_required`, routes, metaAnnotations, toolutil.IconBranch)
}

// registerTagMeta registers the gitlab_tag meta-tool with actions:
// create, get, list, delete, get_signature, list_protected, get_protected,
// protect, and unprotect.
func registerTagMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"create":         wrapAction(client, tags.Create),
		"get":            wrapAction(client, tags.Get),
		"list":           wrapAction(client, tags.List),
		"delete":         wrapVoidAction(client, tags.Delete),
		"get_signature":  wrapAction(client, tags.GetSignature),
		"list_protected": wrapAction(client, tags.ListProtectedTags),
		"get_protected":  wrapAction(client, tags.GetProtectedTag),
		"protect":        wrapAction(client, tags.ProtectTag),
		"unprotect":      wrapVoidAction(client, tags.UnprotectTag),
	}

	addMetaTool(server, "gitlab_tag", `Manage Git tags and protected tags in GitLab projects. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- create: Create a tag from a ref. Params: project_id (required), tag_name (required), ref (required, branch/tag/SHA), message (annotation)
- get: Get tag details. Params: project_id (required), tag_name (required)
- list: List tags with optional search and ordering. Params: project_id (required), search, order_by (name/updated/version), sort (asc/desc), page, per_page
- delete: Delete a tag (and associated release). Params: project_id (required), tag_name (required)
- get_signature: Get X.509 signature of a tag. Params: project_id (required), tag_name (required)
- list_protected: List protected tags. Params: project_id (required), page, per_page
- get_protected: Get a protected tag. Params: project_id (required), tag_name (required)
- protect: Protect a tag or wildcard pattern. Params: project_id (required), tag_name (required, tag name or wildcard e.g. 'v*'), create_access_level (0/30/40), allowed_to_create (array of {user_id, group_id, deploy_key_id, access_level})
- unprotect: Remove tag protection. Params: project_id (required), tag_name (required)`, routes, metaAnnotations, toolutil.IconTag)
}

// registerReleaseMeta registers the gitlab_release meta-tool with actions:
// create, get, get_latest, list, update, delete, link_create, link_create_batch,
// link_get, link_list, link_update, and link_delete.
func registerReleaseMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"create":            wrapAction(client, releases.Create),
		"get":               wrapAction(client, releases.Get),
		"get_latest":        wrapAction(client, releases.GetLatest),
		"list":              wrapAction(client, releases.List),
		"update":            wrapAction(client, releases.Update),
		"delete":            wrapAction(client, releases.Delete),
		"link_create":       wrapAction(client, releaselinks.Create),
		"link_create_batch": wrapAction(client, releaselinks.CreateBatch),
		"link_get":          wrapAction(client, releaselinks.Get),
		"link_list":         wrapAction(client, releaselinks.List),
		"link_update":       wrapAction(client, releaselinks.Update),
		"link_delete":       wrapAction(client, releaselinks.Delete),
	}

	addMetaTool(server, "gitlab_release", `Manage GitLab releases and their asset links. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- create: Create a release for an existing tag. Params: project_id (required), tag_name (required), name, description (Markdown), released_at (ISO 8601)
- get: Get release details. Params: project_id (required), tag_name (required)
- get_latest: Get the latest release. Params: project_id (required)
- list: List all releases. Params: project_id (required), order_by (released_at/created_at), sort (asc/desc), page, per_page
- update: Update release metadata. Params: project_id (required), tag_name (required), name, description, released_at, milestones ([]string, milestone titles)
- delete: Delete a release (tag is preserved). Params: project_id (required), tag_name (required)
- link_create: Add a single asset link to a release. Params: project_id (required), tag_name (required), name (required), url (required), link_type (runbook/package/image/other)
- link_create_batch: Add multiple asset links in one call — use this instead of calling link_create repeatedly. Params: project_id (required), tag_name (required), links (required, array of {name, url, link_type})
- link_get: Get release link details. Params: project_id (required), tag_name (required), link_id (required)
- link_list: List release asset links. Params: project_id (required), tag_name (required), page, per_page
- link_update: Update a release asset link. Params: project_id (required), tag_name (required), link_id (required), name, url, filepath, direct_asset_path, link_type
- link_delete: Remove an asset link. Params: project_id (required), tag_name (required), link_id (required)`, routes, metaAnnotations, toolutil.IconRelease)
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
	routes := map[string]actionFunc{
		"create":                           wrapAction(client, mergerequests.Create),
		"get":                              wrapAction(client, mergerequests.Get),
		"list":                             wrapAction(client, mergerequests.List),
		"list_global":                      wrapAction(client, mergerequests.ListGlobal),
		"list_group":                       wrapAction(client, mergerequests.ListGroup),
		"update":                           wrapAction(client, mergerequests.Update),
		"merge":                            wrapAction(client, mergerequests.Merge),
		"approve":                          wrapAction(client, mergerequests.Approve),
		"unapprove":                        wrapVoidAction(client, mergerequests.Unapprove),
		"commits":                          wrapAction(client, mergerequests.Commits),
		"pipelines":                        wrapAction(client, mergerequests.Pipelines),
		"delete":                           wrapVoidAction(client, mergerequests.Delete),
		"rebase":                           wrapAction(client, mergerequests.Rebase),
		"participants":                     wrapAction(client, mergerequests.Participants),
		"reviewers":                        wrapAction(client, mergerequests.Reviewers),
		"create_pipeline":                  wrapAction(client, mergerequests.CreatePipeline),
		"issues_closed":                    wrapAction(client, mergerequests.IssuesClosed),
		"cancel_auto_merge":                wrapAction(client, mergerequests.CancelAutoMerge),
		"approval_state":                   wrapAction(client, mrapprovals.State),
		"approval_rules":                   wrapAction(client, mrapprovals.Rules),
		"approval_config":                  wrapAction(client, mrapprovals.Config),
		"approval_reset":                   wrapVoidAction(client, mrapprovals.Reset),
		"approval_rule_create":             wrapAction(client, mrapprovals.CreateRule),
		"approval_rule_update":             wrapAction(client, mrapprovals.UpdateRule),
		"approval_rule_delete":             wrapVoidAction(client, mrapprovals.DeleteRule),
		"approval_settings_group_get":      wrapAction(client, mrapprovalsettings.GetGroupSettings),
		"approval_settings_group_update":   wrapAction(client, mrapprovalsettings.UpdateGroupSettings),
		"approval_settings_project_get":    wrapAction(client, mrapprovalsettings.GetProjectSettings),
		"approval_settings_project_update": wrapAction(client, mrapprovalsettings.UpdateProjectSettings),
		"subscribe":                        wrapAction(client, mergerequests.Subscribe),
		"unsubscribe":                      wrapAction(client, mergerequests.Unsubscribe),
		"time_estimate_set":                wrapAction(client, mergerequests.SetTimeEstimate),
		"time_estimate_reset":              wrapAction(client, mergerequests.ResetTimeEstimate),
		"spent_time_add":                   wrapAction(client, mergerequests.AddSpentTime),
		"spent_time_reset":                 wrapAction(client, mergerequests.ResetSpentTime),
		"time_stats":                       wrapAction(client, mergerequests.GetTimeStats),
		"context_commits_list":             wrapAction(client, mrcontextcommits.List),
		"context_commits_create":           wrapAction(client, mrcontextcommits.Create),
		"context_commits_delete":           wrapVoidAction(client, mrcontextcommits.Delete),
		"emoji_mr_list":                    wrapAction(client, awardemoji.ListMRAwardEmoji),
		"emoji_mr_get":                     wrapAction(client, awardemoji.GetMRAwardEmoji),
		"emoji_mr_create":                  wrapAction(client, awardemoji.CreateMRAwardEmoji),
		"emoji_mr_delete":                  wrapVoidAction(client, awardemoji.DeleteMRAwardEmoji),
		"emoji_mr_note_list":               wrapAction(client, awardemoji.ListMRNoteAwardEmoji),
		"emoji_mr_note_get":                wrapAction(client, awardemoji.GetMRNoteAwardEmoji),
		"emoji_mr_note_create":             wrapAction(client, awardemoji.CreateMRNoteAwardEmoji),
		"emoji_mr_note_delete":             wrapVoidAction(client, awardemoji.DeleteMRNoteAwardEmoji),
		"event_mr_label_list":              wrapAction(client, resourceevents.ListMRLabelEvents),
		"event_mr_label_get":               wrapAction(client, resourceevents.GetMRLabelEvent),
		"event_mr_milestone_list":          wrapAction(client, resourceevents.ListMRMilestoneEvents),
		"event_mr_milestone_get":           wrapAction(client, resourceevents.GetMRMilestoneEvent),
		"event_mr_state_list":              wrapAction(client, resourceevents.ListMRStateEvents),
		"event_mr_state_get":               wrapAction(client, resourceevents.GetMRStateEvent),
	}

	addMetaTool(server, "gitlab_merge_request", `Manage GitLab merge requests. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- create: Create a merge request. Params: project_id (required), source_branch (required), target_branch (required — if not specified by the user, retrieve the project default branch via action 'get' on gitlab_project and use its default_branch value; do NOT assume 'main'), title (required), description, assignee_id (single user ID), assignee_ids (multiple user IDs), reviewer_ids, labels (comma-separated string), milestone_id, remove_source_branch (bool), squash (bool), allow_collaboration (bool), target_project_id (for fork MRs)
- get: Get MR details by IID. Params: project_id (required), mr_iid (required)
- list: List merge requests in a project. Params: project_id (required), state (opened/closed/merged/all), labels, not_labels, milestone, scope, search, source_branch, target_branch, author_username, draft (bool), iids ([]int), created_after/created_before (ISO 8601), updated_after/updated_before (ISO 8601), order_by (created_at/updated_at/title), sort (asc/desc), page, per_page
- list_global: List merge requests across all projects. Params: state, labels, not_labels, milestone, scope, search, source_branch, target_branch, author_username, reviewer_username, draft (bool), created_after/created_before, updated_after/updated_before, order_by, sort, page, per_page
- list_group: List merge requests in a group. Params: group_id (required), state, labels, not_labels, milestone, scope, search, source_branch, target_branch, author_username, reviewer_username, draft (bool), created_after/created_before, updated_after/updated_before, order_by, sort, page, per_page
- update: Update MR metadata. Params: project_id (required), mr_iid (required), title, description, target_branch, assignee_id (single user ID), assignee_ids (multiple user IDs), reviewer_ids, labels (comma-separated string), add_labels (comma-separated string), remove_labels (comma-separated string), milestone_id, remove_source_branch (bool), squash (bool), discussion_locked (bool), allow_collaboration (bool), state_event (close/reopen)
- merge: Merge an accepted MR. The server auto-detects enforced project settings (squash, source branch deletion) — do NOT set squash or should_remove_source_branch unless the user explicitly asks. Params: project_id (required), mr_iid (required), merge_commit_message, squash (bool, auto-detected), should_remove_source_branch (bool, auto-detected), auto_merge (bool), sha (safety check), squash_commit_message
- approve: Approve a merge request. Params: project_id (required), mr_iid (required)
- unapprove: Remove your approval. Params: project_id (required), mr_iid (required)
- commits: List all commits in an MR. Params: project_id (required), mr_iid (required), page, per_page
- pipelines: List all pipelines for an MR. Params: project_id (required), mr_iid (required), page, per_page
- delete: PERMANENTLY delete an MR. Params: project_id (required), mr_iid (required)
- rebase: Rebase MR source branch against target. Params: project_id (required), mr_iid (required), skip_ci (bool)
- participants: List MR participants. Params: project_id (required), mr_iid (required)
- reviewers: List MR reviewers with review state. Params: project_id (required), mr_iid (required)
- create_pipeline: Create a new pipeline for an MR. Params: project_id (required), mr_iid (required)
- issues_closed: List issues closed on merge. Params: project_id (required), mr_iid (required), page, per_page
- cancel_auto_merge: Cancel merge-when-pipeline-succeeds. Params: project_id (required), mr_iid (required)
- approval_state: Get the approval state of an MR including rule overrides and per-rule status. Params: project_id (required), mr_iid (required)
- approval_rules: List approval rules for an MR with required/approved counts and eligible approvers. Params: project_id (required), mr_iid (required)
- approval_config: Get the approval configuration including required approvals, current approvers, and user approval status. Params: project_id (required), mr_iid (required)
- approval_reset: Reset all approvals on an MR. Params: project_id (required), mr_iid (required)
- approval_rule_create: Create an approval rule. Params: project_id (required), mr_iid (required), name (required), approvals_required (required), approval_project_rule_id, user_ids ([]int), group_ids ([]int)
- approval_rule_update: Update an approval rule. Params: project_id (required), mr_iid (required), approval_rule_id (required), name, approvals_required, user_ids ([]int), group_ids ([]int)
- approval_rule_delete: Delete an approval rule. Params: project_id (required), mr_iid (required), approval_rule_id (required)
- approval_settings_group_get: Get group-level MR approval settings. Params: group_id (required)
- approval_settings_group_update: Update group-level MR approval settings. Params: group_id (required), allow_author_approval, allow_committer_approval, allow_overrides_approver_list_per_mr, retain_approvals_on_push, require_reauthentication_to_approve (all optional bool)
- approval_settings_project_get: Get project-level MR approval settings. Params: project_id (required)
- approval_settings_project_update: Update project-level MR approval settings. Params: project_id (required), allow_author_approval, allow_committer_approval, allow_overrides_approver_list_per_mr, retain_approvals_on_push, require_reauthentication_to_approve, selective_code_owner_removals (all optional bool)
- subscribe: Subscribe to MR notifications. Params: project_id (required), mr_iid (required)
- unsubscribe: Unsubscribe from MR notifications. Params: project_id (required), mr_iid (required)
- time_estimate_set: Set time estimate. Params: project_id (required), mr_iid (required), duration (required, e.g. '3h30m', '1w2d')
- time_estimate_reset: Reset time estimate to zero. Params: project_id (required), mr_iid (required)
- spent_time_add: Add spent time. Params: project_id (required), mr_iid (required), duration (required), summary (optional)
- spent_time_reset: Reset spent time to zero. Params: project_id (required), mr_iid (required)
- time_stats: Get time tracking stats (estimate and spent). Params: project_id (required), mr_iid (required)
- context_commits_list: List context commits for an MR. Params: project_id (required), mr_iid (required)
- context_commits_create: Add context commits to an MR. Params: project_id (required), mr_iid (required), commits ([]string, required)
- context_commits_delete: Remove context commits from an MR. Params: project_id (required), mr_iid (required), commits ([]string, required)
- emoji_mr_list: List award emoji on a merge request. Params: project_id (required), iid (required), page, per_page
- emoji_mr_get: Get an award emoji on a merge request. Params: project_id (required), iid (required), award_id (required)
- emoji_mr_create: Add award emoji to a merge request. Params: project_id (required), iid (required), name (required)
- emoji_mr_delete: Remove award emoji from a merge request. Params: project_id (required), iid (required), award_id (required)
- emoji_mr_note_list: List award emoji on a merge request note. Params: project_id (required), iid (required), note_id (required), page, per_page
- emoji_mr_note_get: Get an award emoji on a merge request note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- emoji_mr_note_create: Add award emoji to a merge request note. Params: project_id (required), iid (required), note_id (required), name (required)
- emoji_mr_note_delete: Delete award emoji from a merge request note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- event_mr_label_list: List label events on a merge request. Params: project_id (required), mr_iid (required), page, per_page
- event_mr_label_get: Get a label event on a merge request. Params: project_id (required), mr_iid (required), label_event_id (required)
- event_mr_milestone_list: List milestone events on a merge request. Params: project_id (required), mr_iid (required), page, per_page
- event_mr_milestone_get: Get a milestone event on a merge request. Params: project_id (required), mr_iid (required), milestone_event_id (required)
- event_mr_state_list: List state events on a merge request. Params: project_id (required), mr_iid (required), page, per_page
- event_mr_state_get: Get a state event on a merge request. Params: project_id (required), mr_iid (required), state_event_id (required)`, routes, metaAnnotations, toolutil.IconMR)
}

// registerMRReviewMeta registers the gitlab_mr_review meta-tool with actions:
// note_create, note_list, note_update, note_delete, discussion_create,
// discussion_list, discussion_get, discussion_reply, discussion_resolve,
// discussion_note_update, discussion_note_delete, changes_get,
// draft_note_list, draft_note_get, draft_note_create, draft_note_update,
// draft_note_delete, draft_note_publish, draft_note_publish_all,
// diff_versions_list, diff_version_get.
func registerMRReviewMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"note_create":            wrapAction(client, mrnotes.Create),
		"note_list":              wrapAction(client, mrnotes.List),
		"note_get":               wrapAction(client, mrnotes.GetNote),
		"note_update":            wrapAction(client, mrnotes.Update),
		"note_delete":            wrapVoidAction(client, mrnotes.Delete),
		"discussion_create":      wrapAction(client, mrdiscussions.Create),
		"discussion_list":        wrapAction(client, mrdiscussions.List),
		"discussion_get":         wrapAction(client, mrdiscussions.Get),
		"discussion_reply":       wrapAction(client, mrdiscussions.Reply),
		"discussion_resolve":     wrapAction(client, mrdiscussions.Resolve),
		"discussion_note_update": wrapAction(client, mrdiscussions.UpdateNote),
		"discussion_note_delete": wrapVoidAction(client, mrdiscussions.DeleteNote),
		"changes_get":            wrapAction(client, mrchanges.Get),
		"draft_note_list":        wrapAction(client, mrdraftnotes.List),
		"draft_note_get":         wrapAction(client, mrdraftnotes.Get),
		"draft_note_create":      wrapAction(client, mrdraftnotes.Create),
		"draft_note_update":      wrapAction(client, mrdraftnotes.Update),
		"draft_note_delete":      wrapVoidAction(client, mrdraftnotes.Delete),
		"draft_note_publish":     wrapVoidAction(client, mrdraftnotes.Publish),
		"draft_note_publish_all": wrapVoidAction(client, mrdraftnotes.PublishAll),
		"diff_versions_list":     wrapAction(client, mrchanges.ListDiffVersions),
		"diff_version_get":       wrapAction(client, mrchanges.GetDiffVersion),
	}

	addMetaTool(server, "gitlab_mr_review", `Review GitLab merge requests: notes, discussions, draft notes, file changes, and diff versions. Use 'action' to specify the operation and 'params' for action-specific parameters.

IMPORTANT — Batch review workflow: When performing a code review with multiple comments, use draft_note_create (with position for inline comments, or in_reply_to_discussion_id for replies to existing threads) for EACH comment, then call draft_note_publish_all ONCE at the end. This batches all comments and replies into a single notification instead of spamming reviewers with one notification per comment. Only use discussion_create for standalone questions that need immediate visibility.

Actions:
- note_create: Add a comment to an MR. Params: project_id (required), mr_iid (required), body (required, Markdown)
- note_list: List all MR comments. Params: project_id (required), mr_iid (required), order_by (created_at/updated_at), sort (asc/desc), page, per_page
- note_get: Get a single MR comment by note ID. Params: project_id (required), mr_iid (required), note_id (required)
- note_update: Edit a comment. Params: project_id (required), mr_iid (required), note_id (required), body (required)
- note_delete: Delete a comment. Params: project_id (required), mr_iid (required), note_id (required)
- discussion_create: Start a threaded discussion (general or inline diff). Params: project_id (required), mr_iid (required), body (required), position (optional object with base_sha, start_sha, head_sha, new_path, old_path, and EITHER new_line OR old_line — use new_line only for added/modified lines, old_line only for removed lines, both only for unchanged context lines)
- discussion_list: List all discussion threads. Params: project_id (required), mr_iid (required), page, per_page
- discussion_get: Get a single discussion thread by ID. Params: project_id (required), mr_iid (required), discussion_id (required)
- discussion_reply: Reply to a discussion thread. Params: project_id (required), mr_iid (required), discussion_id (required), body (required)
- discussion_resolve: Resolve or unresolve a discussion. Params: project_id (required), mr_iid (required), discussion_id (required), resolved (required, true/false)
- discussion_note_update: Update a note in a discussion (body and/or resolved status). Params: project_id (required), mr_iid (required), discussion_id (required), note_id (required), body (optional), resolved (optional, true/false)
- discussion_note_delete: Delete a note from a discussion. Params: project_id (required), mr_iid (required), discussion_id (required), note_id (required)
- changes_get: Get file diffs for an MR. Large diffs may be empty due to GitLab truncation — check truncated_files in response and use diff_versions_list + diff_version_get for full content. Params: project_id (required), mr_iid (required)
- draft_note_list: List all draft notes on a MR. Params: project_id (required), mr_iid (required, int), order_by, sort, page, per_page
- draft_note_get: Get a single draft note. Params: project_id (required), mr_iid (required, int), note_id (required, int)
- draft_note_create: Create a new draft note (pending review comment). Supports inline diff comments via position and replies to existing discussions via in_reply_to_discussion_id. Use resolve_discussion to resolve the thread when published. Params: project_id (required), mr_iid (required, int), note (required), commit_id, in_reply_to_discussion_id (discussion ID to reply to), resolve_discussion (bool), position (optional object with base_sha, start_sha, head_sha, new_path, old_path, and EITHER new_line OR old_line — use new_line only for added/modified lines, old_line only for removed lines, both only for unchanged context lines)
- draft_note_update: Update a draft note. Params: project_id (required), mr_iid (required, int), note_id (required, int), note, position (optional object for inline comments)
- draft_note_delete: Delete a draft note. Params: project_id (required), mr_iid (required, int), note_id (required, int)
- draft_note_publish: Publish a single draft note. Params: project_id (required), mr_iid (required, int), note_id (required, int)
- draft_note_publish_all: Publish all draft notes on a MR at once (single notification). Params: project_id (required), mr_iid (required, int)
- diff_versions_list: List all diff versions of an MR. Params: project_id (required), mr_iid (required), page, per_page
- diff_version_get: Get a single diff version with commits and file diffs. Params: project_id (required), mr_iid (required), version_id (required), unidiff (bool, optional)`, routes, metaAnnotations, toolutil.IconDiscussion)
}

// registerRepositoryMeta registers the gitlab_repository meta-tool with actions:
// tree, compare, contributors, merge_base, blob, raw_blob, archive, changelog,
// commit operations, file operations, update_submodule, and markdown_render.
func registerRepositoryMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"tree":                          wrapAction(client, repository.Tree),
		"compare":                       wrapAction(client, repository.Compare),
		"contributors":                  wrapAction(client, repository.Contributors),
		"merge_base":                    wrapAction(client, repository.MergeBase),
		"blob":                          wrapAction(client, repository.Blob),
		"raw_blob":                      wrapAction(client, repository.RawBlobContent),
		"archive":                       wrapAction(client, repository.Archive),
		"changelog_add":                 wrapAction(client, repository.AddChangelog),
		"changelog_generate":            wrapAction(client, repository.GenerateChangelogData),
		"commit_create":                 wrapAction(client, commits.Create),
		"commit_list":                   wrapAction(client, commits.List),
		"commit_get":                    wrapAction(client, commits.Get),
		"commit_diff":                   wrapAction(client, commits.Diff),
		"commit_refs":                   wrapAction(client, commits.GetRefs),
		"commit_comments":               wrapAction(client, commits.GetComments),
		"commit_comment_create":         wrapAction(client, commits.PostComment),
		"commit_statuses":               wrapAction(client, commits.GetStatuses),
		"commit_status_set":             wrapAction(client, commits.SetStatus),
		"commit_merge_requests":         wrapAction(client, commits.ListMRsByCommit),
		"commit_cherry_pick":            wrapAction(client, commits.CherryPick),
		"commit_revert":                 wrapAction(client, commits.Revert),
		"commit_signature":              wrapAction(client, commits.GetGPGSignature),
		"file_get":                      wrapAction(client, files.Get),
		"file_create":                   wrapAction(client, files.Create),
		"file_update":                   wrapAction(client, files.Update),
		"file_delete":                   wrapVoidAction(client, files.Delete),
		"file_blame":                    wrapAction(client, files.Blame),
		"file_metadata":                 wrapAction(client, files.GetMetaData),
		"file_raw":                      wrapAction(client, files.GetRaw),
		"update_submodule":              wrapAction(client, repositorysubmodules.Update),
		"list_submodules":               wrapAction(client, repositorysubmodules.List),
		"read_submodule_file":           wrapAction(client, repositorysubmodules.Read),
		"markdown_render":               wrapAction(client, markdown.Render),
		"commit_discussion_list":        wrapAction(client, commitdiscussions.List),
		"commit_discussion_get":         wrapAction(client, commitdiscussions.Get),
		"commit_discussion_create":      wrapAction(client, commitdiscussions.Create),
		"commit_discussion_add_note":    wrapAction(client, commitdiscussions.AddNote),
		"commit_discussion_update_note": wrapAction(client, commitdiscussions.UpdateNote),
		"commit_discussion_delete_note": wrapVoidAction(client, commitdiscussions.DeleteNote),
		"file_history":                  wrapAction(client, commits.List),
	}

	addMetaTool(server, "gitlab_repository", `Interact with GitLab repository content: tree, compare, blobs, contributors, archives, changelogs, commits, and files. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- tree: List files and directories at a path and ref. Params: project_id (required), path, ref, recursive (bool), page, per_page
- compare: Compare two branches, tags, or commits. Params: project_id (required), from (required), to (required), straight (bool)
- contributors: List repo contributors with commit/addition/deletion stats. Params: project_id (required), order_by (name/email/commits), sort (asc/desc), page, per_page
- merge_base: Find common ancestor of 2+ refs. Params: project_id (required), refs (required, array of 2+ branch/tag/SHA)
- blob: Get blob content by SHA (base64). Params: project_id (required), sha (required)
- raw_blob: Get raw text content of a blob by SHA. Params: project_id (required), sha (required)
- archive: Get download URL for repo archive. Params: project_id (required), sha, format (tar.gz/zip/tar.bz2), path
- changelog_add: Add changelog data to file via commit. Params: project_id (required), version (required), branch, config_file, file, from, to, message, trailer
- changelog_generate: Generate changelog notes without committing. Params: project_id (required), version (required), config_file, from, to, trailer
- commit_create: Create a commit with file actions. Params: project_id (required), branch (required), commit_message (required), actions (required, array of {action: create/update/delete/move, file_path, content, previous_path}), start_branch, author_email, author_name
- commit_list: List commits. Params: project_id (required), ref_name, since, until, path, author, with_stats (bool), page, per_page
- file_history: Alias for commit_list — list commits that modified a specific file. Params: project_id (required), path (required, file path to filter by), ref_name, since, until, page, per_page
- commit_get: Get a single commit by SHA. Params: project_id (required), sha (required)
- commit_diff: List diffs for a commit. Params: project_id (required), sha (required), page, per_page
- commit_refs: Get branches/tags a commit is pushed to. Params: project_id (required), sha (required), type (branch/tag/all)
- commit_comments: List comments on a commit. Params: project_id (required), sha (required), page, per_page
- commit_comment_create: Post a comment on a commit. Params: project_id (required), sha (required), note (required), path, line, line_type (new/old)
- commit_statuses: List pipeline statuses of a commit. Params: project_id (required), sha (required), ref, stage, name, pipeline_id, all (bool), page, per_page
- commit_status_set: Set pipeline status of a commit. Params: project_id (required), sha (required), state (required: pending/running/success/failed/canceled), ref, name, context, target_url, description, coverage, pipeline_id
- commit_merge_requests: List MRs associated with a commit. Params: project_id (required), sha (required)
- commit_cherry_pick: Cherry-pick a commit to a branch. Params: project_id (required), sha (required), branch (required), dry_run (bool), message
- commit_revert: Revert a commit on a branch. Params: project_id (required), sha (required), branch (required)
- commit_signature: Get GPG signature of a commit. Params: project_id (required), sha (required)
- file_get: Get file content from a repository. Params: project_id (required), file_path (required), ref (branch/tag/SHA, defaults to default branch)
- file_create: Create a new file. Params: project_id (required), file_path (required), branch (required), content, commit_message (required), start_branch, encoding (text/base64), author_email, author_name, execute_filemode (bool)
- file_update: Update an existing file. Params: project_id (required), file_path (required), branch (required), content, commit_message (required), start_branch, encoding, author_email, author_name, last_commit_id, execute_filemode (bool)
- file_delete: Delete a file from the repository. Params: project_id (required), file_path (required), branch (required), commit_message (required), start_branch, author_email, author_name, last_commit_id
- file_blame: Get blame information for a file. Params: project_id (required), file_path (required), ref, range_start, range_end
- file_metadata: Get file metadata without content. Params: project_id (required), file_path (required), ref
- file_raw: Get raw file content as plain text. Params: project_id (required), file_path (required), ref
- update_submodule: Update a submodule reference. Params: project_id (required), submodule (required, URL-encoded path), branch (required), commit_sha (required), commit_message
- list_submodules: List all submodules defined in the repository. Parses .gitmodules and enriches with commit SHAs and resolved project paths. Params: project_id (required), ref (branch/tag/SHA, optional)
- read_submodule_file: Read a file from inside a submodule transparently. Resolves the target project and pinned commit automatically. Params: project_id (required), submodule_path (required, e.g. libs/core-module), file_path (required, path inside the submodule), ref (optional)
- markdown_render: Render arbitrary markdown text to HTML. Params: text (required), gfm (bool, use GitLab Flavored Markdown), project (path for resolving references)
- commit_discussion_list: List discussions on a commit. Params: project_id (required), commit_id (required), page, per_page
- commit_discussion_get: Get a single commit discussion. Params: project_id (required), commit_id (required), discussion_id (required)
- commit_discussion_create: Create a commit discussion. Params: project_id (required), commit_id (required), body (required), position (optional)
- commit_discussion_add_note: Add a note to a commit discussion. Params: project_id (required), commit_id (required), discussion_id (required), body (required)
- commit_discussion_update_note: Update a note in a commit discussion. Params: project_id (required), commit_id (required), discussion_id (required), note_id (required), body (required)
- commit_discussion_delete_note: Delete a note from a commit discussion. Params: project_id (required), commit_id (required), discussion_id (required), note_id (required)`, routes, metaAnnotations, toolutil.IconFile)
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
	routes := map[string]actionFunc{
		"list":                           wrapAction(client, groups.List),
		"get":                            wrapAction(client, groups.Get),
		"create":                         wrapAction(client, groups.Create),
		"update":                         wrapAction(client, groups.Update),
		"delete":                         wrapVoidAction(client, groups.Delete),
		"restore":                        wrapAction(client, groups.Restore),
		"archive":                        wrapVoidAction(client, groups.Archive),
		"unarchive":                      wrapVoidAction(client, groups.Unarchive),
		"search":                         wrapAction(client, groups.Search),
		"transfer_project":               wrapAction(client, groups.TransferProject),
		"projects":                       wrapAction(client, groups.ListProjects),
		"members":                        wrapAction(client, groups.MembersList),
		"subgroups":                      wrapAction(client, groups.SubgroupsList),
		"issues":                         wrapAction(client, issues.ListGroup),
		"hook_list":                      wrapAction(client, groups.ListHooks),
		"hook_get":                       wrapAction(client, groups.GetHook),
		"hook_add":                       wrapAction(client, groups.AddHook),
		"hook_edit":                      wrapAction(client, groups.EditHook),
		"hook_delete":                    wrapVoidAction(client, groups.DeleteHook),
		"badge_list":                     wrapAction(client, badges.ListGroup),
		"badge_get":                      wrapAction(client, badges.GetGroup),
		"badge_add":                      wrapAction(client, badges.AddGroup),
		"badge_edit":                     wrapAction(client, badges.EditGroup),
		"badge_delete":                   wrapVoidAction(client, badges.DeleteGroup),
		"badge_preview":                  wrapAction(client, badges.PreviewGroup),
		"group_member_get":               wrapAction(client, groupmembers.GetMember),
		"group_member_get_inherited":     wrapAction(client, groupmembers.GetInheritedMember),
		"group_member_add":               wrapAction(client, groupmembers.AddMember),
		"group_member_edit":              wrapAction(client, groupmembers.EditMember),
		"group_member_remove":            wrapVoidAction(client, groupmembers.RemoveMember),
		"group_member_share":             wrapAction(client, groupmembers.ShareGroup),
		"group_member_unshare":           wrapVoidAction(client, groupmembers.UnshareGroup),
		"group_label_list":               wrapAction(client, grouplabels.List),
		"group_label_get":                wrapAction(client, grouplabels.Get),
		"group_label_create":             wrapAction(client, grouplabels.Create),
		"group_label_update":             wrapAction(client, grouplabels.Update),
		"group_label_delete":             wrapVoidAction(client, grouplabels.Delete),
		"group_label_subscribe":          wrapAction(client, grouplabels.Subscribe),
		"group_label_unsubscribe":        wrapVoidAction(client, grouplabels.Unsubscribe),
		"group_milestone_list":           wrapAction(client, groupmilestones.List),
		"group_milestone_get":            wrapAction(client, groupmilestones.Get),
		"group_milestone_create":         wrapAction(client, groupmilestones.Create),
		"group_milestone_update":         wrapAction(client, groupmilestones.Update),
		"group_milestone_delete":         wrapVoidAction(client, groupmilestones.Delete),
		"group_milestone_issues":         wrapAction(client, groupmilestones.GetIssues),
		"group_milestone_merge_requests": wrapAction(client, groupmilestones.GetMergeRequests),
		"group_milestone_burndown":       wrapAction(client, groupmilestones.GetBurndownChartEvents),
		"group_board_list":               wrapAction(client, groupboards.ListGroupBoards),
		"group_board_get":                wrapAction(client, groupboards.GetGroupBoard),
		"group_board_create":             wrapAction(client, groupboards.CreateGroupBoard),
		"group_board_update":             wrapAction(client, groupboards.UpdateGroupBoard),
		"group_board_delete":             wrapVoidAction(client, groupboards.DeleteGroupBoard),
		"group_board_list_lists":         wrapAction(client, groupboards.ListGroupBoardLists),
		"group_board_get_list":           wrapAction(client, groupboards.GetGroupBoardList),
		"group_board_create_list":        wrapAction(client, groupboards.CreateGroupBoardList),
		"group_board_update_list":        wrapAction(client, groupboards.UpdateGroupBoardList),
		"group_board_delete_list":        wrapVoidAction(client, groupboards.DeleteGroupBoardList),
		"group_upload_list":              wrapAction(client, groupmarkdownuploads.List),
		"group_upload_delete_by_id":      wrapVoidAction(client, groupmarkdownuploads.DeleteByID),
		"group_upload_delete_by_secret":  wrapVoidAction(client, groupmarkdownuploads.DeleteBySecretAndFilename),
		"group_relations_schedule":       wrapVoidAction(client, grouprelationsexport.ScheduleExport),
		"group_relations_list_status":    wrapAction(client, grouprelationsexport.ListExportStatus),
		"group_export_schedule":          wrapAction(client, groupimportexport.ScheduleExport),
		"group_export_download":          wrapAction(client, groupimportexport.ExportDownload),
		"group_import_file":              wrapAction(client, groupimportexport.ImportFile),
	}

	if enterprise {
		routes["epic_discussion_list"] = wrapAction(client, epicdiscussions.List)
		routes["epic_discussion_get"] = wrapAction(client, epicdiscussions.Get)
		routes["epic_discussion_create"] = wrapAction(client, epicdiscussions.Create)
		routes["epic_discussion_add_note"] = wrapAction(client, epicdiscussions.AddNote)
		routes["epic_discussion_update_note"] = wrapAction(client, epicdiscussions.UpdateNote)
		routes["epic_discussion_delete_note"] = wrapVoidAction(client, epicdiscussions.DeleteNote)
		routes["epic_list"] = wrapAction(client, epics.List)
		routes["epic_get"] = wrapAction(client, epics.Get)
		routes["epic_get_links"] = wrapAction(client, epics.GetLinks)
		routes["epic_create"] = wrapAction(client, epics.Create)
		routes["epic_update"] = wrapAction(client, epics.Update)
		routes["epic_delete"] = wrapVoidAction(client, epics.Delete)
		routes["epic_issue_list"] = wrapAction(client, epicissues.List)
		routes["epic_issue_assign"] = wrapAction(client, epicissues.Assign)
		routes["epic_issue_remove"] = wrapAction(client, epicissues.Remove)
		routes["epic_issue_update"] = wrapAction(client, epicissues.UpdateOrder)
		routes["epic_note_list"] = wrapAction(client, epicnotes.List)
		routes["epic_note_get"] = wrapAction(client, epicnotes.Get)
		routes["epic_note_create"] = wrapAction(client, epicnotes.Create)
		routes["epic_note_update"] = wrapAction(client, epicnotes.Update)
		routes["epic_note_delete"] = wrapVoidAction(client, epicnotes.Delete)
		routes["epic_board_list"] = wrapAction(client, groupepicboards.List)
		routes["epic_board_get"] = wrapAction(client, groupepicboards.Get)
		routes["wiki_list"] = wrapAction(client, groupwikis.List)
		routes["wiki_get"] = wrapAction(client, groupwikis.Get)
		routes["wiki_create"] = wrapAction(client, groupwikis.Create)
		routes["wiki_edit"] = wrapAction(client, groupwikis.Edit)
		routes["wiki_delete"] = wrapVoidAction(client, groupwikis.Delete)
		routes["protected_branch_list"] = wrapAction(client, groupprotectedbranches.List)
		routes["protected_branch_get"] = wrapAction(client, groupprotectedbranches.Get)
		routes["protected_branch_protect"] = wrapAction(client, groupprotectedbranches.Protect)
		routes["protected_branch_update"] = wrapAction(client, groupprotectedbranches.Update)
		routes["protected_branch_unprotect"] = wrapVoidAction(client, groupprotectedbranches.Unprotect)
		routes["protected_env_list"] = wrapAction(client, groupprotectedenvs.List)
		routes["protected_env_get"] = wrapAction(client, groupprotectedenvs.Get)
		routes["protected_env_protect"] = wrapAction(client, groupprotectedenvs.Protect)
		routes["protected_env_update"] = wrapAction(client, groupprotectedenvs.Update)
		routes["protected_env_unprotect"] = wrapVoidAction(client, groupprotectedenvs.Unprotect)
		routes["release_list"] = wrapAction(client, groupreleases.List)
		routes["ldap_link_list"] = wrapAction(client, groupldap.List)
		routes["ldap_link_add"] = wrapAction(client, groupldap.Add)
		routes["ldap_link_delete"] = wrapVoidAction(client, groupldap.DeleteWithCNOrFilter)
		routes["ldap_link_delete_for_provider"] = wrapVoidAction(client, groupldap.DeleteForProvider)
		routes["saml_link_list"] = wrapAction(client, groupsaml.List)
		routes["saml_link_get"] = wrapAction(client, groupsaml.Get)
		routes["saml_link_add"] = wrapAction(client, groupsaml.Add)
		routes["saml_link_delete"] = wrapVoidAction(client, groupsaml.Delete)
		routes["service_account_list"] = wrapAction(client, groupserviceaccounts.List)
		routes["service_account_create"] = wrapAction(client, groupserviceaccounts.Create)
		routes["service_account_update"] = wrapAction(client, groupserviceaccounts.Update)
		routes["service_account_delete"] = wrapVoidAction(client, groupserviceaccounts.Delete)
		routes["service_account_pat_list"] = wrapAction(client, groupserviceaccounts.ListPATs)
		routes["service_account_pat_create"] = wrapAction(client, groupserviceaccounts.CreatePAT)
		routes["service_account_pat_revoke"] = wrapVoidAction(client, groupserviceaccounts.RevokePAT)
		routes["analytics_issues_count"] = wrapAction(client, groupanalytics.GetIssuesCount)
		routes["analytics_mr_count"] = wrapAction(client, groupanalytics.GetMRCount)
		routes["analytics_members_count"] = wrapAction(client, groupanalytics.GetMembersCount)
		routes["credential_list_pats"] = wrapAction(client, groupcredentials.ListPATs)
		routes["credential_list_ssh_keys"] = wrapAction(client, groupcredentials.ListSSHKeys)
		routes["credential_revoke_pat"] = wrapVoidAction(client, groupcredentials.RevokePAT)
		routes["credential_delete_ssh_key"] = wrapVoidAction(client, groupcredentials.DeleteSSHKey)
		routes["ssh_cert_list"] = wrapAction(client, groupsshcerts.List)
		routes["ssh_cert_create"] = wrapAction(client, groupsshcerts.Create)
		routes["ssh_cert_delete"] = wrapVoidAction(client, groupsshcerts.Delete)
		routes["security_settings_update"] = wrapAction(client, securitysettings.UpdateGroup)
	}

	desc := `Manage GitLab groups and their members. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List accessible groups. Params: search, owned (bool), top_level_only (bool), page, per_page
- get: Get group details. Params: group_id (required, numeric ID or URL-encoded path like 'group/subgroup')
- create: Create a new group. Params: name (required), path, description, visibility (private/internal/public), parent_id, request_access_enabled (bool), lfs_enabled (bool), default_branch
- update: Update a group. Params: group_id (required), name, path, description, visibility, request_access_enabled (bool), lfs_enabled (bool), default_branch
- delete: Delete a group. Params: group_id (required), permanently_remove (bool), full_path (required when permanently_remove=true)
- restore: Restore a group marked for deletion. Params: group_id (required)
- archive: Archive a group. Requires Owner role or administrator. Params: group_id (required)
- unarchive: Unarchive a previously archived group. Requires Owner role or administrator. Params: group_id (required)
- search: Search groups by name. Params: query (required)
- transfer_project: Transfer a project into a group namespace. Params: group_id (required), project_id (required)
- projects: List projects belonging to a group. Set include_subgroups=true to include projects from descendant subgroups. Params: group_id (required), search, archived (bool), visibility, order_by, sort, simple (bool), owned (bool), starred (bool), include_subgroups (bool, recommended for hierarchical groups), with_shared (bool), page, per_page
- members: List all group members (including inherited). Params: group_id (required), query (filter by name/username), page, per_page
- subgroups: List descendant subgroups. Params: group_id (required), search, page, per_page
- issues: List issues across all projects in the group. Params: group_id (required), state, labels, milestone, scope, search, assignee_username, author_username, page, per_page
- hook_list: List group webhooks. Params: group_id (required), page, per_page
- hook_get: Get a specific group webhook. Params: group_id (required), hook_id (required)
- hook_add: Add a new group webhook. Params: group_id (required), url (required), name, description, token, push_events (bool), tag_push_events (bool), merge_requests_events (bool), issues_events (bool), note_events (bool), job_events (bool), pipeline_events (bool), wiki_page_events (bool), deployment_events (bool), releases_events (bool), subgroup_events (bool), member_events (bool), enable_ssl_verification (bool), push_events_branch_filter
- hook_edit: Edit an existing group webhook. Params: group_id (required), hook_id (required), url, name, description, token, (same event booleans as hook_add), enable_ssl_verification (bool)
- hook_delete: Delete a group webhook. Params: group_id (required), hook_id (required)
- badge_list: List group badges. Params: group_id (required), name, page, per_page
- badge_get: Get a group badge. Params: group_id (required), badge_id (required)
- badge_add: Add a badge to a group. Params: group_id (required), link_url (required), image_url (required), name
- badge_edit: Edit a group badge. Params: group_id (required), badge_id (required), link_url, image_url, name
- badge_delete: Delete a group badge. Params: group_id (required), badge_id (required)
- badge_preview: Preview how a group badge renders. Params: group_id (required), link_url (required), image_url (required), name
- group_member_get: Get a specific group member. Params: group_id (required), user_id (required)
- group_member_get_inherited: Get group member including inherited membership. Params: group_id (required), user_id (required)
- group_member_add: Add a group member. Params: group_id (required), user_id (required), access_level (required), expires_at
- group_member_edit: Edit a group member. Params: group_id (required), user_id (required), access_level (required), expires_at
- group_member_remove: Remove a group member. Params: group_id (required), user_id (required)
- group_member_share: Share group with another group. Params: group_id (required), shared_with_group_id (required), group_access (required), expires_at
- group_member_unshare: Unshare group. Params: group_id (required), shared_with_group_id (required)
- group_label_list: List group labels. Params: group_id (required), search, with_counts (bool), include_ancestor_groups (bool), include_descendant_groups (bool), page, per_page
- group_label_get: Get a group label. Params: group_id (required), label_id (required)
- group_label_create: Create a group label. Params: group_id (required), name (required), color (required), description
- group_label_update: Update a group label. Params: group_id (required), label_id (required), new_name, color, description
- group_label_delete: Delete a group label. Params: group_id (required), label_id (required)
- group_label_subscribe: Subscribe to a group label. Params: group_id (required), label_id (required)
- group_label_unsubscribe: Unsubscribe from a group label. Params: group_id (required), label_id (required)
- group_milestone_list: List group milestones. Params: group_id (required), state, title, search, include_ancestors (bool), include_descendants (bool), page, per_page
- group_milestone_get: Get a group milestone. Params: group_id (required), milestone_iid (required)
- group_milestone_create: Create a group milestone. Params: group_id (required), title (required), description, start_date, due_date
- group_milestone_update: Update a group milestone. Params: group_id (required), milestone_iid (required), title, description, start_date, due_date, state_event
- group_milestone_delete: Delete a group milestone. Params: group_id (required), milestone_iid (required)
- group_milestone_issues: List issues in a group milestone. Params: group_id (required), milestone_iid (required), page, per_page
- group_milestone_merge_requests: List MRs in a group milestone. Params: group_id (required), milestone_iid (required), page, per_page
- group_milestone_burndown: Get burndown chart events. Params: group_id (required), milestone_iid (required)
- group_board_list: List group boards. Params: group_id (required), page, per_page
- group_board_get: Get a group board. Params: group_id (required), board_id (required)
- group_board_create: Create a group board. Params: group_id (required), name (required)
- group_board_update: Update a group board. Params: group_id (required), board_id (required), name, assignee_id, milestone_id, labels (comma-separated string), weight
- group_board_delete: Delete a group board. Params: group_id (required), board_id (required)
- group_board_list_lists: List board lists. Params: group_id (required), board_id (required)
- group_board_get_list: Get a board list. Params: group_id (required), board_id (required), list_id (required)
- group_board_create_list: Create a board list. Params: group_id (required), board_id (required), label_id
- group_board_update_list: Update a board list. Params: group_id (required), board_id (required), list_id (required), position
- group_board_delete_list: Delete a board list. Params: group_id (required), board_id (required), list_id (required)
- group_upload_list: List group uploads. Params: group_id (required)
- group_upload_delete_by_id: Delete a group upload by ID. Params: group_id (required), upload_id (required)
- group_upload_delete_by_secret: Delete a group upload by secret. Params: group_id (required), secret (required), filename (required)
- group_relations_schedule: Schedule group relations export. Params: group_id (required)
- group_relations_list_status: List group relations export status. Params: group_id (required)
- group_export_schedule: Schedule group export. Params: group_id (required)
- group_export_download: Download group export archive. Params: group_id (required)
- group_import_file: Import a group from file. Params: name (required), path (required), file (required), parent_id`

	if enterprise {
		desc += `

Epics (Premium+ — requires GITLAB_ENTERPRISE=true):
- epic_discussion_list: List epic discussions. Params: group_id (required), epic_id (required), page, per_page
- epic_discussion_get: Get an epic discussion. Params: group_id (required), epic_id (required), discussion_id (required)
- epic_discussion_create: Create an epic discussion. Params: group_id (required), epic_id (required), body (required)
- epic_discussion_add_note: Add note to epic discussion. Params: group_id (required), epic_id (required), discussion_id (required), body (required)
- epic_discussion_update_note: Update note in epic discussion. Params: group_id (required), epic_id (required), discussion_id (required), note_id (required), body
- epic_discussion_delete_note: Delete note from epic discussion. Params: group_id (required), epic_id (required), discussion_id (required), note_id (required)
- epic_list: List epics in a group. Params: group_id (required), author_id, labels, order_by, sort, search, state, include_ancestor_groups (bool), include_descendant_groups (bool), page, per_page
- epic_get: Get a single epic. Params: group_id (required), epic_iid (required)
- epic_get_links: Get linked epics. Params: group_id (required), epic_iid (required)
- epic_create: Create an epic in a group. Params: group_id (required), title (required), description, labels, confidential (bool), parent_id, color, start_date_fixed, due_date_fixed
- epic_update: Update an epic. Params: group_id (required), epic_iid (required), title, description, labels, confidential (bool), state_event, add_labels, remove_labels, color, start_date_fixed, due_date_fixed
- epic_delete: Delete an epic. Params: group_id (required), epic_iid (required)
- epic_issue_list: List issues assigned to an epic. Params: group_id (required), epic_iid (required), page, per_page
- epic_issue_assign: Assign an issue to an epic. Params: group_id (required), epic_iid (required), issue_id (required)
- epic_issue_remove: Remove an issue from an epic. Params: group_id (required), epic_iid (required), epic_issue_id (required)
- epic_issue_update: Reorder an issue in an epic. Params: group_id (required), epic_iid (required), epic_issue_id (required), move_before_id, move_after_id
- epic_note_list: List notes on an epic. Params: group_id (required), epic_iid (required), order_by, sort, page, per_page
- epic_note_get: Get a single epic note. Params: group_id (required), epic_iid (required), note_id (required)
- epic_note_create: Add a note to an epic. Params: group_id (required), epic_iid (required), body (required)
- epic_note_update: Update an epic note. Params: group_id (required), epic_iid (required), note_id (required), body (required)
- epic_note_delete: Delete an epic note. Params: group_id (required), epic_iid (required), note_id (required)
- epic_board_list: List group epic boards. Params: group_id (required), page, per_page
- epic_board_get: Get a group epic board. Params: group_id (required), board_id (required)

Group Wikis (Premium+ — requires GITLAB_ENTERPRISE=true):
- wiki_list: List group wiki pages. Params: group_id (required), with_content (bool)
- wiki_get: Get a group wiki page. Params: group_id (required), slug (required), render_html (bool), version
- wiki_create: Create a group wiki page. Params: group_id (required), title (required), content (required), format (markdown/rdoc/asciidoc/org)
- wiki_edit: Edit a group wiki page. Params: group_id (required), slug (required), title, content, format
- wiki_delete: Delete a group wiki page. Params: group_id (required), slug (required)

Group Protected Branches (Premium+ — requires GITLAB_ENTERPRISE=true):
- protected_branch_list: List group protected branches. Params: group_id (required), search, page, per_page
- protected_branch_get: Get a group protected branch. Params: group_id (required), branch (required)
- protected_branch_protect: Protect a group branch. Params: group_id (required), name (required), push_access_level, merge_access_level, unprotect_access_level, allow_force_push (bool), code_owner_approval_required (bool), allowed_to_push, allowed_to_merge, allowed_to_unprotect
- protected_branch_update: Update a group protected branch. Params: group_id (required), branch (required), name, allow_force_push (bool), code_owner_approval_required (bool), allowed_to_push, allowed_to_merge, allowed_to_unprotect
- protected_branch_unprotect: Unprotect a group branch. Params: group_id (required), branch (required)

Group Protected Environments (Premium+ — requires GITLAB_ENTERPRISE=true):
- protected_env_list: List group protected environments. Params: group_id (required), page, per_page
- protected_env_get: Get a group protected environment. Params: group_id (required), environment (required)
- protected_env_protect: Protect a group environment. Params: group_id (required), name (required), deploy_access_levels, required_approval_count, approval_rules
- protected_env_update: Update a group protected environment. Params: group_id (required), environment (required), name, deploy_access_levels, required_approval_count, approval_rules
- protected_env_unprotect: Unprotect a group environment. Params: group_id (required), environment (required)

Group Releases (Premium+ — requires GITLAB_ENTERPRISE=true):
- release_list: List releases across group projects. Params: group_id (required), simple (bool), page, per_page

LDAP Links (Premium+ — requires GITLAB_ENTERPRISE=true):
- ldap_link_list: List group LDAP links. Params: group_id (required)
- ldap_link_add: Add a group LDAP link. Params: group_id (required), cn (required), group_access (required, access level int), provider (required)
- ldap_link_delete: Delete a group LDAP link by CN or filter. Params: group_id (required), cn, filter, provider
- ldap_link_delete_for_provider: Delete a group LDAP link for a specific provider. Params: group_id (required), provider (required), cn (required)

SAML Links (Premium+ — requires GITLAB_ENTERPRISE=true):
- saml_link_list: List group SAML links. Params: group_id (required)
- saml_link_get: Get a group SAML link. Params: group_id (required), saml_group_name (required)
- saml_link_add: Add a group SAML link. Params: group_id (required), saml_group_name (required), access_level (required, access level int)
- saml_link_delete: Delete a group SAML link. Params: group_id (required), saml_group_name (required)

Service Accounts (Premium+ — requires GITLAB_ENTERPRISE=true):
- service_account_list: List group service accounts. Params: group_id (required), order_by, sort, page, per_page
- service_account_create: Create a group service account. Params: group_id (required), name (required), username (required)
- service_account_update: Update a group service account. Params: group_id (required), service_account_id (required), name, username
- service_account_delete: Delete a group service account. Params: group_id (required), service_account_id (required), hard_delete (bool)
- service_account_pat_list: List personal access tokens for a group service account. Params: group_id (required), service_account_id (required), page, per_page
- service_account_pat_create: Create a personal access token for a group service account. Params: group_id (required), service_account_id (required), name (required), scopes (required, array), expires_at (YYYY-MM-DD)
- service_account_pat_revoke: Revoke a personal access token for a group service account. Params: group_id (required), service_account_id (required), token_id (required)
- analytics_issues_count: Get count of recently created issues. Params: group_path (required, URL-encoded path)
- analytics_mr_count: Get count of recently created merge requests. Params: group_path (required, URL-encoded path)
- analytics_members_count: Get count of recently added members. Params: group_path (required, URL-encoded path)

Credentials (Ultimate — requires GITLAB_ENTERPRISE=true):
- credential_list_pats: List personal access tokens managed by a group. Params: group_id (required), search, state (active/inactive), revoked (bool), page, per_page
- credential_list_ssh_keys: List SSH keys managed by a group. Params: group_id (required), page, per_page
- credential_revoke_pat: Revoke a personal access token. Params: group_id (required), token_id (required)
- credential_delete_ssh_key: Delete an SSH key. Params: group_id (required), key_id (required)

SSH Certificates (Premium+ — requires GITLAB_ENTERPRISE=true):
- ssh_cert_list: List SSH certificates for a group. Params: group_id (required)
- ssh_cert_create: Create an SSH certificate. Params: group_id (required), key (required), title (required)
- ssh_cert_delete: Delete an SSH certificate. Params: group_id (required), certificate_id (required)

Security Settings (Ultimate — requires GITLAB_ENTERPRISE=true):
- security_settings_update: Update group secret push protection. Params: group_id (required), secret_push_protection_enabled (required), projects_to_exclude (optional, array of project IDs)`
	}

	addMetaTool(server, "gitlab_group", desc, routes, metaAnnotations, toolutil.IconGroup)
}

// registerIssueMeta registers the gitlab_issue meta-tool with actions:
// create, get, list, update, delete, note_create, note_list, note_get,
// note_update, note_delete, list_group, link_list, link_get, link_create, link_delete,
// work_item_get, work_item_list, work_item_create, work_item_update, work_item_delete.
func registerIssueMeta(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	routes := map[string]actionFunc{
		"create":                     wrapAction(client, issues.Create),
		"get":                        wrapAction(client, issues.Get),
		"get_by_id":                  wrapAction(client, issues.GetByID),
		"list":                       wrapAction(client, issues.List),
		"list_all":                   wrapAction(client, issues.ListAll),
		"update":                     wrapAction(client, issues.Update),
		"delete":                     wrapVoidAction(client, issues.Delete),
		"list_group":                 wrapAction(client, issues.ListGroup),
		"reorder":                    wrapAction(client, issues.Reorder),
		"move":                       wrapAction(client, issues.Move),
		"subscribe":                  wrapAction(client, issues.Subscribe),
		"unsubscribe":                wrapAction(client, issues.Unsubscribe),
		"create_todo":                wrapAction(client, issues.CreateTodo),
		"note_create":                wrapAction(client, issuenotes.Create),
		"note_list":                  wrapAction(client, issuenotes.List),
		"note_get":                   wrapAction(client, issuenotes.GetNote),
		"note_update":                wrapAction(client, issuenotes.Update),
		"note_delete":                wrapVoidAction(client, issuenotes.Delete),
		"link_list":                  wrapAction(client, issuelinks.List),
		"link_get":                   wrapAction(client, issuelinks.Get),
		"link_create":                wrapAction(client, issuelinks.Create),
		"link_delete":                wrapVoidAction(client, issuelinks.Delete),
		"time_estimate_set":          wrapAction(client, issues.SetTimeEstimate),
		"time_estimate_reset":        wrapAction(client, issues.ResetTimeEstimate),
		"spent_time_add":             wrapAction(client, issues.AddSpentTime),
		"spent_time_reset":           wrapAction(client, issues.ResetSpentTime),
		"time_stats_get":             wrapAction(client, issues.GetTimeStats),
		"participants":               wrapAction(client, issues.GetParticipants),
		"mrs_closing":                wrapAction(client, issues.ListMRsClosing),
		"mrs_related":                wrapAction(client, issues.ListMRsRelated),
		"work_item_get":              wrapAction(client, workitems.Get),
		"work_item_list":             wrapAction(client, workitems.List),
		"work_item_create":           wrapAction(client, workitems.Create),
		"work_item_update":           wrapAction(client, workitems.Update),
		"work_item_delete":           wrapVoidAction(client, workitems.Delete),
		"discussion_list":            wrapAction(client, issuediscussions.List),
		"discussion_get":             wrapAction(client, issuediscussions.Get),
		"discussion_create":          wrapAction(client, issuediscussions.Create),
		"discussion_add_note":        wrapAction(client, issuediscussions.AddNote),
		"discussion_update_note":     wrapAction(client, issuediscussions.UpdateNote),
		"discussion_delete_note":     wrapVoidAction(client, issuediscussions.DeleteNote),
		"statistics_get":             wrapAction(client, issuestatistics.Get),
		"statistics_get_group":       wrapAction(client, issuestatistics.GetGroup),
		"statistics_get_project":     wrapAction(client, issuestatistics.GetProject),
		"emoji_issue_list":           wrapAction(client, awardemoji.ListIssueAwardEmoji),
		"emoji_issue_get":            wrapAction(client, awardemoji.GetIssueAwardEmoji),
		"emoji_issue_create":         wrapAction(client, awardemoji.CreateIssueAwardEmoji),
		"emoji_issue_delete":         wrapVoidAction(client, awardemoji.DeleteIssueAwardEmoji),
		"emoji_issue_note_list":      wrapAction(client, awardemoji.ListIssueNoteAwardEmoji),
		"emoji_issue_note_get":       wrapAction(client, awardemoji.GetIssueNoteAwardEmoji),
		"emoji_issue_note_create":    wrapAction(client, awardemoji.CreateIssueNoteAwardEmoji),
		"emoji_issue_note_delete":    wrapVoidAction(client, awardemoji.DeleteIssueNoteAwardEmoji),
		"event_issue_label_list":     wrapAction(client, resourceevents.ListIssueLabelEvents),
		"event_issue_label_get":      wrapAction(client, resourceevents.GetIssueLabelEvent),
		"event_issue_milestone_list": wrapAction(client, resourceevents.ListIssueMilestoneEvents),
		"event_issue_milestone_get":  wrapAction(client, resourceevents.GetIssueMilestoneEvent),
		"event_issue_state_list":     wrapAction(client, resourceevents.ListIssueStateEvents),
		"event_issue_state_get":      wrapAction(client, resourceevents.GetIssueStateEvent),
		"event_issue_iteration_list": wrapAction(client, resourceevents.ListIssueIterationEvents),
		"event_issue_iteration_get":  wrapAction(client, resourceevents.GetIssueIterationEvent),
		"event_issue_weight_list":    wrapAction(client, resourceevents.ListIssueWeightEvents),
	}

	if enterprise {
		routes["iteration_list_project"] = wrapAction(client, projectiterations.List)
		routes["iteration_list_group"] = wrapAction(client, groupiterations.List)
	}

	desc := `Manage GitLab issues. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- create: Create a new issue. Params: project_id (required), title (required), description, assignee_id (single user ID), assignee_ids ([]int, multiple user IDs), labels (comma-separated string), milestone_id (int), due_date (YYYY-MM-DD), confidential (bool), issue_type (issue/incident/test_case/task), weight (int), epic_id (int)
- get: Get issue details. Params: project_id (required), issue_iid (required)
- get_by_id: Get issue by global ID. Params: issue_id (required)
- list: List project issues. Params: project_id (required), state (opened/closed/all), labels (comma-separated), not_labels, milestone, scope (created_by_me/assigned_to_me/all), search, assignee_username, author_username, iids ([]int), issue_type, confidential (bool), created_after/created_before (ISO 8601), updated_after/updated_before (ISO 8601), order_by (created_at/updated_at/priority/due_date), sort (asc/desc), page, per_page
- list_all: List all issues visible to authenticated user (global). Params: state, labels, milestone, scope, search, assignee_username, author_username, order_by, sort, created_after/created_before, updated_after/updated_before, confidential (bool), page, per_page
- update: Update an issue. Note: time tracking (estimates/spent time) uses dedicated actions (time_estimate_set, spent_time_add, etc.) — do NOT pass time params here. Params: project_id (required), issue_iid (required), title, description, state_event (close/reopen), assignee_id (single user ID), assignee_ids (multiple user IDs), labels (comma-separated string), add_labels (comma-separated string), remove_labels (comma-separated string), milestone_id, due_date, confidential (bool), issue_type, weight (int), epic_id (int), discussion_locked (bool)
- delete: Delete an issue permanently. Params: project_id (required), issue_iid (required)
- list_group: List issues for a group. Params: group_id (required), state, labels, milestone, scope, search, order_by, sort, page, per_page
- reorder: Reorder an issue relative to others. Params: project_id (required), issue_iid (required), move_after_id (int), move_before_id (int)
- move: Move an issue to another project. Params: project_id (required), issue_iid (required), to_project_id (required)
- subscribe: Subscribe to issue notifications. Params: project_id (required), issue_iid (required)
- unsubscribe: Unsubscribe from issue notifications. Params: project_id (required), issue_iid (required)
- create_todo: Create a to-do for the issue. Params: project_id (required), issue_iid (required)
- time_estimate_set: Set time estimate. Params: project_id (required), issue_iid (required), duration (required, e.g. 3h30m)
- time_estimate_reset: Reset time estimate to zero. Params: project_id (required), issue_iid (required)
- spent_time_add: Add spent time. Params: project_id (required), issue_iid (required), duration (required, e.g. 1h), summary
- spent_time_reset: Reset spent time to zero. Params: project_id (required), issue_iid (required)
- time_stats_get: Get time tracking statistics. Params: project_id (required), issue_iid (required)
- participants: List issue participants. Params: project_id (required), issue_iid (required)
- mrs_closing: List MRs that will close this issue. Params: project_id (required), issue_iid (required), page, per_page
- mrs_related: List MRs related to this issue. Params: project_id (required), issue_iid (required), page, per_page
- note_create: Add a comment to an issue. Params: project_id (required), issue_iid (required), body (required), internal (bool)
- note_list: List issue comments. Params: project_id (required), issue_iid (required), order_by, sort, page, per_page
- note_get: Get a single issue comment by note ID. Params: project_id (required), issue_iid (required), note_id (required)
- note_update: Edit an issue comment. Params: project_id (required), issue_iid (required), note_id (required), body (required)
- note_delete: Delete an issue comment. Params: project_id (required), issue_iid (required), note_id (required)
- link_list: List linked issues. Params: project_id (required), issue_iid (required)
- link_get: Get a specific link. Params: project_id (required), issue_iid (required), issue_link_id (required)
- link_create: Create a link between issues. Params: project_id (required), issue_iid (required), target_project_id (required), target_issue_iid (required), link_type
- link_delete: Delete an issue link. Params: project_id (required), issue_iid (required), issue_link_id (required)
- work_item_get: Get a single work item by IID. Params: full_path (required), iid (required)
- work_item_list: List work items for a project or group. Params: full_path (required), state, search, types, author_username, label_name, confidential, sort, first, after
- work_item_create: Create a new work item. Params: full_path (required), work_item_type_id (required), title (required), description, confidential, assignee_ids, milestone_id, label_ids, weight, health_status, color, due_date, start_date
- work_item_update: Update an existing work item by IID. Params: full_path (required), iid (required), title, state_event (CLOSE/REOPEN), description, assignee_ids, milestone_id, crm_contact_ids, parent_id, add_label_ids, remove_label_ids, start_date, due_date, weight, health_status, iteration_id, color
- work_item_delete: Permanently delete a work item by IID. Params: full_path (required), iid (required)
- discussion_list: List issue discussions. Params: project_id (required), issue_iid (required), page, per_page
- discussion_get: Get an issue discussion. Params: project_id (required), issue_iid (required), discussion_id (required)
- discussion_create: Create an issue discussion. Params: project_id (required), issue_iid (required), body (required)
- discussion_add_note: Add note to issue discussion. Params: project_id (required), issue_iid (required), discussion_id (required), body (required)
- discussion_update_note: Update note in issue discussion. Params: project_id (required), issue_iid (required), discussion_id (required), note_id (required), body (required)
- discussion_delete_note: Delete note from issue discussion. Params: project_id (required), issue_iid (required), discussion_id (required), note_id (required)
- statistics_get: Get global issue statistics. Params: (none or same filters as list)
- statistics_get_group: Get group issue statistics. Params: group_id (required)
- statistics_get_project: Get project issue statistics. Params: project_id (required)
- emoji_issue_list: List award emoji on an issue. Params: project_id (required), iid (required), page, per_page
- emoji_issue_get: Get an award emoji on an issue. Params: project_id (required), iid (required), award_id (required)
- emoji_issue_create: Add award emoji to an issue. Params: project_id (required), iid (required), name (required)
- emoji_issue_delete: Delete award emoji from an issue. Params: project_id (required), iid (required), award_id (required)
- emoji_issue_note_list: List award emoji on an issue note. Params: project_id (required), iid (required), note_id (required), page, per_page
- emoji_issue_note_get: Get an award emoji on an issue note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- emoji_issue_note_create: Add award emoji to an issue note. Params: project_id (required), iid (required), note_id (required), name (required)
- emoji_issue_note_delete: Delete award emoji from an issue note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- event_issue_label_list: List label events on an issue. Params: project_id (required), issue_iid (required), page, per_page
- event_issue_label_get: Get a label event. Params: project_id (required), issue_iid (required), label_event_id (required)
- event_issue_milestone_list: List milestone events on an issue. Params: project_id (required), issue_iid (required), page, per_page
- event_issue_milestone_get: Get a milestone event on an issue. Params: project_id (required), issue_iid (required), milestone_event_id (required)
- event_issue_state_list: List state events on an issue. Params: project_id (required), issue_iid (required), page, per_page
- event_issue_state_get: Get a state event on an issue. Params: project_id (required), issue_iid (required), state_event_id (required)
- event_issue_iteration_list: List iteration events on an issue. Params: project_id (required), issue_iid (required), page, per_page
- event_issue_iteration_get: Get an iteration event on an issue. Params: project_id (required), issue_iid (required), iteration_event_id (required)
- event_issue_weight_list: List weight events on an issue. Params: project_id (required), issue_iid (required), page, per_page`

	if enterprise {
		desc += `

Iterations (Premium+ — requires GITLAB_ENTERPRISE=true):
- iteration_list_project: List iterations for a project. Params: project_id (required), state (1=opened, 2=upcoming, 3=current, 4=closed), search (string), include_ancestors (bool), page, per_page
- iteration_list_group: List iterations for a group. Params: group_id (required), state, search, include_ancestors (bool), page, per_page`
	}

	addMetaTool(server, "gitlab_issue", desc, routes, metaAnnotations, toolutil.IconIssue)
}

// registerPipelineMeta registers the gitlab_pipeline meta-tool with actions:
// list, get, cancel, retry, and delete.
func registerPipelineMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                         wrapAction(client, pipelines.List),
		"get":                          wrapAction(client, pipelines.Get),
		"cancel":                       wrapAction(client, pipelines.Cancel),
		"retry":                        wrapAction(client, pipelines.Retry),
		"delete":                       wrapVoidAction(client, pipelines.Delete),
		"variables":                    wrapAction(client, pipelines.GetVariables),
		"test_report":                  wrapAction(client, pipelines.GetTestReport),
		"test_report_summary":          wrapAction(client, pipelines.GetTestReportSummary),
		"latest":                       wrapAction(client, pipelines.GetLatest),
		"create":                       wrapAction(client, pipelines.Create),
		"update_metadata":              wrapAction(client, pipelines.UpdateMetadata),
		"trigger_list":                 wrapAction(client, pipelinetriggers.ListTriggers),
		"trigger_get":                  wrapAction(client, pipelinetriggers.GetTrigger),
		"trigger_create":               wrapAction(client, pipelinetriggers.CreateTrigger),
		"trigger_update":               wrapAction(client, pipelinetriggers.UpdateTrigger),
		"trigger_delete":               wrapVoidAction(client, pipelinetriggers.DeleteTrigger),
		"trigger_run":                  wrapAction(client, pipelinetriggers.RunTrigger),
		"resource_group_list":          wrapAction(client, resourcegroups.ListAll),
		"resource_group_get":           wrapAction(client, resourcegroups.Get),
		"resource_group_edit":          wrapAction(client, resourcegroups.Edit),
		"resource_group_upcoming_jobs": wrapAction(client, resourcegroups.ListUpcomingJobs),
	}

	addMetaTool(server, "gitlab_pipeline", `Manage GitLab CI/CD pipelines. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List pipelines for a project. Params: project_id (required), status (success/failed/running/pending/canceled), scope (running/pending/finished/branches/tags), source (push/web/schedule/merge_request_event), ref, sha, username, page, per_page
- get: Get pipeline details. Params: project_id (required), pipeline_id (required)
- cancel: Cancel a running pipeline. Params: project_id (required), pipeline_id (required)
- retry: Retry all failed jobs in a pipeline. Params: project_id (required), pipeline_id (required)
- delete: PERMANENTLY delete a pipeline and all its jobs. Params: project_id (required), pipeline_id (required)
- variables: Get pipeline variables. Params: project_id (required), pipeline_id (required)
- test_report: Get full test report. Params: project_id (required), pipeline_id (required)
- test_report_summary: Get test report summary. Params: project_id (required), pipeline_id (required)
- latest: Get latest pipeline. Params: project_id (required), ref (optional branch/tag)
- create: Create a new pipeline. Params: project_id (required), ref (required), variables (optional array of {key, value, variable_type})
- update_metadata: Update pipeline name. Params: project_id (required), pipeline_id (required), name (required)
- trigger_list: List pipeline triggers. Params: project_id (required), page, per_page
- trigger_get: Get a pipeline trigger. Params: project_id (required), trigger_id (required)
- trigger_create: Create a pipeline trigger. Params: project_id (required), description (required)
- trigger_update: Update a pipeline trigger. Params: project_id (required), trigger_id (required), description
- trigger_delete: Delete a pipeline trigger. Params: project_id (required), trigger_id (required)
- trigger_run: Run a pipeline trigger. Params: project_id (required), ref (required), token (required), variables (map)
- resource_group_list: List resource groups. Params: project_id (required), page, per_page
- resource_group_get: Get a resource group. Params: project_id (required), key (required)
- resource_group_edit: Edit a resource group. Params: project_id (required), key (required), process_mode
- resource_group_upcoming_jobs: List upcoming jobs for a resource group. Params: project_id (required), key (required), page, per_page`, routes, metaAnnotations, toolutil.IconPipeline)
}

// registerJobMeta registers the gitlab_job meta-tool with actions:
// list, list_project, get, trace, cancel, retry, list_bridges, artifacts, download_artifacts,
// download_single_artifact, download_single_artifact_by_ref, erase, keep_artifacts, play,
// delete_artifacts, delete_project_artifacts.
func registerJobMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                            wrapAction(client, jobs.List),
		"list_project":                    wrapAction(client, jobs.ListProject),
		"get":                             wrapAction(client, jobs.Get),
		"trace":                           wrapAction(client, jobs.Trace),
		"cancel":                          wrapAction(client, jobs.Cancel),
		"retry":                           wrapAction(client, jobs.Retry),
		"list_bridges":                    wrapAction(client, jobs.ListBridges),
		"artifacts":                       wrapAction(client, jobs.GetArtifacts),
		"download_artifacts":              wrapAction(client, jobs.DownloadArtifacts),
		"download_single_artifact":        wrapAction(client, jobs.DownloadSingleArtifact),
		"download_single_artifact_by_ref": wrapAction(client, jobs.DownloadSingleArtifactByRef),
		"erase":                           wrapAction(client, jobs.Erase),
		"keep_artifacts":                  wrapAction(client, jobs.KeepArtifacts),
		"play":                            wrapAction(client, jobs.Play),
		"delete_artifacts":                wrapVoidAction(client, jobs.DeleteArtifacts),
		"delete_project_artifacts":        wrapVoidAction(client, jobs.DeleteProjectArtifacts),
		"token_scope_get":                 wrapAction(client, jobtokenscope.GetAccessSettings),
		"token_scope_patch":               wrapAction(client, jobtokenscope.PatchAccessSettings),
		"token_scope_list_inbound":        wrapAction(client, jobtokenscope.ListInboundAllowlist),
		"token_scope_add_project":         wrapAction(client, jobtokenscope.AddProjectAllowlist),
		"token_scope_remove_project":      wrapVoidAction(client, jobtokenscope.RemoveProjectAllowlist),
		"token_scope_list_groups":         wrapAction(client, jobtokenscope.ListGroupAllowlist),
		"token_scope_add_group":           wrapAction(client, jobtokenscope.AddGroupAllowlist),
		"token_scope_remove_group":        wrapVoidAction(client, jobtokenscope.RemoveGroupAllowlist),
	}

	addMetaTool(server, "gitlab_job", `Manage GitLab CI/CD jobs. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List jobs for a pipeline. Params: project_id (required), pipeline_id (required), scope, page, per_page
- list_project: List all jobs across a project. Params: project_id (required), scope, include_retried, page, per_page
- get: Get job details. Params: project_id (required), job_id (required)
- trace: Get job log output (truncated to 100KB). Params: project_id (required), job_id (required)
- cancel: Cancel a running or pending job. Params: project_id (required), job_id (required)
- retry: Retry a failed or canceled job. Params: project_id (required), job_id (required)
- list_bridges: List bridge (trigger) jobs for a pipeline. Params: project_id (required), pipeline_id (required), scope, page, per_page
- artifacts: Download artifacts archive for a job (base64, max 1MB). Params: project_id (required), job_id (required)
- download_artifacts: Download artifacts archive by ref/job name (base64, max 1MB). Params: project_id (required), ref_name (required), job
- download_single_artifact: Download a single artifact file by job ID and path. Params: project_id (required), job_id (required), artifact_path (required)
- download_single_artifact_by_ref: Download a single artifact file by ref and path. Params: project_id (required), ref_name (required), artifact_path (required), job
- erase: Erase a job's trace and artifacts. Params: project_id (required), job_id (required)
- keep_artifacts: Prevent artifacts from expiring. Params: project_id (required), job_id (required)
- play: Trigger a manual job. Params: project_id (required), job_id (required), variables (optional array of {key, value, variable_type})
- delete_artifacts: Delete artifacts for a specific job. Params: project_id (required), job_id (required)
- delete_project_artifacts: Delete all artifacts across a project. Params: project_id (required)
- token_scope_get: Get CI/CD job token access settings. Params: project_id (required)
- token_scope_patch: Update CI/CD job token access settings. Params: project_id (required), enabled (bool)
- token_scope_list_inbound: List inbound project allowlist for job tokens. Params: project_id (required), page, per_page
- token_scope_add_project: Add a project to the inbound allowlist. Params: project_id (required), target_project_id (required)
- token_scope_remove_project: Remove a project from the inbound allowlist. Params: project_id (required), target_project_id (required)
- token_scope_list_groups: List group allowlist for job tokens. Params: project_id (required), page, per_page
- token_scope_add_group: Add a group to the job token allowlist. Params: project_id (required), target_group_id (required)
- token_scope_remove_group: Remove a group from the job token allowlist. Params: project_id (required), target_group_id (required)`, routes, metaAnnotations, toolutil.IconJob)
}

// registerUserMeta registers the gitlab_user meta-tool with user,
// SSH key, email, event, notification, key, GPG key, impersonation token, and task-list management actions.
func registerUserMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"current":                     wrapAction(client, users.Current),
		"list":                        wrapAction(client, users.List),
		"get":                         wrapAction(client, users.Get),
		"get_status":                  wrapAction(client, users.GetStatus),
		"set_status":                  wrapAction(client, users.SetStatus),
		"ssh_keys":                    wrapAction(client, users.ListSSHKeys),
		"emails":                      wrapAction(client, users.ListEmails),
		"contribution_events":         wrapAction(client, users.ListContributionEvents),
		"associations_count":          wrapAction(client, users.GetAssociationsCount),
		"todo_list":                   wrapAction(client, todos.List),
		"todo_mark_done":              wrapAction(client, todos.MarkDone),
		"todo_mark_all_done":          wrapAction(client, todos.MarkAllDone),
		"event_list_project":          wrapAction(client, events.ListProjectEvents),
		"event_list_contributions":    wrapAction(client, events.ListCurrentUserContributionEvents),
		"notification_global_get":     wrapAction(client, notifications.GetGlobalSettings),
		"notification_project_get":    wrapAction(client, notifications.GetSettingsForProject),
		"notification_group_get":      wrapAction(client, notifications.GetSettingsForGroup),
		"notification_global_update":  wrapAction(client, notifications.UpdateGlobalSettings),
		"notification_project_update": wrapAction(client, notifications.UpdateSettingsForProject),
		"notification_group_update":   wrapAction(client, notifications.UpdateSettingsForGroup),
		"key_get_with_user":           wrapAction(client, keys.GetKeyWithUser),
		"key_get_by_fingerprint":      wrapAction(client, keys.GetKeyByFingerprint),
		"namespace_list":              wrapAction(client, namespaces.List),
		"namespace_get":               wrapAction(client, namespaces.Get),
		"namespace_exists":            wrapAction(client, namespaces.Exists),
		"namespace_search":            wrapAction(client, namespaces.Search),
		"avatar_get":                  wrapAction(client, avatar.Get),
		"me":                          wrapAction(client, users.Current),
		// Extended user admin actions
		"block":              wrapAction(client, users.BlockUser),
		"unblock":            wrapAction(client, users.UnblockUser),
		"ban":                wrapAction(client, users.BanUser),
		"unban":              wrapAction(client, users.UnbanUser),
		"activate":           wrapAction(client, users.ActivateUser),
		"deactivate":         wrapAction(client, users.DeactivateUser),
		"approve":            wrapAction(client, users.ApproveUser),
		"reject":             wrapAction(client, users.RejectUser),
		"disable_two_factor": wrapAction(client, users.DisableTwoFactor),
		// User CRUD
		"create": wrapAction(client, users.Create),
		"modify": wrapAction(client, users.Modify),
		"delete": wrapAction(client, users.Delete),
		// Extended SSH keys
		"ssh_keys_for_user":       wrapAction(client, users.ListSSHKeysForUser),
		"get_ssh_key":             wrapAction(client, users.GetSSHKey),
		"get_ssh_key_for_user":    wrapAction(client, users.GetSSHKeyForUser),
		"add_ssh_key":             wrapAction(client, users.AddSSHKey),
		"add_ssh_key_for_user":    wrapAction(client, users.AddSSHKeyForUser),
		"delete_ssh_key":          wrapAction(client, users.DeleteSSHKey),
		"delete_ssh_key_for_user": wrapAction(client, users.DeleteSSHKeyForUser),
		// Misc user tools
		"current_user_status": wrapAction(client, users.CurrentUserStatus),
		"activities":          wrapAction(client, users.GetUserActivities),
		"memberships":         wrapAction(client, users.GetUserMemberships),
		"create_runner":       wrapAction(client, users.CreateUserRunner),
		"delete_identity":     wrapAction(client, users.DeleteUserIdentity),
		// GPG keys
		"gpg_keys":                wrapAction(client, usergpgkeys.List),
		"gpg_keys_for_user":       wrapAction(client, usergpgkeys.ListForUser),
		"get_gpg_key":             wrapAction(client, usergpgkeys.Get),
		"get_gpg_key_for_user":    wrapAction(client, usergpgkeys.GetForUser),
		"add_gpg_key":             wrapAction(client, usergpgkeys.Add),
		"add_gpg_key_for_user":    wrapAction(client, usergpgkeys.AddForUser),
		"delete_gpg_key":          wrapAction(client, usergpgkeys.Delete),
		"delete_gpg_key_for_user": wrapAction(client, usergpgkeys.DeleteForUser),
		// Emails (extended)
		"emails_for_user":       wrapAction(client, useremails.ListForUser),
		"get_email":             wrapAction(client, useremails.Get),
		"add_email":             wrapAction(client, useremails.Add),
		"add_email_for_user":    wrapAction(client, useremails.AddForUser),
		"delete_email":          wrapAction(client, useremails.Delete),
		"delete_email_for_user": wrapAction(client, useremails.DeleteForUser),
		// Impersonation tokens
		"list_impersonation_tokens":    wrapAction(client, impersonationtokens.List),
		"get_impersonation_token":      wrapAction(client, impersonationtokens.Get),
		"create_impersonation_token":   wrapAction(client, impersonationtokens.Create),
		"revoke_impersonation_token":   wrapAction(client, impersonationtokens.Revoke),
		"create_personal_access_token": wrapAction(client, impersonationtokens.CreatePAT),
		// Service accounts
		"create_service_account":  wrapAction(client, users.CreateServiceAccount),
		"list_service_accounts":   wrapAction(client, users.ListServiceAccounts),
		"create_current_user_pat": wrapAction(client, users.CreateCurrentUserPAT),
	}

	addMetaTool(server, "gitlab_user", `GitLab user, SSH keys, GPG keys, emails, impersonation tokens, service accounts, to-do, namespace, and notification operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- current: Get information about the currently authenticated user. Params: (none required)
- me: Alias for current — get the authenticated user. Params: (none required)
- list: List GitLab users with optional filters. Params: search, username, active (bool), blocked (bool), external (bool), order_by, sort, page, per_page
- get: Get a single user by ID. Params: user_id (required, int)
- get_status: Get a user's status (emoji, message, availability). Params: user_id (required, int)
- set_status: Set current user's status. Params: emoji, message, availability (not_set/busy), clear_status_after (30_minutes/3_hours/8_hours/1_day/3_days/7_days/30_days)
- ssh_keys: List SSH keys for the current user. Params: page, per_page
- emails: List email addresses for the current user. Params: (none)
- contribution_events: List contribution events for a user. Params: user_id (required, int), action, target_type, before (YYYY-MM-DD), after (YYYY-MM-DD), sort, page, per_page
- associations_count: Get count of user's groups, projects, issues, and merge requests. Params: user_id (required, int)
- block: Block a user (admin). Params: user_id (required, int)
- unblock: Unblock a user (admin). Params: user_id (required, int)
- ban: Ban a user (admin). Params: user_id (required, int)
- unban: Unban a user (admin). Params: user_id (required, int)
- activate: Activate a user (admin). Params: user_id (required, int)
- deactivate: Deactivate a user (admin). Params: user_id (required, int)
- approve: Approve a user (admin). Params: user_id (required, int)
- reject: Reject a user (admin). Params: user_id (required, int)
- disable_two_factor: Disable 2FA for a user (admin). Params: user_id (required, int)
- create: Create a new user (admin). Params: email (required), name (required), username (required), password, reset_password (bool), force_random_password (bool), skip_confirmation (bool), admin (bool), external (bool), bio, location, job_title, organization, projects_limit (int), note
- modify: Modify an existing user (admin). Params: user_id (required, int), email, name, username, bio, location, job_title, organization, projects_limit (int), admin (bool), external (bool), note
- delete: Delete a user (admin). Params: user_id (required, int)
- ssh_keys_for_user: List SSH keys for a specific user. Params: user_id (required, int), page, per_page
- get_ssh_key: Get SSH key by ID. Params: key_id (required, int)
- get_ssh_key_for_user: Get SSH key for a user. Params: user_id (required, int), key_id (required, int)
- add_ssh_key: Add SSH key to current user. Params: title (required), key (required), expires_at (YYYY-MM-DD), usage_type (auth/signing)
- add_ssh_key_for_user: Add SSH key to a user (admin). Params: user_id (required, int), title (required), key (required), expires_at, usage_type
- delete_ssh_key: Delete SSH key from current user. Params: key_id (required, int)
- delete_ssh_key_for_user: Delete SSH key from user (admin). Params: user_id (required, int), key_id (required, int)
- current_user_status: Get current user's status. Params: (none)
- activities: List user activities (admin). Params: from (YYYY-MM-DD), page, per_page
- memberships: List a user's memberships. Params: user_id (required, int), type (Project/Namespace), page, per_page
- create_runner: Create a runner for current user. Params: runner_type (required), group_id (int), project_id (int), description, paused (bool), locked (bool), run_untagged (bool), tag_list, access_level, maximum_timeout (int), maintenance_note
- delete_identity: Delete a user's identity (admin). Params: user_id (required, int), provider (required)
- gpg_keys: List GPG keys for the current user. Params: (none)
- gpg_keys_for_user: List GPG keys for a user. Params: user_id (required, int)
- get_gpg_key: Get a GPG key by ID. Params: key_id (required, int)
- get_gpg_key_for_user: Get a GPG key for a user. Params: user_id (required, int), key_id (required, int)
- add_gpg_key: Add a GPG key to current user. Params: key (required, armored GPG public key)
- add_gpg_key_for_user: Add a GPG key to a user (admin). Params: user_id (required, int), key (required)
- delete_gpg_key: Delete a GPG key from current user. Params: key_id (required, int)
- delete_gpg_key_for_user: Delete a GPG key from a user (admin). Params: user_id (required, int), key_id (required, int)
- emails_for_user: List emails for a user. Params: user_id (required, int), page, per_page
- get_email: Get an email by ID. Params: email_id (required, int)
- add_email: Add email to current user. Params: email (required), skip_confirmation (bool)
- add_email_for_user: Add email to a user (admin). Params: user_id (required, int), email (required), skip_confirmation (bool)
- delete_email: Delete email from current user. Params: email_id (required, int)
- delete_email_for_user: Delete email from a user (admin). Params: user_id (required, int), email_id (required, int)
- list_impersonation_tokens: List impersonation tokens for a user. Params: user_id (required, int), state (active/inactive), page, per_page
- get_impersonation_token: Get an impersonation token. Params: user_id (required, int), token_id (required, int)
- create_impersonation_token: Create impersonation token (admin). Params: user_id (required, int), name (required), scopes (required, array), expires_at (YYYY-MM-DD)
- revoke_impersonation_token: Revoke impersonation token (admin). Params: user_id (required, int), token_id (required, int)
- create_personal_access_token: Create PAT for a user (admin). Params: user_id (required, int), name (required), scopes (required, array), description, expires_at (YYYY-MM-DD)
- create_service_account: Create a service account. Params: name, username, email
- list_service_accounts: List service accounts. Params: order_by, sort, page, per_page
- create_current_user_pat: Create PAT for current user. Params: name (required), scopes (required, array), description, expires_at (YYYY-MM-DD)
- todo_list: List to-do items with optional filters. Params: action (assigned/mentioned/build_failed/marked/approval_required/directly_addressed), author_id (int), project_id (int), group_id (int), state (pending/done), type (Issue/MergeRequest/DesignManagement::Design/AlertManagement::Alert), page, per_page
- todo_mark_done: Mark a single to-do item as done. Params: id (required, int)
- todo_mark_all_done: Mark all pending to-do items as done. Params: (none)
- event_list_project: List visible events for a project. Params: project_id (required), action, target_type, before (YYYY-MM-DD), after (YYYY-MM-DD), sort, page, per_page
- event_list_contributions: List current user's contribution events. Params: action, target_type, before, after, sort, page, per_page
- notification_global_get: Get global notification settings. Params: (none)
- notification_project_get: Get project notification settings. Params: project_id (required)
- notification_group_get: Get group notification settings. Params: group_id (required)
- notification_global_update: Update global notification settings. Params: level, notification_email, new_note, new_issue, reopen_issue, close_issue, merge_merge_request, etc.
- notification_project_update: Update project notification settings. Params: project_id (required), level, new_note, etc.
- notification_group_update: Update group notification settings. Params: group_id (required), level, new_note, etc.
- key_get_with_user: Get an SSH key by ID with user info. Params: key_id (required)
- key_get_by_fingerprint: Get an SSH key by fingerprint. Params: fingerprint (required)
- namespace_list: List namespaces accessible by the user. Params: search, owned_only (bool), page, per_page
- namespace_get: Get a namespace by ID or path. Params: namespace_id (required)
- namespace_exists: Check if a namespace path exists. Params: namespace (required), parent_id
- namespace_search: Search namespaces. Params: search (required)
- avatar_get: Get an avatar URL by email. Params: email (required), size (int)`, routes, metaAnnotations, toolutil.IconUser)
}

// registerWikiMeta registers the gitlab_wiki meta-tool with actions:
// list, get, create, update, delete, upload_attachment.
func registerWikiMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":              wrapAction(client, wikis.List),
		"get":               wrapAction(client, wikis.Get),
		"create":            wrapAction(client, wikis.Create),
		"update":            wrapAction(client, wikis.Update),
		"delete":            wrapVoidAction(client, wikis.Delete),
		"upload_attachment": wrapAction(client, wikis.UploadAttachment),
	}

	addMetaTool(server, "gitlab_wiki", `Manage GitLab project wiki pages. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List all wiki pages in a project. Params: project_id (required), with_content (bool, include page content)
- get: Get a single wiki page by slug. Params: project_id (required), slug (required), render_html (bool), version (string, SHA for specific revision)
- create: Create a new wiki page. Params: project_id (required), title (required), content (required), format (markdown/rdoc/asciidoc/org)
- update: Update an existing wiki page. Params: project_id (required), slug (required), title, content, format
- delete: Delete a wiki page. Params: project_id (required), slug (required)
- upload_attachment: Upload a file attachment to a wiki. Params: project_id (required), filename (required), content_base64 or file_path (one required), branch (optional)`, routes, metaAnnotations, toolutil.IconWiki)
}

// registerEnvironmentMeta registers the gitlab_environment meta-tool with actions:
// list, get, create, update, delete, stop.
func registerEnvironmentMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                wrapAction(client, environments.List),
		"get":                 wrapAction(client, environments.Get),
		"create":              wrapAction(client, environments.Create),
		"update":              wrapAction(client, environments.Update),
		"delete":              wrapVoidAction(client, environments.Delete),
		"stop":                wrapAction(client, environments.Stop),
		"protected_list":      wrapAction(client, protectedenvs.List),
		"protected_get":       wrapAction(client, protectedenvs.Get),
		"protected_protect":   wrapAction(client, protectedenvs.Protect),
		"protected_update":    wrapAction(client, protectedenvs.Update),
		"protected_unprotect": wrapVoidAction(client, protectedenvs.Unprotect),
		"freeze_list":         wrapAction(client, freezeperiods.List),
		"freeze_get":          wrapAction(client, freezeperiods.Get),
		"freeze_create":       wrapAction(client, freezeperiods.Create),
		"freeze_update":       wrapAction(client, freezeperiods.Update),
		"freeze_delete":       wrapVoidAction(client, freezeperiods.Delete),
	}

	addMetaTool(server, "gitlab_environment", `Manage environments in a GitLab project. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List environments. Params: project_id (required), name, search, states, page, per_page
- get: Get environment details. Params: project_id (required), environment_id (required, int)
- create: Create an environment. Params: project_id (required), name (required), description, external_url, tier
- update: Update an environment. Params: project_id (required), environment_id (required, int), name, description, external_url, tier
- delete: Delete an environment. Params: project_id (required), environment_id (required, int)
- stop: Stop a running environment. Params: project_id (required), environment_id (required, int), force (bool)
- protected_list: List protected environments. Params: project_id (required), page, per_page
- protected_get: Get a protected environment. Params: project_id (required), name (required)
- protected_protect: Protect an environment. Params: project_id (required), name (required), deploy_access_levels, approval_rules
- protected_update: Update a protected environment. Params: project_id (required), name (required), deploy_access_levels, approval_rules
- protected_unprotect: Unprotect an environment. Params: project_id (required), name (required)
- freeze_list: List deploy freeze periods. Params: project_id (required), page, per_page
- freeze_get: Get a freeze period. Params: project_id (required), freeze_period_id (required)
- freeze_create: Create a freeze period. Params: project_id (required), freeze_start (required, cron), freeze_end (required, cron), cron_timezone
- freeze_update: Update a freeze period. Params: project_id (required), freeze_period_id (required), freeze_start, freeze_end, cron_timezone
- freeze_delete: Delete a freeze period. Params: project_id (required), freeze_period_id (required)`, routes, metaAnnotations, toolutil.IconEnvironment)
}

// registerDeploymentMeta registers the gitlab_deployment meta-tool with actions:
// list, get, create, update, delete, approve_or_reject.
func registerDeploymentMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":              wrapAction(client, deployments.List),
		"get":               wrapAction(client, deployments.Get),
		"create":            wrapAction(client, deployments.Create),
		"update":            wrapAction(client, deployments.Update),
		"delete":            wrapVoidAction(client, deployments.Delete),
		"approve_or_reject": wrapAction(client, deployments.ApproveOrReject),
		"merge_requests":    wrapAction(client, deploymentmergerequests.List),
	}

	addMetaTool(server, "gitlab_deployment", `Manage deployments in a GitLab project. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List deployments. Params: project_id (required), order_by, sort, environment, status, page, per_page
- get: Get deployment details. Params: project_id (required), deployment_id (required, int)
- create: Create a deployment. Params: project_id (required), environment (required), ref (required), sha (required), tag (bool), status
- update: Update deployment status. Params: project_id (required), deployment_id (required, int), status (required)
- delete: Delete a deployment. Params: project_id (required), deployment_id (required, int)
- approve_or_reject: Approve or reject a blocked deployment. Params: project_id (required), deployment_id (required, int), status (required, approved/rejected), comment
- merge_requests: List merge requests associated with a deployment. Params: project_id (required), deployment_id (required, int), state, order_by, sort, page, per_page`, routes, metaAnnotations, toolutil.IconDeploy)
}

// registerPipelineScheduleMeta registers the gitlab_pipeline_schedule meta-tool with actions:
// list, get, create, update, delete, run, take_ownership, create_variable, edit_variable, delete_variable, list_triggered_pipelines.
func registerPipelineScheduleMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                     wrapAction(client, pipelineschedules.List),
		"get":                      wrapAction(client, pipelineschedules.Get),
		"create":                   wrapAction(client, pipelineschedules.Create),
		"update":                   wrapAction(client, pipelineschedules.Update),
		"delete":                   wrapVoidAction(client, pipelineschedules.Delete),
		"run":                      wrapAction(client, pipelineschedules.Run),
		"take_ownership":           wrapAction(client, pipelineschedules.TakeOwnership),
		"create_variable":          wrapAction(client, pipelineschedules.CreateVariable),
		"edit_variable":            wrapAction(client, pipelineschedules.EditVariable),
		"delete_variable":          wrapVoidAction(client, pipelineschedules.DeleteVariable),
		"list_triggered_pipelines": wrapAction(client, pipelineschedules.ListTriggeredPipelines),
	}

	addMetaTool(server, "gitlab_pipeline_schedule", `Manage pipeline schedules in a GitLab project. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List pipeline schedules. Params: project_id (required), scope (active/inactive), page, per_page
- get: Get schedule details. Params: project_id (required), schedule_id (required, int)
- create: Create a schedule. Params: project_id (required), description (required), ref (required), cron (required), cron_timezone, active (bool)
- update: Update a schedule. Params: project_id (required), schedule_id (required, int), description, ref, cron, cron_timezone, active (bool)
- delete: Delete a schedule. Params: project_id (required), schedule_id (required, int)
- run: Trigger immediate run. Params: project_id (required), schedule_id (required, int)
- take_ownership: Take ownership of a schedule. Params: project_id (required), schedule_id (required, int)
- create_variable: Create a schedule variable. Params: project_id (required), schedule_id (required, int), key (required), value (required), variable_type (env_var/file)
- edit_variable: Edit a schedule variable. Params: project_id (required), schedule_id (required, int), key (required), value (required), variable_type
- delete_variable: Delete a schedule variable. Params: project_id (required), schedule_id (required, int), key (required)
- list_triggered_pipelines: List pipelines triggered by schedule. Params: project_id (required), schedule_id (required, int), page, per_page`, routes, metaAnnotations, toolutil.IconSchedule)
}

// registerCIVariableMeta registers the gitlab_ci_variable meta-tool with actions:
// list, get, create, update, delete.
func registerCIVariableMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":            wrapAction(client, civariables.List),
		"get":             wrapAction(client, civariables.Get),
		"create":          wrapAction(client, civariables.Create),
		"update":          wrapAction(client, civariables.Update),
		"delete":          wrapVoidAction(client, civariables.Delete),
		"group_list":      wrapAction(client, groupvariables.List),
		"group_get":       wrapAction(client, groupvariables.Get),
		"group_create":    wrapAction(client, groupvariables.Create),
		"group_update":    wrapAction(client, groupvariables.Update),
		"group_delete":    wrapVoidAction(client, groupvariables.Delete),
		"instance_list":   wrapAction(client, instancevariables.List),
		"instance_get":    wrapAction(client, instancevariables.Get),
		"instance_create": wrapAction(client, instancevariables.Create),
		"instance_update": wrapAction(client, instancevariables.Update),
		"instance_delete": wrapVoidAction(client, instancevariables.Delete),
	}

	addMetaTool(server, "gitlab_ci_variable", `Manage CI/CD variables in a GitLab project. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- list: List variables. Params: project_id (required), page, per_page
- get: Get variable by key. Params: project_id (required), key (required), environment_scope
- create: Create variable. Params: project_id (required), key (required), value (required), description, variable_type, protected (bool), masked (bool), masked_and_hidden (bool), raw (bool), environment_scope
- update: Update variable. Params: project_id (required), key (required), value, description, variable_type, protected (bool), masked (bool), raw (bool), environment_scope
- delete: Delete variable. Params: project_id (required), key (required), environment_scope
- group_list: List group CI/CD variables. Params: group_id (required), page, per_page
- group_get: Get a group variable. Params: group_id (required), key (required)
- group_create: Create a group variable. Params: group_id (required), key (required), value (required), description, variable_type, protected (bool), masked (bool), raw (bool), environment_scope
- group_update: Update a group variable. Params: group_id (required), key (required), value, description, variable_type, protected (bool), masked (bool), raw (bool), environment_scope
- group_delete: Delete a group variable. Params: group_id (required), key (required)
- instance_list: List instance-level CI/CD variables. Params: page, per_page
- instance_get: Get an instance variable. Params: key (required)
- instance_create: Create an instance variable. Params: key (required), value (required), description, variable_type, protected (bool), masked (bool), raw (bool)
- instance_update: Update an instance variable. Params: key (required), value, description, variable_type, protected (bool), masked (bool), raw (bool)
- instance_delete: Delete an instance variable. Params: key (required)`, routes, metaAnnotations, toolutil.IconVariable)
}

// registerTemplateMeta registers the gitlab_template meta-tool with actions:
// lint, lint_project, ci_yml_list, ci_yml_get, dockerfile_list, dockerfile_get,
// gitignore_list, gitignore_get, license_list, license_get, project_template_list, project_template_get.
func registerTemplateMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"lint":                  wrapAction(client, cilint.LintContent),
		"lint_project":          wrapAction(client, cilint.LintProject),
		"ci_yml_list":           wrapAction(client, ciyamltemplates.List),
		"ci_yml_get":            wrapAction(client, ciyamltemplates.Get),
		"dockerfile_list":       wrapAction(client, dockerfiletemplates.List),
		"dockerfile_get":        wrapAction(client, dockerfiletemplates.Get),
		"gitignore_list":        wrapAction(client, gitignoretemplates.List),
		"gitignore_get":         wrapAction(client, gitignoretemplates.Get),
		"license_list":          wrapAction(client, licensetemplates.List),
		"license_get":           wrapAction(client, licensetemplates.Get),
		"project_template_list": wrapAction(client, projecttemplates.List),
		"project_template_get":  wrapAction(client, projecttemplates.Get),
	}

	addMetaTool(server, "gitlab_template", `GitLab CI/CD templates and configuration validation. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- lint: Validate arbitrary YAML content in project namespace. Params: project_id (required), content (required), dry_run (bool), include_jobs (bool), ref
- lint_project: Validate a project's existing .gitlab-ci.yml. Params: project_id (required), content_ref, dry_run (bool), dry_run_ref, include_jobs (bool), ref
- ci_yml_list: List all CI/CD YAML templates. Params: page, per_page
- ci_yml_get: Get a CI/CD YAML template by key. Params: key (required)
- dockerfile_list: List all Dockerfile templates. Params: page, per_page
- dockerfile_get: Get a Dockerfile template by key. Params: key (required)
- gitignore_list: List all .gitignore templates. Params: page, per_page
- gitignore_get: Get a .gitignore template by key. Params: key (required)
- license_list: List all license templates. Params: page, per_page, popular (bool)
- license_get: Get a license template by key. Params: key (required), project, fullname
- project_template_list: List project templates of a given type. Params: project_id (required), template_type (required), page, per_page
- project_template_get: Get a single project template. Params: project_id (required), template_type (required), key (required)`, routes, readOnlyMetaAnnotations, toolutil.IconTemplate)
}

// registerAdminMeta registers the gitlab_admin meta-tool with actions:
// topic_list, topic_get, topic_create, topic_update, topic_delete,
// settings_get, settings_update, appearance_get, appearance_update.
func registerAdminMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"topic_list":                     wrapAction(client, topics.List),
		"topic_get":                      wrapAction(client, topics.Get),
		"topic_create":                   wrapAction(client, topics.Create),
		"topic_update":                   wrapAction(client, topics.Update),
		"topic_delete":                   wrapVoidAction(client, topics.Delete),
		"settings_get":                   wrapAction(client, settings.Get),
		"settings_update":                wrapAction(client, settings.Update),
		"appearance_get":                 wrapAction(client, appearance.Get),
		"appearance_update":              wrapAction(client, appearance.Update),
		"broadcast_message_list":         wrapAction(client, broadcastmessages.List),
		"broadcast_message_get":          wrapAction(client, broadcastmessages.Get),
		"broadcast_message_create":       wrapAction(client, broadcastmessages.Create),
		"broadcast_message_update":       wrapAction(client, broadcastmessages.Update),
		"broadcast_message_delete":       wrapVoidAction(client, broadcastmessages.Delete),
		"feature_list":                   wrapAction(client, features.List),
		"feature_list_definitions":       wrapAction(client, features.ListDefinitions),
		"feature_set":                    wrapAction(client, features.Set),
		"feature_delete":                 wrapVoidAction(client, features.Delete),
		"license_get":                    wrapAction(client, license.Get),
		"license_add":                    wrapAction(client, license.Add),
		"license_delete":                 wrapVoidAction(client, license.Delete),
		"system_hook_list":               wrapAction(client, systemhooks.List),
		"system_hook_get":                wrapAction(client, systemhooks.Get),
		"system_hook_add":                wrapAction(client, systemhooks.Add),
		"system_hook_test":               wrapAction(client, systemhooks.Test),
		"system_hook_delete":             wrapVoidAction(client, systemhooks.Delete),
		"sidekiq_queue_metrics":          wrapAction(client, sidekiq.GetQueueMetrics),
		"sidekiq_process_metrics":        wrapAction(client, sidekiq.GetProcessMetrics),
		"sidekiq_job_stats":              wrapAction(client, sidekiq.GetJobStats),
		"sidekiq_compound_metrics":       wrapAction(client, sidekiq.GetCompoundMetrics),
		"plan_limits_get":                wrapAction(client, planlimits.Get),
		"plan_limits_change":             wrapAction(client, planlimits.Change),
		"usage_data_service_ping":        wrapAction(client, usagedata.GetServicePing),
		"usage_data_non_sql_metrics":     wrapAction(client, usagedata.GetNonSQLMetrics),
		"usage_data_queries":             wrapAction(client, usagedata.GetQueries),
		"usage_data_metric_definitions":  wrapAction(client, usagedata.GetMetricDefinitions),
		"usage_data_track_event":         wrapAction(client, usagedata.TrackEvent),
		"usage_data_track_events":        wrapAction(client, usagedata.TrackEvents),
		"db_migration_mark":              wrapAction(client, dbmigrations.Mark),
		"application_list":               wrapAction(client, applications.List),
		"application_create":             wrapAction(client, applications.Create),
		"application_delete":             wrapVoidAction(client, applications.Delete),
		"app_statistics_get":             wrapAction(client, appstatistics.Get),
		"metadata_get":                   wrapAction(client, metadata.Get),
		"custom_attr_list":               wrapAction(client, customattributes.List),
		"custom_attr_get":                wrapAction(client, customattributes.Get),
		"custom_attr_set":                wrapAction(client, customattributes.Set),
		"custom_attr_delete":             wrapVoidAction(client, customattributes.Delete),
		"bulk_import_start":              wrapAction(client, bulkimports.StartMigration),
		"error_tracking_list":            wrapAction(client, errortracking.ListClientKeys),
		"error_tracking_create":          wrapAction(client, errortracking.CreateClientKey),
		"error_tracking_delete":          wrapVoidAction(client, errortracking.DeleteClientKey),
		"error_tracking_get_settings":    wrapAction(client, errortracking.GetSettings),
		"error_tracking_update_settings": wrapAction(client, errortracking.EnableDisable),
		"alert_metric_image_list":        wrapAction(client, alertmanagement.ListMetricImages),
		"alert_metric_image_upload":      wrapAction(client, alertmanagement.UploadMetricImage),
		"alert_metric_image_update":      wrapAction(client, alertmanagement.UpdateMetricImage),
		"alert_metric_image_delete":      wrapVoidAction(client, alertmanagement.DeleteMetricImage),
		"secure_file_list":               wrapAction(client, securefiles.List),
		"secure_file_get":                wrapAction(client, securefiles.Show),
		"secure_file_create":             wrapAction(client, securefiles.Create),
		"secure_file_delete":             wrapVoidAction(client, securefiles.Remove),
		"terraform_state_list":           wrapAction(client, terraformstates.List),
		"terraform_state_get":            wrapAction(client, terraformstates.Get),
		"terraform_state_delete":         wrapVoidAction(client, terraformstates.Delete),
		"terraform_state_lock":           wrapAction(client, terraformstates.Lock),
		"terraform_state_unlock":         wrapAction(client, terraformstates.Unlock),
		"terraform_version_delete":       wrapVoidAction(client, terraformstates.DeleteVersion),
		"cluster_agent_list":             wrapAction(client, clusteragents.ListAgents),
		"cluster_agent_get":              wrapAction(client, clusteragents.GetAgent),
		"cluster_agent_register":         wrapAction(client, clusteragents.RegisterAgent),
		"cluster_agent_delete":           wrapVoidAction(client, clusteragents.DeleteAgent),
		"cluster_agent_token_list":       wrapAction(client, clusteragents.ListAgentTokens),
		"cluster_agent_token_get":        wrapAction(client, clusteragents.GetAgentToken),
		"cluster_agent_token_create":     wrapAction(client, clusteragents.CreateAgentToken),
		"cluster_agent_token_revoke":     wrapVoidAction(client, clusteragents.RevokeAgentToken),
		"dependency_proxy_delete":        wrapVoidAction(client, dependencyproxy.Purge),
		"import_github":                  wrapAction(client, importservice.ImportFromGitHub),
		"import_bitbucket":               wrapAction(client, importservice.ImportFromBitbucketCloud),
		"import_bitbucket_server":        wrapAction(client, importservice.ImportFromBitbucketServer),
		"import_cancel_github":           wrapAction(client, importservice.CancelGitHubImport),
		"import_gists":                   wrapVoidAction(client, importservice.ImportGists),
	}

	addMetaTool(server, "gitlab_admin", `GitLab admin and instance-level operations. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- topic_list: List project topics. Params: search, page, per_page
- topic_get: Get a project topic by ID. Params: topic_id (required)
- topic_create: Create a new project topic (admin). Params: name (required), title, description
- topic_update: Update a project topic (admin). Params: topic_id (required), name, title, description
- topic_delete: Delete a project topic (admin). Params: topic_id (required)
- settings_get: Get current application settings (admin). No params required
- settings_update: Update application settings (admin). Params: settings (map of setting_name to value)
- appearance_get: Get application appearance (admin). No params required
- appearance_update: Update application appearance (admin). Params: title, description, header_message, footer_message, message_background_color, message_font_color, email_header_and_footer_enabled, pwa_name, pwa_short_name, pwa_description, member_guidelines, new_project_guidelines, profile_image_guidelines
- broadcast_message_list: List broadcast messages (admin). Params: page, per_page
- broadcast_message_get: Get a broadcast message (admin). Params: id (required)
- broadcast_message_create: Create a broadcast message (admin). Params: message (required), starts_at, ends_at, broadcast_type, theme, dismissable, target_path, target_access_levels
- broadcast_message_update: Update a broadcast message (admin). Params: id (required), message, starts_at, ends_at, broadcast_type, theme, dismissable
- broadcast_message_delete: Delete a broadcast message (admin). Params: id (required)
- feature_list: List all feature flags (admin). No params.
- feature_list_definitions: List all feature definitions (admin). No params.
- feature_set: Set or create a feature flag (admin). Params: name (required), value (required), key, feature_group, user, group, namespace, project, repository, force
- feature_delete: Delete a feature flag (admin). Params: name (required)
- license_get: Get current GitLab license (admin). No params.
- license_add: Add a new GitLab license (admin). Params: license (required, Base64-encoded)
- license_delete: Delete a GitLab license (admin). Params: id (required)
- system_hook_list: List all system hooks (admin). No params.
- system_hook_get: Get a system hook (admin). Params: id (required)
- system_hook_add: Add a system hook (admin). Params: url (required), token, push_events, tag_push_events, merge_requests_events, repository_update_events, enable_ssl_verification
- system_hook_test: Test a system hook (admin). Params: id (required)
- system_hook_delete: Delete a system hook (admin). Params: id (required)
- sidekiq_queue_metrics: Get Sidekiq queue metrics (admin). No params.
- sidekiq_process_metrics: Get Sidekiq process metrics (admin). No params.
- sidekiq_job_stats: Get Sidekiq job statistics (admin). No params.
- sidekiq_compound_metrics: Get all Sidekiq metrics combined (admin). No params.
- plan_limits_get: Get current plan limits (admin). Params: plan_name (optional)
- plan_limits_change: Change plan limits (admin). Params: plan_name (required), conan_max_file_size, generic_packages_max_file_size, helm_max_file_size, maven_max_file_size, npm_max_file_size, nuget_max_file_size, pypi_max_file_size, terraform_module_max_file_size
- usage_data_service_ping: Get service ping data (admin). No params.
- usage_data_non_sql_metrics: Get non-SQL service ping metrics (admin). No params.
- usage_data_queries: Get service ping SQL queries (admin). No params.
- usage_data_metric_definitions: Get metric definitions as YAML (admin). No params.
- usage_data_track_event: Track a usage event. Params: event (required), send_to_snowplow, namespace_id, project_id
- usage_data_track_events: Track multiple usage events. Params: events (required, array)
- db_migration_mark: Mark a pending migration as successful (admin). Params: version (required), database (optional)
- application_list: List all OAuth2 applications (admin). Params: page, per_page
- application_create: Create an OAuth2 application (admin). Params: name (required), redirect_uri (required), scopes (required), confidential
- application_delete: Delete an OAuth2 application (admin). Params: id (required)
- app_statistics_get: Get application statistics (admin). No params.
- metadata_get: Get GitLab instance metadata (version, revision, KAS, enterprise). No params.
- custom_attr_list: List custom attributes for a resource (admin). Params: resource_type (required: user/group/project), resource_id (required)
- custom_attr_get: Get a custom attribute by key (admin). Params: resource_type (required), resource_id (required), key (required)
- custom_attr_set: Set (create/update) a custom attribute (admin). Params: resource_type (required), resource_id (required), key (required), value (required)
- custom_attr_delete: Delete a custom attribute (admin). Params: resource_type (required), resource_id (required), key (required)
- bulk_import_start: Start a bulk import migration (admin). Params: url (required, source GitLab URL), access_token (required), entities (required, array of {source_type, source_full_path, destination_slug, destination_namespace, migrate_projects, migrate_memberships})
- error_tracking_list: List error tracking client keys. Params: project_id (required), page, per_page
- error_tracking_create: Create error tracking client key. Params: project_id (required)
- error_tracking_delete: Delete error tracking client key. Params: project_id (required), key_id (required)
- error_tracking_get_settings: Get error tracking settings. Params: project_id (required)
- error_tracking_update_settings: Enable/disable error tracking. Params: project_id (required), active, integrated
- alert_metric_image_list: List alert metric images. Params: project_id (required), alert_iid (required), page, per_page
- alert_metric_image_upload: Upload alert metric image. Params: project_id (required), alert_iid (required), url (required), url_text
- alert_metric_image_update: Update alert metric image. Params: project_id (required), alert_iid (required), image_id (required), url, url_text
- alert_metric_image_delete: Delete alert metric image. Params: project_id (required), alert_iid (required), image_id (required)
- secure_file_list: List secure files. Params: project_id (required), page, per_page
- secure_file_get: Get a secure file. Params: project_id (required), file_id (required)
- secure_file_create: Create a secure file. Params: project_id (required), name (required), content (required, base64-encoded)
- secure_file_delete: Delete a secure file. Params: project_id (required), file_id (required)
- terraform_state_list: List Terraform states. Params: project_path (required, e.g. group/project)
- terraform_state_get: Get a Terraform state. Params: project_path (required, e.g. group/project), name (required)
- terraform_state_delete: Delete a Terraform state. Params: project_id (required), name (required)
- terraform_state_lock: Lock a Terraform state. Params: project_id (required), name (required)
- terraform_state_unlock: Unlock a Terraform state. Params: project_id (required), name (required)
- terraform_version_delete: Delete a Terraform state version. Params: project_id (required), name (required), serial (required)
- cluster_agent_list: List cluster agents. Params: project_id (required), page, per_page
- cluster_agent_get: Get a cluster agent. Params: project_id (required), agent_id (required)
- cluster_agent_register: Register a cluster agent. Params: project_id (required), name (required)
- cluster_agent_delete: Delete a cluster agent. Params: project_id (required), agent_id (required)
- cluster_agent_token_list: List cluster agent tokens. Params: project_id (required), agent_id (required), page, per_page
- cluster_agent_token_get: Get a cluster agent token. Params: project_id (required), agent_id (required), token_id (required)
- cluster_agent_token_create: Create a cluster agent token. Params: project_id (required), agent_id (required), name (required)
- cluster_agent_token_revoke: Revoke a cluster agent token. Params: project_id (required), agent_id (required), token_id (required)
- dependency_proxy_delete: Purge dependency proxy cache. Params: group_id (required)
- import_github: Import project from GitHub. Params: personal_access_token (required), repo_id (required), target_namespace (required), new_name
- import_bitbucket: Import project from Bitbucket Cloud. Params: bitbucket_username (required), bitbucket_app_password (required), repo_path (required), target_namespace (required), new_name
- import_bitbucket_server: Import from Bitbucket Server. Params: bitbucket_server_url (required), bitbucket_server_username (required), personal_access_token (required), bitbucket_server_project (required), bitbucket_server_repo (required), new_namespace, new_name
- import_cancel_github: Cancel a GitHub import. Params: project_id (required)
- import_gists: Import GitHub gists. Params: personal_access_token (required)`, routes, metaAnnotations, toolutil.IconServer)
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
	routes := map[string]actionFunc{
		"token_project_list":           wrapAction(client, accesstokens.ProjectList),
		"token_project_get":            wrapAction(client, accesstokens.ProjectGet),
		"token_project_create":         wrapAction(client, accesstokens.ProjectCreate),
		"token_project_rotate":         wrapAction(client, accesstokens.ProjectRotate),
		"token_project_rotate_self":    wrapAction(client, accesstokens.ProjectRotateSelf),
		"token_project_revoke":         wrapVoidAction(client, accesstokens.ProjectRevoke),
		"token_group_list":             wrapAction(client, accesstokens.GroupList),
		"token_group_get":              wrapAction(client, accesstokens.GroupGet),
		"token_group_create":           wrapAction(client, accesstokens.GroupCreate),
		"token_group_rotate":           wrapAction(client, accesstokens.GroupRotate),
		"token_group_rotate_self":      wrapAction(client, accesstokens.GroupRotateSelf),
		"token_group_revoke":           wrapVoidAction(client, accesstokens.GroupRevoke),
		"token_personal_list":          wrapAction(client, accesstokens.PersonalList),
		"token_personal_get":           wrapAction(client, accesstokens.PersonalGet),
		"token_personal_rotate":        wrapAction(client, accesstokens.PersonalRotate),
		"token_personal_rotate_self":   wrapAction(client, accesstokens.PersonalRotateSelf),
		"token_personal_revoke":        wrapVoidAction(client, accesstokens.PersonalRevoke),
		"token_personal_revoke_self":   wrapVoidAction(client, accesstokens.PersonalRevokeSelf),
		"deploy_token_list_all":        wrapAction(client, deploytokens.ListAll),
		"deploy_token_list_project":    wrapAction(client, deploytokens.ListProject),
		"deploy_token_list_group":      wrapAction(client, deploytokens.ListGroup),
		"deploy_token_get_project":     wrapAction(client, deploytokens.GetProject),
		"deploy_token_get_group":       wrapAction(client, deploytokens.GetGroup),
		"deploy_token_create_project":  wrapAction(client, deploytokens.CreateProject),
		"deploy_token_create_group":    wrapAction(client, deploytokens.CreateGroup),
		"deploy_token_delete_project":  wrapVoidAction(client, deploytokens.DeleteProject),
		"deploy_token_delete_group":    wrapVoidAction(client, deploytokens.DeleteGroup),
		"deploy_key_list_project":      wrapAction(client, deploykeys.ListProject),
		"deploy_key_get":               wrapAction(client, deploykeys.Get),
		"deploy_key_add":               wrapAction(client, deploykeys.Add),
		"deploy_key_update":            wrapAction(client, deploykeys.Update),
		"deploy_key_delete":            wrapVoidAction(client, deploykeys.Delete),
		"deploy_key_enable":            wrapAction(client, deploykeys.Enable),
		"deploy_key_list_all":          wrapAction(client, deploykeys.ListAll),
		"deploy_key_add_instance":      wrapAction(client, deploykeys.AddInstance),
		"deploy_key_list_user_project": wrapAction(client, deploykeys.ListUserProject),
		"request_list_project":         wrapAction(client, accessrequests.ListProject),
		"request_list_group":           wrapAction(client, accessrequests.ListGroup),
		"request_project":              wrapAction(client, accessrequests.RequestProject),
		"request_group":                wrapAction(client, accessrequests.RequestGroup),
		"approve_project":              wrapAction(client, accessrequests.ApproveProject),
		"approve_group":                wrapAction(client, accessrequests.ApproveGroup),
		"deny_project":                 wrapVoidAction(client, accessrequests.DenyProject),
		"deny_group":                   wrapVoidAction(client, accessrequests.DenyGroup),
		"invite_list_project":          wrapAction(client, invites.ListPendingProjectInvitations),
		"invite_list_group":            wrapAction(client, invites.ListPendingGroupInvitations),
		"invite_project":               wrapAction(client, invites.ProjectInvites),
		"invite_group":                 wrapAction(client, invites.GroupInvites),
	}
	addMetaTool(server, "gitlab_access", `Manage GitLab access tokens, deploy tokens, deploy keys, access requests, and invitations.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- token_project_list: List project access tokens. Params: project_id (required), page, per_page
- token_project_get: Get a project access token. Params: project_id (required), token_id (required)
- token_project_create: Create project access token. Params: project_id (required), name (required), scopes (required), expires_at, access_level
- token_project_rotate: Rotate project access token. Params: project_id (required), token_id (required), expires_at
- token_project_rotate_self: Rotate the calling project access token. Params: project_id (required), expires_at
- token_project_revoke: Revoke project access token. Params: project_id (required), token_id (required)
- token_group_list: List group access tokens. Params: group_id (required), page, per_page
- token_group_get: Get a group access token. Params: group_id (required), token_id (required)
- token_group_create: Create group access token. Params: group_id (required), name (required), scopes (required), expires_at, access_level
- token_group_rotate: Rotate group access token. Params: group_id (required), token_id (required), expires_at
- token_group_rotate_self: Rotate the calling group access token. Params: group_id (required), expires_at
- token_group_revoke: Revoke group access token. Params: group_id (required), token_id (required)
- token_personal_list: List personal access tokens. Params: user_id, page, per_page
- token_personal_get: Get a personal access token. Params: token_id (required)
- token_personal_rotate: Rotate personal access token. Params: token_id (required), expires_at
- token_personal_rotate_self: Rotate the calling personal access token. Params: expires_at
- token_personal_revoke: Revoke personal access token. Params: token_id (required)
- token_personal_revoke_self: Revoke the calling personal access token. Params: (none)
- deploy_token_list_all: List all deploy tokens (admin). Params: page, per_page
- deploy_token_list_project: List project deploy tokens. Params: project_id (required), page, per_page
- deploy_token_list_group: List group deploy tokens. Params: group_id (required), page, per_page
- deploy_token_get_project: Get project deploy token. Params: project_id (required), deploy_token_id (required)
- deploy_token_get_group: Get group deploy token. Params: group_id (required), deploy_token_id (required)
- deploy_token_create_project: Create project deploy token. Params: project_id (required), name (required), scopes (required), expires_at
- deploy_token_create_group: Create group deploy token. Params: group_id (required), name (required), scopes (required), expires_at
- deploy_token_delete_project: Delete project deploy token. Params: project_id (required), deploy_token_id (required)
- deploy_token_delete_group: Delete group deploy token. Params: group_id (required), deploy_token_id (required)
- deploy_key_list_project: List project deploy keys. Params: project_id (required), page, per_page
- deploy_key_get: Get a deploy key. Params: project_id (required), deploy_key_id (required)
- deploy_key_add: Add deploy key to project. Params: project_id (required), title (required), key (required), can_push
- deploy_key_update: Update a deploy key. Params: project_id (required), deploy_key_id (required), title, can_push
- deploy_key_delete: Delete a deploy key. Params: project_id (required), deploy_key_id (required)
- deploy_key_enable: Enable a deploy key for a project. Params: project_id (required), deploy_key_id (required)
- deploy_key_list_all: List all deploy keys (admin). Params: page, per_page
- deploy_key_add_instance: Add instance-level deploy key. Params: title (required), key (required)
- deploy_key_list_user_project: List deploy keys for a user project. Params: project_id (required), page, per_page
- request_list_project: List project access requests. Params: project_id (required), page, per_page
- request_list_group: List group access requests. Params: group_id (required), page, per_page
- request_project: Request access to project. Params: project_id (required)
- request_group: Request access to group. Params: group_id (required)
- approve_project: Approve project access request. Params: project_id (required), user_id (required), access_level
- approve_group: Approve group access request. Params: group_id (required), user_id (required), access_level
- deny_project: Deny project access request. Params: project_id (required), user_id (required)
- deny_group: Deny group access request. Params: group_id (required), user_id (required)
- invite_list_project: List pending project invitations. Params: project_id (required), page, per_page
- invite_list_group: List pending group invitations. Params: group_id (required), page, per_page
- invite_project: Invite members to project. Params: project_id (required), email (required), access_level (required), expires_at
- invite_group: Invite members to group. Params: group_id (required), email (required), access_level (required), expires_at`, routes, metaAnnotations, toolutil.IconToken)
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

	routes := map[string]actionFunc{
		"publish":                  publishAction,
		"download":                 downloadAction,
		"list":                     wrapAction(client, packages.List),
		"file_list":                wrapAction(client, packages.FileList),
		"delete":                   deleteAction,
		"file_delete":              fileDeleteAction,
		"publish_and_link":         publishAndLinkAction,
		"publish_directory":        publishDirAction,
		"registry_list_project":    wrapAction(client, containerregistry.ListProject),
		"registry_list_group":      wrapAction(client, containerregistry.ListGroup),
		"registry_get":             wrapAction(client, containerregistry.GetRepository),
		"registry_delete":          wrapVoidAction(client, containerregistry.DeleteRepository),
		"registry_tag_list":        wrapAction(client, containerregistry.ListTags),
		"registry_tag_get":         wrapAction(client, containerregistry.GetTag),
		"registry_tag_delete":      wrapVoidAction(client, containerregistry.DeleteTag),
		"registry_tag_delete_bulk": wrapVoidAction(client, containerregistry.DeleteTagsBulk),
		"registry_rule_list":       wrapAction(client, containerregistry.ListProtectionRules),
		"registry_rule_create":     wrapAction(client, containerregistry.CreateProtectionRule),
		"registry_rule_update":     wrapAction(client, containerregistry.UpdateProtectionRule),
		"registry_rule_delete":     wrapVoidAction(client, containerregistry.DeleteProtectionRule),
		"protection_rule_list":     wrapAction(client, protectedpackages.List),
		"protection_rule_create":   wrapAction(client, protectedpackages.Create),
		"protection_rule_update":   wrapAction(client, protectedpackages.Update),
		"protection_rule_delete":   wrapVoidAction(client, protectedpackages.Delete),
	}

	addMetaTool(server, "gitlab_package", `Manage GitLab Generic Package Registry and Container Registry. Use 'action' to specify the operation and 'params' for action-specific parameters.
Valid actions: `+validActionsString(routes)+`

Actions:
- publish: Upload a file to the package registry. Provide either file_path or content_base64, not both. Params: project_id (required), package_name (required), package_version (required), file_name (required), file_path or content_base64 (one required), status (optional: default/hidden)
- download: Download a package file to a local path. Params: project_id (required), package_name (required), package_version (required), file_name (required), output_path (required)
- list: List packages in a project with optional filtering. Params: project_id (required), package_name, package_version, package_type (generic/npm/maven/etc.), order_by (name/created_at/version/type), sort (asc/desc), page, per_page
- file_list: List files within a specific package. Params: project_id (required), package_id (required), page, per_page
- delete: Delete an entire package and all its files. Params: project_id (required), package_id (required)
- file_delete: Delete a single file from a package. Params: project_id (required), package_id (required), package_file_id (required)
- publish_and_link: Publish a file and create a release link pointing to it. Params: project_id (required), package_name (required), package_version (required), file_name (required), file_path or content_base64 (one required), tag_name (required), link_name (optional), link_type (optional: package/runbook/image/other), status (optional)
- publish_directory: Publish all matching files from a directory. Params: project_id (required), package_name (required), package_version (required), directory_path (required), include_pattern (optional glob), status (optional)
- registry_list_project: List project container registry repos. Params: project_id (required), tags (bool), tags_count (bool), page, per_page
- registry_list_group: List group container registry repos. Params: group_id (required), page, per_page
- registry_get: Get single container registry repo. Params: repository_id (required, int), tags (bool), tags_count (bool)
- registry_delete: Delete container registry repo. Params: project_id (required), repository_id (required, int)
- registry_tag_list: List tags for a container registry repo. Params: project_id (required), repository_id (required, int), page, per_page
- registry_tag_get: Get tag details. Params: project_id (required), repository_id (required, int), tag_name (required)
- registry_tag_delete: Delete a single tag. Params: project_id (required), repository_id (required, int), tag_name (required)
- registry_tag_delete_bulk: Bulk delete tags by regex. Params: project_id (required), repository_id (required, int), name_regex_delete, name_regex_keep, keep_n (int), older_than
- registry_rule_list: List container registry protection rules. Params: project_id (required)
- registry_rule_create: Create protection rule. Params: project_id (required), repository_path_pattern (required), minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_update: Update protection rule. Params: project_id (required), rule_id (required, int), repository_path_pattern, minimum_access_level_for_push, minimum_access_level_for_delete
- registry_rule_delete: Delete protection rule. Params: project_id (required), rule_id (required, int)
- protection_rule_list: List package protection rules. Params: project_id (required), page, per_page
- protection_rule_create: Create package protection rule. Params: project_id (required), package_name_pattern (required), package_type (required), minimum_access_level_for_push (maintainer/owner/admin), minimum_access_level_for_delete (maintainer/owner/admin)
- protection_rule_update: Update package protection rule. Params: project_id (required), rule_id (required, int), package_name_pattern, package_type, minimum_access_level_for_push, minimum_access_level_for_delete
- protection_rule_delete: Delete package protection rule. Params: project_id (required), rule_id (required, int)`, routes, metaAnnotations, toolutil.IconPackage)
}

// registerSnippetMeta registers the gitlab_snippet meta-tool with actions:
// list, list_all, get, content, file_content, create, update, delete, explore,
// project_list, project_get, project_content, project_create, project_update, project_delete,
// discussion_list, discussion_get, discussion_create, discussion_add_note,
// discussion_update_note, discussion_delete_note, note_list, note_get, note_create,
// note_update, and note_delete.
func registerSnippetMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                      wrapAction(client, snippets.List),
		"list_all":                  wrapAction(client, snippets.ListAll),
		"get":                       wrapAction(client, snippets.Get),
		"content":                   wrapAction(client, snippets.Content),
		"file_content":              wrapAction(client, snippets.FileContent),
		"create":                    wrapAction(client, snippets.Create),
		"update":                    wrapAction(client, snippets.Update),
		"delete":                    wrapVoidAction(client, snippets.Delete),
		"explore":                   wrapAction(client, snippets.Explore),
		"project_list":              wrapAction(client, snippets.ProjectList),
		"project_get":               wrapAction(client, snippets.ProjectGet),
		"project_content":           wrapAction(client, snippets.ProjectContent),
		"project_create":            wrapAction(client, snippets.ProjectCreate),
		"project_update":            wrapAction(client, snippets.ProjectUpdate),
		"project_delete":            wrapVoidAction(client, snippets.ProjectDelete),
		"discussion_list":           wrapAction(client, snippetdiscussions.List),
		"discussion_get":            wrapAction(client, snippetdiscussions.Get),
		"discussion_create":         wrapAction(client, snippetdiscussions.Create),
		"discussion_add_note":       wrapAction(client, snippetdiscussions.AddNote),
		"discussion_update_note":    wrapAction(client, snippetdiscussions.UpdateNote),
		"discussion_delete_note":    wrapVoidAction(client, snippetdiscussions.DeleteNote),
		"note_list":                 wrapAction(client, snippetnotes.List),
		"note_get":                  wrapAction(client, snippetnotes.Get),
		"note_create":               wrapAction(client, snippetnotes.Create),
		"note_update":               wrapAction(client, snippetnotes.Update),
		"note_delete":               wrapVoidAction(client, snippetnotes.Delete),
		"emoji_snippet_list":        wrapAction(client, awardemoji.ListSnippetAwardEmoji),
		"emoji_snippet_get":         wrapAction(client, awardemoji.GetSnippetAwardEmoji),
		"emoji_snippet_create":      wrapAction(client, awardemoji.CreateSnippetAwardEmoji),
		"emoji_snippet_delete":      wrapVoidAction(client, awardemoji.DeleteSnippetAwardEmoji),
		"emoji_snippet_note_list":   wrapAction(client, awardemoji.ListSnippetNoteAwardEmoji),
		"emoji_snippet_note_get":    wrapAction(client, awardemoji.GetSnippetNoteAwardEmoji),
		"emoji_snippet_note_create": wrapAction(client, awardemoji.CreateSnippetNoteAwardEmoji),
		"emoji_snippet_note_delete": wrapVoidAction(client, awardemoji.DeleteSnippetNoteAwardEmoji),
	}
	addMetaTool(server, "gitlab_snippet", `Manage GitLab snippets (personal and project), snippet discussions, and snippet notes.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List current user's snippets. Params: page, per_page
- list_all: List all public snippets. Params: page, per_page
- get: Get a snippet. Params: snippet_id (required)
- content: Get raw snippet content. Params: snippet_id (required)
- file_content: Get a specific file from a snippet. Params: snippet_id (required), file_path (required)
- create: Create a snippet. Params: title (required), file_name (required), content (required), visibility, description
- update: Update a snippet. Params: snippet_id (required), title, file_name, content, visibility, description
- delete: Delete a snippet. Params: snippet_id (required)
- explore: List all public snippets. Params: page, per_page
- project_list: List project snippets. Params: project_id (required), page, per_page
- project_get: Get a project snippet. Params: project_id (required), snippet_id (required)
- project_content: Get raw project snippet content. Params: project_id (required), snippet_id (required)
- project_create: Create a project snippet. Params: project_id (required), title (required), file_name (required), content (required), visibility
- project_update: Update a project snippet. Params: project_id (required), snippet_id (required), title, file_name, content, visibility
- project_delete: Delete a project snippet. Params: project_id (required), snippet_id (required)
- discussion_list: List snippet discussions. Params: snippet_id (required), page, per_page
- discussion_get: Get a snippet discussion. Params: snippet_id (required), discussion_id (required)
- discussion_create: Create a snippet discussion. Params: snippet_id (required), body (required)
- discussion_add_note: Add note to snippet discussion. Params: snippet_id (required), discussion_id (required), body (required)
- discussion_update_note: Update note in snippet discussion. Params: snippet_id (required), discussion_id (required), note_id (required), body (required)
- discussion_delete_note: Delete note from snippet discussion. Params: snippet_id (required), discussion_id (required), note_id (required)
- note_list: List snippet notes. Params: project_id (required), snippet_id (required), order_by, sort, page, per_page
- note_get: Get a snippet note. Params: project_id (required), snippet_id (required), note_id (required)
- note_create: Add a note to a snippet. Params: project_id (required), snippet_id (required), body (required)
- note_update: Update a snippet note. Params: project_id (required), snippet_id (required), note_id (required), body (required)
- note_delete: Delete a snippet note. Params: project_id (required), snippet_id (required), note_id (required)
- emoji_snippet_list: List award emoji on a snippet. Params: project_id (required), iid (required), page, per_page
- emoji_snippet_get: Get an award emoji on a snippet. Params: project_id (required), iid (required), award_id (required)
- emoji_snippet_create: Add award emoji to a snippet. Params: project_id (required), iid (required), name (required)
- emoji_snippet_delete: Remove award emoji from a snippet. Params: project_id (required), iid (required), award_id (required)
- emoji_snippet_note_list: List award emoji on a snippet note. Params: project_id (required), iid (required), note_id (required), page, per_page
- emoji_snippet_note_get: Get an award emoji on a snippet note. Params: project_id (required), iid (required), note_id (required), award_id (required)
- emoji_snippet_note_create: Add award emoji to a snippet note. Params: project_id (required), iid (required), note_id (required), name (required)
- emoji_snippet_note_delete: Delete award emoji from a snippet note. Params: project_id (required), iid (required), note_id (required), award_id (required)`, routes, metaAnnotations, toolutil.IconSnippet)
}

// registerFeatureFlagsMeta registers the gitlab_feature_flags meta-tool with actions:
// feature_flag_list, feature_flag_get, feature_flag_create, feature_flag_update, feature_flag_delete,
// ff_user_list_list, ff_user_list_get, ff_user_list_create, ff_user_list_update, and ff_user_list_delete.
func registerFeatureFlagsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"feature_flag_list":   wrapAction(client, featureflags.ListFeatureFlags),
		"feature_flag_get":    wrapAction(client, featureflags.GetFeatureFlag),
		"feature_flag_create": wrapAction(client, featureflags.CreateFeatureFlag),
		"feature_flag_update": wrapAction(client, featureflags.UpdateFeatureFlag),
		"feature_flag_delete": wrapVoidAction(client, featureflags.DeleteFeatureFlag),
		"ff_user_list_list":   wrapAction(client, ffuserlists.ListUserLists),
		"ff_user_list_get":    wrapAction(client, ffuserlists.GetUserList),
		"ff_user_list_create": wrapAction(client, ffuserlists.CreateUserList),
		"ff_user_list_update": wrapAction(client, ffuserlists.UpdateUserList),
		"ff_user_list_delete": wrapVoidAction(client, ffuserlists.DeleteUserList),
	}
	addMetaTool(server, "gitlab_feature_flags", `Manage GitLab feature flags and feature flag user lists.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- feature_flag_list: List feature flags. Params: project_id (required), scope (enabled/disabled), page, per_page
- feature_flag_get: Get a feature flag. Params: project_id (required), name (required)
- feature_flag_create: Create a feature flag. Params: project_id (required), name (required), version (required), description, active, strategies
- feature_flag_update: Update a feature flag. Params: project_id (required), name (required), description, active, strategies
- feature_flag_delete: Delete a feature flag. Params: project_id (required), name (required)
- ff_user_list_list: List feature flag user lists (named sets of user IDs). Params: project_id (required), page, per_page
- ff_user_list_get: Get a feature flag user list by IID. Params: project_id (required), iid (required)
- ff_user_list_create: Create a feature flag user list. Params: project_id (required), name (required), user_xids (required, comma-separated user identifiers)
- ff_user_list_update: Update a feature flag user list. Params: project_id (required), iid (required), name, user_xids
- ff_user_list_delete: Delete a feature flag user list. Params: project_id (required), iid (required)`, routes, metaAnnotations, toolutil.IconConfig)
}

// registerMergeTrainMeta registers the gitlab_merge_train meta-tool with actions
// for listing, getting, and adding merge requests to merge trains.
func registerMergeTrainMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list_project": wrapAction(client, mergetrains.ListProjectMergeTrains),
		"list_branch":  wrapAction(client, mergetrains.ListMergeRequestInMergeTrain),
		"get":          wrapAction(client, mergetrains.GetMergeRequestOnMergeTrain),
		"add":          wrapAction(client, mergetrains.AddMergeRequestToMergeTrain),
	}
	addMetaTool(server, "gitlab_merge_train", `Manage GitLab merge trains (automated merge queues for target branches).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list_project: List all merge trains for a project. Params: project_id (required), scope (active/complete), sort (asc/desc), page, per_page
- list_branch: List merge requests in a merge train for a specific branch. Params: project_id (required), target_branch (required), scope, sort, page, per_page
- get: Get the status of a merge request in a merge train. Params: project_id (required), merge_request_id (required)
- add: Add a merge request to a merge train. Params: project_id (required), merge_request_id (required), auto_merge (bool), sha (string), squash (bool)`, routes, metaAnnotations, toolutil.IconMR)
}

// registerAuditEventMeta registers the gitlab_audit_event meta-tool with actions
// for listing and getting audit events at instance, group, and project levels.
func registerAuditEventMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list_instance": wrapAction(client, auditevents.ListInstance),
		"get_instance":  wrapAction(client, auditevents.GetInstance),
		"list_group":    wrapAction(client, auditevents.ListGroup),
		"get_group":     wrapAction(client, auditevents.GetGroup),
		"list_project":  wrapAction(client, auditevents.ListProject),
		"get_project":   wrapAction(client, auditevents.GetProject),
	}
	addMetaTool(server, "gitlab_audit_event", `Manage GitLab audit events (instance, group, and project level).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list_instance: List instance-level audit events (admin only). Params: created_after (YYYY-MM-DD), created_before (YYYY-MM-DD), page, per_page
- get_instance: Get a single instance audit event. Params: event_id (required)
- list_group: List group audit events. Params: group_id (required), created_after, created_before, page, per_page
- get_group: Get a single group audit event. Params: group_id (required), event_id (required)
- list_project: List project audit events. Params: project_id (required), created_after, created_before, page, per_page
- get_project: Get a single project audit event. Params: project_id (required), event_id (required)`, routes, metaAnnotations, toolutil.IconEvent)
}

// registerDORAMetricsMeta registers the gitlab_dora_metrics meta-tool with actions
// for retrieving DORA metrics at project and group levels.
func registerDORAMetricsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"project": wrapAction(client, dorametrics.GetProjectMetrics),
		"group":   wrapAction(client, dorametrics.GetGroupMetrics),
	}
	addMetaTool(server, "gitlab_dora_metrics", `Get GitLab DORA metrics (deployment frequency, lead time, MTTR, change failure rate).
Use "action" to specify the scope. Valid actions: `+validActionsString(routes)+`

Actions:
- project: Get DORA metrics for a project. Params: project_id (required), metric (required: deployment_frequency|lead_time_for_changes|time_to_restore_service|change_failure_rate), start_date (YYYY-MM-DD), end_date (YYYY-MM-DD), interval (daily|monthly|all), environment_tiers (array)
- group: Get DORA metrics for a group. Params: group_id (required), metric (required), start_date, end_date, interval, environment_tiers`, routes, metaAnnotations, toolutil.IconAnalytics)
}

// registerDependencyMeta registers the gitlab_dependency meta-tool with actions
// for listing project dependencies and managing dependency list exports (SBOM).
func registerDependencyMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":            wrapAction(client, dependencies.ListDeps),
		"export_create":   wrapAction(client, dependencies.CreateExport),
		"export_get":      wrapAction(client, dependencies.GetExport),
		"export_download": wrapAction(client, dependencies.DownloadExport),
	}
	addMetaTool(server, "gitlab_dependency", `Manage GitLab project dependencies and SBOM exports.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List project dependencies. Params: project_id (required), package_manager (optional filter), page, per_page
- export_create: Create a dependency list export (SBOM) from a pipeline. Params: pipeline_id (required), export_type (default: sbom)
- export_get: Check status of a dependency list export. Params: export_id (required)
- export_download: Download a completed export (CycloneDX JSON, max 1MB). Params: export_id (required)`, routes, metaAnnotations, toolutil.IconPackage)
}

// registerExternalStatusCheckMeta registers the gitlab_external_status_check meta-tool with actions
// for managing external status checks on merge requests and projects (legacy + project-scoped APIs).
func registerExternalStatusCheckMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list_mr_checks":         wrapAction(client, externalstatuschecks.ListMergeStatusChecks),
		"set_status":             wrapVoidAction(client, externalstatuschecks.SetExternalStatusCheckStatus),
		"list_project_checks":    wrapAction(client, externalstatuschecks.ListProjectStatusChecks),
		"create":                 wrapVoidAction(client, externalstatuschecks.CreateExternalStatusCheck),
		"delete":                 wrapVoidAction(client, externalstatuschecks.DeleteExternalStatusCheck),
		"update":                 wrapVoidAction(client, externalstatuschecks.UpdateExternalStatusCheck),
		"retry":                  wrapVoidAction(client, externalstatuschecks.RetryFailedStatusCheckForMR),
		"list_project_mr_checks": wrapAction(client, externalstatuschecks.ListProjectMRExternalStatusChecks),
		"list_project":           wrapAction(client, externalstatuschecks.ListProjectExternalStatusChecks),
		"create_project":         wrapAction(client, externalstatuschecks.CreateProjectExternalStatusCheck),
		"delete_project":         wrapVoidAction(client, externalstatuschecks.DeleteProjectExternalStatusCheck),
		"update_project":         wrapAction(client, externalstatuschecks.UpdateProjectExternalStatusCheck),
		"retry_project":          wrapVoidAction(client, externalstatuschecks.RetryFailedExternalStatusCheckForProjectMR),
		"set_project_mr_status":  wrapVoidAction(client, externalstatuschecks.SetProjectMRExternalStatusCheckStatus),
	}
	addMetaTool(server, "gitlab_external_status_check", `Manage GitLab external status checks for merge requests and projects.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions (legacy):
- list_mr_checks: List status checks for an MR. Params: project_id, mr_iid (required), page, per_page
- set_status: Set status of an external status check for an MR. Params: project_id, mr_iid, sha, external_status_check_id, status (required)
- list_project_checks: List external status checks for a project. Params: project_id (required), page, per_page
- create: Create an external status check. Params: project_id, name, external_url (required), protected_branch_ids
- delete: Delete an external status check. Params: project_id, check_id (required)
- update: Update an external status check. Params: project_id, check_id (required), name, external_url, protected_branch_ids
- retry: Retry a failed status check for an MR. Params: project_id, mr_iid, check_id (required)

Actions (project-scoped, preferred):
- list_project_mr_checks: List status checks for a project MR. Params: project_id, mr_iid (required), page, per_page
- list_project: List external status checks for a project. Params: project_id (required), page, per_page
- create_project: Create an external status check (returns created object). Params: project_id, name, external_url (required), shared_secret, protected_branch_ids
- delete_project: Delete an external status check. Params: project_id, check_id (required)
- update_project: Update an external status check (returns updated object). Params: project_id, check_id (required), name, external_url, shared_secret, protected_branch_ids
- retry_project: Retry a failed status check for a project MR. Params: project_id, mr_iid, check_id (required)
- set_project_mr_status: Set status of an external status check. Params: project_id, mr_iid, sha, external_status_check_id, status (required)`, routes, metaAnnotations, toolutil.IconSecurity)
}

// registerGroupSCIMMeta registers the gitlab_group_scim meta-tool with actions
// for managing SCIM identities in a group.
func registerGroupSCIMMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":   wrapAction(client, groupscim.List),
		"get":    wrapAction(client, groupscim.Get),
		"update": wrapVoidAction(client, groupscim.Update),
		"delete": wrapVoidAction(client, groupscim.Delete),
	}
	addMetaTool(server, "gitlab_group_scim", `Manage SCIM identities for a GitLab group.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List SCIM identities for a group. Params: group_id (required)
- get: Get a single SCIM identity. Params: group_id (required), uid (required)
- update: Update a SCIM identity's external UID. Params: group_id (required), uid (required), extern_uid (required)
- delete: Delete a SCIM identity. Params: group_id (required), uid (required)`, routes, metaAnnotations, toolutil.IconSecurity)
}

// registerMemberRoleMeta registers the gitlab_member_role meta-tool with actions
// for managing custom member roles at instance and group levels.
func registerMemberRoleMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list_instance":   wrapAction(client, memberroles.ListInstance),
		"create_instance": wrapAction(client, memberroles.CreateInstance),
		"delete_instance": wrapVoidAction(client, memberroles.DeleteInstance),
		"list_group":      wrapAction(client, memberroles.ListGroup),
		"create_group":    wrapAction(client, memberroles.CreateGroup),
		"delete_group":    wrapVoidAction(client, memberroles.DeleteGroup),
	}
	addMetaTool(server, "gitlab_member_role", `Manage custom member roles in GitLab at instance or group level.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list_instance: List all instance-level member roles. No params required.
- create_instance: Create an instance-level member role. Params: name (required), base_access_level (required, 10/20/30/40/50), description, permissions (object with bool fields)
- delete_instance: Delete an instance-level member role. Params: member_role_id (required)
- list_group: List member roles for a group. Params: group_id (required)
- create_group: Create a group-level member role. Params: group_id (required), name (required), base_access_level (required), description, permissions
- delete_group: Delete a group-level member role. Params: group_id (required), member_role_id (required)`, routes, metaAnnotations, toolutil.IconSecurity)
}

// registerEnterpriseUserMeta registers the gitlab_enterprise_user meta-tool with actions
// for listing, getting, disabling 2FA, and deleting enterprise users.
func registerEnterpriseUserMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":        wrapAction(client, enterpriseusers.List),
		"get":         wrapAction(client, enterpriseusers.Get),
		"disable_2fa": wrapVoidAction(client, enterpriseusers.Disable2FA),
		"delete":      wrapVoidAction(client, enterpriseusers.Delete),
	}
	addMetaTool(server, "gitlab_enterprise_user", `Manage enterprise users for a GitLab group.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List enterprise users. Params: group_id (required), username, search, active, blocked, created_after, created_before, two_factor, page, per_page
- get: Get details of a specific enterprise user. Params: group_id (required), user_id (required)
- disable_2fa: Disable two-factor authentication for an enterprise user. Params: group_id (required), user_id (required)
- delete: Delete an enterprise user. Params: group_id (required), user_id (required), hard_delete (optional)`, routes, metaAnnotations, toolutil.IconUser)
}

// registerAttestationMeta registers the gitlab_attestation meta-tool with actions
// for listing and downloading build attestations.
func registerAttestationMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":     wrapAction(client, attestations.List),
		"download": wrapAction(client, attestations.Download),
	}
	addMetaTool(server, "gitlab_attestation", `Manage build attestations for a GitLab project.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List attestations for a project matching a subject digest. Params: project_id (required), subject_digest (required)
- download: Download a specific attestation. Params: project_id (required), attestation_iid (required)`, routes, readOnlyMetaAnnotations, toolutil.IconSecurity)
}

// registerCompliancePolicyMeta registers the gitlab_compliance_policy meta-tool with actions:
// get, update.
func registerCompliancePolicyMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"get":    wrapAction(client, compliancepolicy.Get),
		"update": wrapAction(client, compliancepolicy.Update),
	}
	addMetaTool(server, "gitlab_compliance_policy", `Manage admin compliance policy settings (CSP namespace).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- get: Get current compliance policy settings (admin). No params required.
- update: Update compliance policy settings (admin). Params: csp_namespace_id (optional, int64)`, routes, metaAnnotations, toolutil.IconSecurity)
}

// registerProjectAliasMeta registers the gitlab_project_alias meta-tool with actions:
// list, get, create, delete.
func registerProjectAliasMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":   wrapAction(client, projectaliases.List),
		"get":    wrapAction(client, projectaliases.Get),
		"create": wrapAction(client, projectaliases.Create),
		"delete": wrapVoidAction(client, projectaliases.Delete),
	}
	addMetaTool(server, "gitlab_project_alias", `Manage GitLab project aliases (admin, Premium/Ultimate).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List all project aliases. No params required.
- get: Get a project alias by name. Params: name (required)
- create: Create a project alias. Params: name (required), project_id (required, int64)
- delete: Delete a project alias. Params: name (required)`, routes, metaAnnotations, toolutil.IconProject)
}

// registerGeoMeta registers the gitlab_geo enterprise meta-tool that provides
// Geo replication site management (create, list, get, edit, delete, repair, status).
func registerGeoMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"create":      wrapAction(client, geo.Create),
		"list":        wrapAction(client, geo.List),
		"get":         wrapAction(client, geo.Get),
		"edit":        wrapAction(client, geo.Edit),
		"delete":      wrapVoidAction(client, geo.Delete),
		"repair":      wrapAction(client, geo.Repair),
		"list_status": wrapAction(client, geo.ListStatus),
		"get_status":  wrapAction(client, geo.GetStatus),
	}
	addMetaTool(server, "gitlab_geo", `Manage GitLab Geo replication sites (admin, Premium/Ultimate).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- create: Create a Geo site. Params: name, url, primary, enabled, internal_url, files_max_capacity, repos_max_capacity, verification_max_capacity, container_repositories_max_capacity, sync_object_storage, selective_sync_type, selective_sync_shards, selective_sync_namespace_ids, minimum_reverification_interval
- list: List all Geo sites. Pagination: page, per_page
- get: Get a Geo site. Params: id (required)
- edit: Edit a Geo site. Params: id (required), plus any fields from create (except primary, sync_object_storage)
- delete: Delete a Geo site. Params: id (required)
- repair: Repair OAuth for a Geo site. Params: id (required)
- list_status: List replication status of all Geo sites. Pagination: page, per_page
- get_status: Get replication status of a Geo site. Params: id (required)`, routes, metaAnnotations, toolutil.IconServer)
}

// registerModelRegistryMeta registers the gitlab_model_registry enterprise meta-tool
// that provides ML model registry operations (download model package files).
func registerModelRegistryMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"download": wrapAction(client, modelregistry.Download),
	}
	addMetaTool(server, "gitlab_model_registry", `Manage GitLab ML model registry (Premium/Ultimate).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- download: Download a ML model package file. Params: project_id (required), model_version_id (required), path (required), filename (required). Returns base64-encoded content.`, routes, readOnlyMetaAnnotations, toolutil.IconPackage)
}

// registerStorageMoveMeta registers the gitlab_storage_move enterprise meta-tool
// that provides repository storage move operations for projects, groups, and snippets.
func registerStorageMoveMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"retrieve_all_project":    wrapAction(client, projectstoragemoves.RetrieveAll),
		"retrieve_project":        wrapAction(client, projectstoragemoves.RetrieveForProject),
		"get_project":             wrapAction(client, projectstoragemoves.Get),
		"get_project_for_project": wrapAction(client, projectstoragemoves.GetForProject),
		"schedule_project":        wrapAction(client, projectstoragemoves.Schedule),
		"schedule_all_project":    wrapAction(client, projectstoragemoves.ScheduleAll),
		"retrieve_all_group":      wrapAction(client, groupstoragemoves.RetrieveAll),
		"retrieve_group":          wrapAction(client, groupstoragemoves.RetrieveForGroup),
		"get_group":               wrapAction(client, groupstoragemoves.Get),
		"get_group_for_group":     wrapAction(client, groupstoragemoves.GetForGroup),
		"schedule_group":          wrapAction(client, groupstoragemoves.Schedule),
		"schedule_all_group":      wrapAction(client, groupstoragemoves.ScheduleAll),
		"retrieve_all_snippet":    wrapAction(client, snippetstoragemoves.RetrieveAll),
		"retrieve_snippet":        wrapAction(client, snippetstoragemoves.RetrieveForSnippet),
		"get_snippet":             wrapAction(client, snippetstoragemoves.Get),
		"get_snippet_for_snippet": wrapAction(client, snippetstoragemoves.GetForSnippet),
		"schedule_snippet":        wrapAction(client, snippetstoragemoves.Schedule),
		"schedule_all_snippet":    wrapAction(client, snippetstoragemoves.ScheduleAll),
	}
	addMetaTool(server, "gitlab_storage_move", `Manage GitLab repository storage moves across projects, groups, and snippets (admin only).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions (Project):
- retrieve_all_project: List all project storage moves. Pagination: page, per_page
- retrieve_project: List storage moves for a project. Params: project_id (required). Pagination: page, per_page
- get_project: Get a project storage move by ID. Params: id (required)
- get_project_for_project: Get a storage move for a specific project. Params: project_id (required), id (required)
- schedule_project: Schedule a storage move for a project. Params: project_id (required), destination_storage_name (optional)
- schedule_all_project: Schedule storage moves for all projects. Params: source_storage_name (optional), destination_storage_name (optional)

Actions (Group):
- retrieve_all_group: List all group storage moves. Pagination: page, per_page
- retrieve_group: List storage moves for a group. Params: group_id (required). Pagination: page, per_page
- get_group: Get a group storage move by ID. Params: id (required)
- get_group_for_group: Get a storage move for a specific group. Params: group_id (required), id (required)
- schedule_group: Schedule a storage move for a group. Params: group_id (required), destination_storage_name (optional)
- schedule_all_group: Schedule storage moves for all groups. Params: source_storage_name (optional), destination_storage_name (optional)

Actions (Snippet):
- retrieve_all_snippet: List all snippet storage moves. Pagination: page, per_page
- retrieve_snippet: List storage moves for a snippet. Params: snippet_id (required). Pagination: page, per_page
- get_snippet: Get a snippet storage move by ID. Params: id (required)
- get_snippet_for_snippet: Get a storage move for a specific snippet. Params: snippet_id (required), id (required)
- schedule_snippet: Schedule a storage move for a snippet. Params: snippet_id (required), destination_storage_name (optional)
- schedule_all_snippet: Schedule storage moves for all snippets. Params: source_storage_name (optional), destination_storage_name (optional)`, routes, metaAnnotations, toolutil.IconServer)
}

// registerVulnerabilityMeta registers the gitlab_vulnerability meta-tool with actions:
// list, get, dismiss, confirm, resolve, revert, severity_count, pipeline_security_summary.
func registerVulnerabilityMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":                      wrapAction(client, vulnerabilities.List),
		"get":                       wrapAction(client, vulnerabilities.Get),
		"dismiss":                   wrapAction(client, vulnerabilities.Dismiss),
		"confirm":                   wrapAction(client, vulnerabilities.Confirm),
		"resolve":                   wrapAction(client, vulnerabilities.Resolve),
		"revert":                    wrapAction(client, vulnerabilities.Revert),
		"severity_count":            wrapAction(client, vulnerabilities.SeverityCount),
		"pipeline_security_summary": wrapAction(client, vulnerabilities.PipelineSecuritySummary),
	}
	addMetaTool(server, "gitlab_vulnerability", `Manage project vulnerabilities via GraphQL API (Premium/Ultimate).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List project vulnerabilities. Params: project_path (required), severity (optional, array), state (optional, array), scanner (optional, array), report_type (optional, array), has_issues (optional, bool), has_resolution (optional, bool), sort (optional). Pagination: first, after
- get: Get a single vulnerability by GID. Params: id (required, GID string e.g. gid://gitlab/Vulnerability/42)
- dismiss: Dismiss a vulnerability. Params: id (required, GID), comment (optional), dismissal_reason (optional: ACCEPTABLE_RISK, FALSE_POSITIVE, MITIGATING_CONTROL, USED_IN_TESTS, NOT_APPLICABLE)
- confirm: Confirm a detected vulnerability. Params: id (required, GID)
- resolve: Resolve a vulnerability. Params: id (required, GID)
- revert: Revert a vulnerability to detected state. Params: id (required, GID)
- severity_count: Get vulnerability severity counts for a project. Params: project_path (required)
- pipeline_security_summary: Get security report summary for a pipeline. Params: project_path (required), pipeline_iid (required)`, routes, metaAnnotations, toolutil.IconSecurity)
}

// registerSecurityFindingsMeta registers the gitlab_security_finding meta-tool with actions: list.
func registerSecurityFindingsMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list": wrapAction(client, securityfindings.List),
	}
	addMetaTool(server, "gitlab_security_finding", `List pipeline security report findings via GraphQL API (Premium/Ultimate). Replaces deprecated REST vulnerability_findings endpoint.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List security report findings for a pipeline. Params: project_path (required), pipeline_iid (required), severity (optional, array), confidence (optional, array), scanner (optional, array), report_type (optional, array). Pagination: first, after`, routes, readOnlyMetaAnnotations, toolutil.IconSecurity)
}

// registerCICatalogMeta registers the gitlab_ci_catalog meta-tool with actions: list, get.
func registerCICatalogMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list": wrapAction(client, cicatalog.List),
		"get":  wrapAction(client, cicatalog.Get),
	}
	addMetaTool(server, "gitlab_ci_catalog", `Discover and inspect CI/CD Catalog resources via GraphQL API (Premium/Ultimate).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List CI/CD Catalog resources. Params: search (optional), scope (optional: ALL, NAMESPACED), sort (optional: NAME_ASC, NAME_DESC, LATEST_RELEASED_AT_ASC, LATEST_RELEASED_AT_DESC, STAR_COUNT_ASC, STAR_COUNT_DESC). Pagination: first, after
- get: Get a CI/CD Catalog resource by GID or project full path. Params: id (optional, GID e.g. gid://gitlab/Ci::CatalogResource/1), full_path (optional, e.g. my-group/my-components). One of id or full_path is required.`, routes, readOnlyMetaAnnotations, toolutil.IconPackage)
}

// registerBranchRulesMeta registers the gitlab_branch_rule meta-tool with actions: list.
func registerBranchRulesMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list": wrapAction(client, branchrules.List),
	}
	addMetaTool(server, "gitlab_branch_rule", `Query branch rules for a project via GraphQL API (Premium/Ultimate). Provides an aggregated read-only view of branch protections, approval rules, and external status checks.
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List branch rules for a project. Params: project_path (required, e.g. my-group/my-project). Pagination: first, after`, routes, readOnlyMetaAnnotations, toolutil.IconBranch)
}

// registerCustomEmojiMeta registers the gitlab_custom_emoji meta-tool with actions: list, create, delete.
func registerCustomEmojiMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]actionFunc{
		"list":   wrapAction(client, customemoji.List),
		"create": wrapAction(client, customemoji.Create),
		"delete": wrapVoidAction(client, customemoji.Delete),
	}
	addMetaTool(server, "gitlab_custom_emoji", `Manage group-level custom emoji via GraphQL API (Premium/Ultimate). Custom emoji are group-level assets with custom images, distinct from award emoji (reactions).
Use "action" to specify the operation. Valid actions: `+validActionsString(routes)+`

Actions:
- list: List custom emoji for a group. Params: group_path (required). Pagination: first, after
- create: Create a custom emoji. Params: group_path (required), name (required, without colons), url (required, image URL)
- delete: Delete a custom emoji. Params: id (required, GID e.g. gid://gitlab/CustomEmoji/1)`, routes, metaAnnotations, toolutil.IconLabel)
}

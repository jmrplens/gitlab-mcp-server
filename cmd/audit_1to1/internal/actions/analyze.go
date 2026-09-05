package actions

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

const (
	coveredRawGeneric     = "COVERED_RAW. Generic slug-dispatched integration action (gitlab_set_integration / gitlab_*_group_integration)"
	coveredGenericSearch  = "COVERED_GENERIC. Method-value scoped search"
	coveredGraphQLEpicDsc = "COVERED_GRAPHQL. Epicdiscussions pkg"
	coveredGraphQLEpicNts = "COVERED_GRAPHQL. Epicnotes pkg"
	skipBinaryBytes       = "INTENTIONAL_SKIP_BINARY. File bytes"
	coveredGraphQLEpics   = "COVERED_GRAPHQL. Epics pkg"
	coveredGenericMethod  = "COVERED_GENERIC. Method-value"
	coveredGenericDeprPrj = "COVERED_GENERIC. Deprecated; project variant"
)

// serviceCoverage reports the SDK method coverage of one client-go service
// interface across the packages that reference it.
type serviceCoverage struct {
	Service        string   `json:"service"`
	Packages       []string `json:"packages"`
	APIMethods     int      `json:"api_methods"`
	CoveredMethods int      `json:"covered_methods"`
	MissingMethods []string `json:"missing_methods,omitempty"`
}

type report struct {
	SchemaVersion    int               `json:"schema_version"`
	ClientGoPath     string            `json:"client_go_path"`
	Summary          reportSummary     `json:"summary"`
	Services         []serviceCoverage `json:"services"`
	StaleAcceptances []string          `json:"stale_acceptances,omitempty"`
}

type reportSummary struct {
	Services         int `json:"services"`
	ServicesWithGaps int `json:"services_with_gaps"`
	APIMethods       int `json:"api_methods"`
	CoveredMethods   int `json:"covered_methods"`
	MissingMethods   int `json:"missing_methods"`
}

// marshalIndent is the JSON encoder, a variable so a test can reach the
// encoding failure branch that a report of strings and ints never produces.
var marshalIndent = json.MarshalIndent

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline). gapsOnly filters to entries with at
// least one finding, matching the original -gaps-only flag.
func Run(root string, gapsOnly bool) ([]byte, error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, err
	}
	content, err := marshalIndent(rep, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), nil
}

func buildReport(root string, gapsOnly bool) (report, error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return report{}, err
	}
	usage := shared.CollectServiceUsage(pkgs)
	services := make([]serviceCoverage, 0, len(usage))
	for _, use := range usage {
		cov := coverageForService(use)
		if gapsOnly && len(cov.MissingMethods) == 0 {
			continue
		}
		services = append(services, cov)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Service < services[j].Service })

	var staleAcceptances []string
	for _, use := range usage {
		service := use.Service()
		for method := range use.Called {
			key := service + "." + method
			if _, ok := acceptedMissingMethods[key]; ok {
				staleAcceptances = append(staleAcceptances, key)
			}
		}
	}
	sort.Strings(staleAcceptances)

	return report{
		SchemaVersion:    shared.SchemaVersion,
		ClientGoPath:     shared.ClientGoPkgPath,
		Summary:          summarize(services),
		Services:         services,
		StaleAcceptances: staleAcceptances,
	}, nil
}

// acceptedMissingMethods adjudicates SDK service methods that the call-expression
// scanner reports as uncovered but which are legitimately handled another way (so
// they are NOT genuine R-ACTION gaps). The scanner only sees direct
// client.GL().Svc.Method(...) calls, so it misses: methods passed as VALUES into a
// generic helper, RAW REST handlers (client.GL().NewRequest+Do with a path string),
// GraphQL handlers (ADR-0006), and capabilities covered by a superseding/generic
// variant. It also can't know which endpoints are INTENTIONALLY not exposed (binary
// file transfer, CI-job-token self-lookup). Each entry carries a category+rationale.
// Key = "<Service>.<Method>" (service name without the ServiceInterface suffix).
// Methods NOT listed here and still uncovered are the genuine new-tool backlog.
var acceptedMissingMethods = map[string]string{
	// COVERED_VARIANT — an alternative SDK binding for an endpoint another
	// covered method already drives; one action per endpoint is the rule.
	"RepositoryFiles.GetRawFile": "COVERED_VARIANT. Same raw-file endpoint; gitlab_file_raw calls the streaming GetRawFileReader (client-go v2.58.0) so UPLOAD_MAX_FILE_SIZE is enforced without buffering the blob",
	// COVERED_VARIANT — the wrapper's return type ([]*BoardList) cannot decode
	// the single-object response GitLab actually sends, so the handler issues
	// the request directly. client-go!2996 is open against release-client-3.0
	// and has not merged, so no v2 release carries the fix. Retire this entry
	// and the raw request once the client-go version this project depends on
	// actually contains !2996 — the v3 bump is when to check, not the condition
	// itself, since an open MR may land in a later minor or not at all.
	// Tracked in docs/development/upstream-bugs.md.
	"GroupIssueBoards.UpdateIssueBoardList": "COVERED_VARIANT. group_board_list_update calls the same PUT endpoint via a raw request because the v2 wrapper declares []*BoardList and can never decode a successful response (client-go!2996, open for v3)",

	// COVERED_GENERIC — method-value passed to a generic helper (not a call.Fun).
	"AwardEmoji.ListIssuesAwardEmojiOnNote":         "COVERED_GENERIC. Method-value -> listNoteAwardEmoji (gitlab_issue_note_emoji_list)",
	"AwardEmoji.CreateIssuesAwardEmojiOnNote":       "COVERED_GENERIC. Method-value -> createNoteAwardEmoji",
	"AwardEmoji.DeleteIssuesAwardEmojiOnNote":       "COVERED_GENERIC. Method-value -> deleteNoteAwardEmoji",
	"AwardEmoji.ListMergeRequestAwardEmojiOnNote":   "COVERED_GENERIC. Method-value (gitlab_mr_note_emoji_list)",
	"AwardEmoji.CreateMergeRequestAwardEmojiOnNote": coveredGenericMethod,
	"AwardEmoji.DeleteMergeRequestAwardEmojiOnNote": coveredGenericMethod,
	"AwardEmoji.ListSnippetAwardEmojiOnNote":        "COVERED_GENERIC. Method-value (gitlab_snippet_note_emoji_list)",
	"AwardEmoji.CreateSnippetAwardEmojiOnNote":      coveredGenericMethod,
	"AwardEmoji.DeleteSnippetAwardEmojiOnNote":      coveredGenericMethod,
	"Search.Commits":                "COVERED_GENERIC. Method-value in runScopedSearch (gitlab_search_commits)",
	"Search.CommitsByGroup":         coveredGenericSearch,
	"Search.CommitsByProject":       coveredGenericSearch,
	"Search.Issues":                 coveredGenericSearch,
	"Search.IssuesByGroup":          coveredGenericSearch,
	"Search.IssuesByProject":        coveredGenericSearch,
	"Search.MergeRequests":          coveredGenericSearch,
	"Search.MergeRequestsByGroup":   coveredGenericSearch,
	"Search.MergeRequestsByProject": coveredGenericSearch,
	"Search.Milestones":             coveredGenericSearch,
	"Search.MilestonesByGroup":      coveredGenericSearch,
	"Search.MilestonesByProject":    coveredGenericSearch,
	"Runners.ListRunners":           "COVERED_GENERIC. Method-value (gitlab_runner_list)",
	"Runners.ListAllRunners":        "COVERED_GENERIC. Method-value (gitlab_runner_list_all)",

	// COVERED_GRAPHQL — ADR-0006 GraphQL handlers (no SDK service call).
	"Epics.CreateEpic":                     coveredGraphQLEpics,
	"Epics.DeleteEpic":                     coveredGraphQLEpics,
	"Epics.GetEpic":                        coveredGraphQLEpics,
	"Epics.UpdateEpic":                     coveredGraphQLEpics,
	"Notes.CreateEpicNote":                 coveredGraphQLEpicNts,
	"Notes.DeleteEpicNote":                 coveredGraphQLEpicNts,
	"Notes.GetEpicNote":                    coveredGraphQLEpicNts,
	"Notes.ListEpicNotes":                  coveredGraphQLEpicNts,
	"Notes.UpdateEpicNote":                 coveredGraphQLEpicNts,
	"Discussions.AddEpicDiscussionNote":    coveredGraphQLEpicDsc,
	"Discussions.CreateEpicDiscussion":     coveredGraphQLEpicDsc,
	"Discussions.DeleteEpicDiscussionNote": coveredGraphQLEpicDsc,
	"Discussions.GetEpicDiscussion":        coveredGraphQLEpicDsc,
	"Discussions.ListGroupEpicDiscussions": coveredGraphQLEpicDsc,
	"Discussions.UpdateEpicDiscussionNote": coveredGraphQLEpicDsc,

	// COVERED_RAW — raw REST (NewRequest/Do or URL-built); SDK method not called.
	"Commits.GetGPGSignature":                           "COVERED_RAW. gitlab_commit_signature",
	"Deployments.GetProjectDeployment":                  "COVERED_RAW. gitlab_deployment_get",
	"Deployments.ListProjectDeployments":                "COVERED_RAW. gitlab_deployment_list",
	"Jobs.ListPipelineJobs":                             "COVERED_RAW. gitlab_job_list",
	"Jobs.ListProjectJobs":                              "COVERED_RAW. gitlab_job_list_project",
	"MergeRequestApprovals.ChangeApprovalConfiguration": "COVERED_RAW. gitlab_mr_approval_config",
	"MergeRequestApprovals.CreateApprovalRule":          "COVERED_RAW. gitlab_mr_approval_rule_create",
	"MergeRequestApprovals.GetApprovalRules":            "COVERED_RAW. gitlab_mr_approval_rules",
	"MergeRequestApprovals.GetApprovalState":            "COVERED_RAW. gitlab_mr_approval_state",
	"MergeRequestApprovals.UpdateApprovalRule":          "COVERED_RAW. gitlab_mr_approval_rule_update",
	"PipelineSchedules.GetPipelineSchedule":             "COVERED_RAW. gitlab_pipeline_schedule_get",
	"IssueBoards.GetIssueBoard":                         "COVERED_RAW. gitlab_board_get",
	"IssueBoards.GetIssueBoardLists":                    "COVERED_RAW. gitlab_board_list_lists",
	"GroupIssueBoards.CreateGroupIssueBoard":            "COVERED_RAW. Groupboards raw",
	"GroupIssueBoards.GetGroupIssueBoard":               "COVERED_RAW. gitlab_group_board_get",
	"GroupIssueBoards.ListGroupIssueBoards":             "COVERED_RAW. gitlab_group_board_list",
	"GroupIssueBoards.UpdateIssueBoard":                 "COVERED_RAW. gitlab_group_board_update",
	"Projects.AddProjectHook":                           "COVERED_RAW. gitlab_project_hook_add",
	"Projects.EditProjectHook":                          "COVERED_RAW. gitlab_project_hook_edit",
	"Projects.GetProjectHook":                           "COVERED_RAW. gitlab_project_hook_get",
	"Projects.ListProjectHooks":                         "COVERED_RAW. gitlab_project_hook_list",
	"Projects.ListUserContributedProjects":              "COVERED_RAW. gitlab_project_list_user_contributed",
	"Projects.ListUserProjects":                         "COVERED_RAW. gitlab_project_list_user_projects",
	"Projects.ListUserStarredProjects":                  "COVERED_RAW. gitlab_project_list_user_starred",
	"ProjectImportExport.ImportFromFile":                "COVERED_RAW. gitlab_import_project_from_file",
	"ProjectImportExport.ImportStatus":                  "COVERED_RAW. gitlab_get_project_import_status",
	"Features.SetFeatureFlag":                           "COVERED_RAW. gitlab_set_feature_flag (raw POST)",
	"Repositories.Archive":                              "COVERED_RAW. gitlab_repository_archive (URL-built)",
	"GenericPackages.DownloadPackageFile":               "COVERED_RAW. gitlab_package_download (streamed)",

	// COVERED_GENERIC — superseding/generic variant covers the same capability.
	"MergeRequests.GetMergeRequestApprovals":                      "COVERED_GENERIC. Mrapprovals config/state",
	"MergeRequests.GetMergeRequestChanges":                        "COVERED_GENERIC. gitlab_mr_changes_get (ListMergeRequestDiffs)",
	"ProjectMembers.ListProjectMembers":                           "COVERED_GENERIC. gitlab_project_members_list (ListAllProjectMembers)",
	"Groups.ListGroupMembers":                                     "COVERED_GENERIC. gitlab_group_members_list (ListAllGroupMembers)",
	"Groups.ListSubGroups":                                        "COVERED_GENERIC. gitlab_subgroups_list (ListDescendantGroups)",
	"Groups.DeleteGroupLDAPLink":                                  "COVERED_GENERIC. Groupldap ForProvider/WithCNOrFilter variants",
	"PersonalAccessTokens.RotatePersonalAccessTokenByID":          "COVERED_GENERIC. gitlab_personal_access_token_rotate",
	"ExternalStatusChecks.CreateExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.DeleteExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.ListMergeStatusChecks":                  "COVERED_GENERIC. Deprecated; ListProjectMergeRequestExternalStatusChecks",
	"ExternalStatusChecks.RetryFailedStatusCheckForAMergeRequest": coveredGenericDeprPrj,
	"ExternalStatusChecks.SetExternalStatusCheckStatus":           "COVERED_GENERIC. Deprecated; SetProjectMergeRequestExternalStatusCheckStatus",
	"ExternalStatusChecks.UpdateExternalStatusCheck":              coveredGenericDeprPrj,
	"ErrorTracking.EnableDisableErrorTracking":                    "COVERED_GENERIC. Deprecated; gitlab_enable_disable_error_tracking",
	"ErrorTracking.CreateErrorTrackingSettings":                   "COVERED_GENERIC. Same settings resource as UpdateErrorTrackingSettings",

	// INTENTIONAL_SKIP_BINARY — raw file/archive/state bytes, unsuitable for JSON tools.
	"GroupMarkdownUploads.DownloadGroupMarkdownUploadByID":                    skipBinaryBytes,
	"GroupMarkdownUploads.DownloadGroupMarkdownUploadBySecretAndFilename":     skipBinaryBytes,
	"ProjectMarkdownUploads.DownloadProjectMarkdownUploadByID":                skipBinaryBytes,
	"ProjectMarkdownUploads.DownloadProjectMarkdownUploadBySecretAndFilename": skipBinaryBytes,
	"GroupRelationsExport.ExportDownload":                                     "INTENTIONAL_SKIP_BINARY. Export archive bytes",
	"SecureFiles.DownloadSecureFile":                                          "INTENTIONAL_SKIP_BINARY. Secure file bytes",
	"TerraformStates.Download":                                                "INTENTIONAL_SKIP_BINARY. Tfstate bytes",
	"TerraformStates.DownloadLatest":                                          "INTENTIONAL_SKIP_BINARY. Tfstate bytes",
	"Groups.DownloadAvatar":                                                   "INTENTIONAL_SKIP_BINARY. Group avatar image bytes (*bytes.Reader); upload IS exposed",

	// INTENTIONAL_SKIP_OTHER
	"Jobs.GetJobTokensJob":       "INTENTIONAL_SKIP_OTHER. CI-job-token self-lookup; not usable with a PAT",
	"Repositories.StreamArchive": "INTENTIONAL_SKIP_OTHER. Streaming dup of Repositories.Archive (gitlab_repository_archive)",

	// COVERED_RAW — the generic slug-dispatched integration actions (project
	// gitlab_set_integration + group list/get/set/delete) cover the full integration
	// slug surface at project and group scope via raw REST PUT/GET/DELETE.
	"Integrations.DeleteGroupMattermostIntegration":              coveredRawGeneric,
	"Integrations.DeleteGroupMattermostSlashCommandsIntegration": coveredRawGeneric,
	"Integrations.DisableGroupHarbor":                            coveredRawGeneric,
	"Integrations.DisableGroupJira":                              coveredRawGeneric,
	"Integrations.DisableGroupMicrosoftTeamsNotifications":       coveredRawGeneric,
	"Integrations.DisableGroupSlack":                             coveredRawGeneric,
	"Integrations.DisableGroupWebexTeams":                        coveredRawGeneric,
	"Integrations.DisableProjectGoogleChat":                      coveredRawGeneric,
	"Integrations.GetGroupDiscordSettings":                       coveredRawGeneric,
	"Integrations.GetGroupGoogleChatSettings":                    coveredRawGeneric,
	"Integrations.GetGroupHarborSettings":                        coveredRawGeneric,
	"Integrations.GetGroupJiraSettings":                          coveredRawGeneric,
	"Integrations.GetGroupMatrixSettings":                        coveredRawGeneric,
	"Integrations.GetGroupMattermostIntegration":                 coveredRawGeneric,
	"Integrations.GetGroupMattermostSettings":                    coveredRawGeneric,
	"Integrations.GetGroupMattermostSlashCommandsIntegration":    coveredRawGeneric,
	"Integrations.GetGroupMicrosoftTeamsNotifications":           coveredRawGeneric,
	"Integrations.GetGroupSlackSettings":                         coveredRawGeneric,
	"Integrations.GetGroupTelegramSettings":                      coveredRawGeneric,
	"Integrations.GetGroupWebexTeamsSettings":                    coveredRawGeneric,
	"Integrations.GetProjectGoogleChatSettings":                  coveredRawGeneric,
	"Integrations.ListActiveGroupIntegrations":                   coveredRawGeneric,
	"Integrations.SetGroupMattermostIntegration":                 coveredRawGeneric,
	"Integrations.SetGroupMattermostSlashCommandsIntegration":    coveredRawGeneric,
	"Integrations.SetGroupMicrosoftTeamsNotifications":           coveredRawGeneric,
	"Integrations.SetGroupSlackSettings":                         coveredRawGeneric,
	"Integrations.SetGroupWebexTeamsSettings":                    coveredRawGeneric,
	"Integrations.SetProjectGoogleChatSettings":                  coveredRawGeneric,
	"Integrations.SetUpGroupHarbor":                              coveredRawGeneric,
	"Integrations.SetUpGroupJira":                                coveredRawGeneric,
	"Services.GetSlackApplication":                               coveredRawGeneric,
	"Services.SetCustomIssueTrackerService":                      coveredRawGeneric,
	"Services.SetDataDogService":                                 coveredRawGeneric,
	"Services.SetDiscordService":                                 coveredRawGeneric,
	"Services.SetDroneCIService":                                 coveredRawGeneric,
	"Services.SetEmailsOnPushService":                            coveredRawGeneric,
	"Services.SetExternalWikiService":                            coveredRawGeneric,
	"Services.SetGithubService":                                  coveredRawGeneric,
	"Services.SetHarborService":                                  coveredRawGeneric,
	"Services.SetJenkinsCIService":                               coveredRawGeneric,
	"Services.SetMatrixService":                                  coveredRawGeneric,
	"Services.SetMattermostService":                              coveredRawGeneric,
	"Services.SetMattermostSlashCommandsService":                 coveredRawGeneric,
	"Services.SetMicrosoftTeamsService":                          coveredRawGeneric,
	"Services.SetPipelinesEmailService":                          coveredRawGeneric,
	"Services.SetRedmineService":                                 coveredRawGeneric,
	"Services.SetSlackApplication":                               coveredRawGeneric,
	"Services.SetSlackService":                                   coveredRawGeneric,
	"Services.SetSlackSlashCommandsService":                      coveredRawGeneric,
	"Services.SetTelegramService":                                coveredRawGeneric,
	"Services.SetYouTrackService":                                coveredRawGeneric,
}

// isAcceptedMissingMethod reports whether an uncovered SDK method is adjudicated as
// covered-another-way or intentionally not exposed (so not a genuine R-ACTION gap).
func isAcceptedMissingMethod(service, method string) bool {
	_, ok := acceptedMissingMethods[service+"."+method]
	return ok
}

func coverageForService(use *shared.ServiceUsage) serviceCoverage {
	apiMethods := shared.APIMethodNames(use.Named)
	service := use.Service()
	covered := 0
	var missing []string
	for _, method := range apiMethods {
		if _, ok := use.Called[method]; ok {
			covered++
			continue
		}
		// Adjudicated: covered via raw/GraphQL/generic, or intentionally not exposed.
		if isAcceptedMissingMethod(service, method) {
			covered++
			continue
		}
		missing = append(missing, method)
	}
	sort.Strings(missing)
	return serviceCoverage{
		Service:        use.Named.Obj().Name(),
		Packages:       shared.SortedSet(use.Packages),
		APIMethods:     len(apiMethods),
		CoveredMethods: covered,
		MissingMethods: missing,
	}
}

func summarize(services []serviceCoverage) reportSummary {
	s := reportSummary{Services: len(services)}
	for _, svc := range services {
		if len(svc.MissingMethods) > 0 {
			s.ServicesWithGaps++
		}
		s.APIMethods += svc.APIMethods
		s.CoveredMethods += svc.CoveredMethods
		s.MissingMethods += len(svc.MissingMethods)
	}
	return s
}

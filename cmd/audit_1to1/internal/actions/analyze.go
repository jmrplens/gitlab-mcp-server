// Package actions reports client-go SDK endpoints that no MCP
// action invokes (R-ACTION). For every package under internal/tools it resolves,
// with full Go type information, each call site of the form
// client.GL().{Service}.{Method}(...). The receiver type is a client-go service
// interface; its API methods are those whose signature ends in a variadic
// ...RequestOptionFunc. Methods on a used service that no handler calls are
// reported as candidate missing actions, grouped by the service and the
// internal/tools packages that reference it.
//
// The output is a candidate backlog, not a hard gate: a method may be
// intentionally unexposed, or owned by a sibling package. A human adjudicates
// each entry.
package actions

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
)

const (
	requestOptionMarker = "RequestOptionFunc"

	coveredRawGeneric     = "COVERED_RAW — generic slug-dispatched integration action (gitlab_set_integration / gitlab_*_group_integration)"
	coveredGenericSearch  = "COVERED_GENERIC — method-value scoped search"
	coveredGraphQLEpicDsc = "COVERED_GRAPHQL — epicdiscussions pkg"
	coveredGraphQLEpicNts = "COVERED_GRAPHQL — epicnotes pkg"
	skipBinaryBytes       = "INTENTIONAL_SKIP_BINARY — file bytes"
	coveredGraphQLEpics   = "COVERED_GRAPHQL — epics pkg"
	coveredGenericMethod  = "COVERED_GENERIC — method-value"
	coveredGenericDeprPrj = "COVERED_GENERIC — deprecated; project variant"
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

// serviceUsage accumulates, for one client-go service interface, the set of
// methods called and the internal/tools packages that reference it.
type serviceUsage struct {
	named  *types.Named
	called map[string]struct{}
	pkgs   map[string]struct{}
}

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline). gapsOnly filters to entries with at
// least one finding, matching the original -gaps-only flag.
func Run(root string, gapsOnly bool) ([]byte, error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(rep, "", "  ")
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
	usage := map[string]*serviceUsage{}
	for _, pkg := range pkgs {
		collectServiceUsage(pkg, usage)
	}
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
		service := strings.TrimSuffix(use.named.Obj().Name(), "ServiceInterface")
		for method := range use.called {
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

// collectServiceUsage walks every call expression in pkg and records calls whose
// receiver type is a client-go service interface.
func collectServiceUsage(pkg *packages.Package, usage map[string]*serviceUsage) {
	pkgName := shortPackage(pkg.PkgPath)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			named, ok := clientGoServiceInterface(pkg.TypesInfo.TypeOf(sel.X))
			if !ok {
				return true
			}
			use := usageFor(usage, named)
			use.called[sel.Sel.Name] = struct{}{}
			use.pkgs[pkgName] = struct{}{}
			return true
		})
	}
}

func usageFor(usage map[string]*serviceUsage, named *types.Named) *serviceUsage {
	key := named.Obj().Name()
	use, ok := usage[key]
	if !ok {
		use = &serviceUsage{named: named, called: map[string]struct{}{}, pkgs: map[string]struct{}{}}
		usage[key] = use
	}
	return use
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
	// COVERED_GENERIC — method-value passed to a generic helper (not a call.Fun).
	"AwardEmoji.ListIssuesAwardEmojiOnNote":         "COVERED_GENERIC — method-value -> listNoteAwardEmoji (gitlab_issue_note_emoji_list)",
	"AwardEmoji.CreateIssuesAwardEmojiOnNote":       "COVERED_GENERIC — method-value -> createNoteAwardEmoji",
	"AwardEmoji.DeleteIssuesAwardEmojiOnNote":       "COVERED_GENERIC — method-value -> deleteNoteAwardEmoji",
	"AwardEmoji.ListMergeRequestAwardEmojiOnNote":   "COVERED_GENERIC — method-value (gitlab_mr_note_emoji_list)",
	"AwardEmoji.CreateMergeRequestAwardEmojiOnNote": coveredGenericMethod,
	"AwardEmoji.DeleteMergeRequestAwardEmojiOnNote": coveredGenericMethod,
	"AwardEmoji.ListSnippetAwardEmojiOnNote":        "COVERED_GENERIC — method-value (gitlab_snippet_note_emoji_list)",
	"AwardEmoji.CreateSnippetAwardEmojiOnNote":      coveredGenericMethod,
	"AwardEmoji.DeleteSnippetAwardEmojiOnNote":      coveredGenericMethod,
	"Search.Commits":                "COVERED_GENERIC — method-value in runScopedSearch (gitlab_search_commits)",
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
	"Runners.ListRunners":           "COVERED_GENERIC — method-value (gitlab_runner_list)",
	"Runners.ListAllRunners":        "COVERED_GENERIC — method-value (gitlab_runner_list_all)",

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
	"Commits.GetGPGSignature":                           "COVERED_RAW — gitlab_commit_signature",
	"Deployments.GetProjectDeployment":                  "COVERED_RAW — gitlab_deployment_get",
	"Deployments.ListProjectDeployments":                "COVERED_RAW — gitlab_deployment_list",
	"Jobs.ListPipelineJobs":                             "COVERED_RAW — gitlab_job_list",
	"Jobs.ListProjectJobs":                              "COVERED_RAW — gitlab_job_list_project",
	"MergeRequestApprovals.ChangeApprovalConfiguration": "COVERED_RAW — gitlab_mr_approval_config",
	"MergeRequestApprovals.CreateApprovalRule":          "COVERED_RAW — gitlab_mr_approval_rule_create",
	"MergeRequestApprovals.GetApprovalRules":            "COVERED_RAW — gitlab_mr_approval_rules",
	"MergeRequestApprovals.GetApprovalState":            "COVERED_RAW — gitlab_mr_approval_state",
	"MergeRequestApprovals.UpdateApprovalRule":          "COVERED_RAW — gitlab_mr_approval_rule_update",
	"PipelineSchedules.GetPipelineSchedule":             "COVERED_RAW — gitlab_pipeline_schedule_get",
	"IssueBoards.GetIssueBoard":                         "COVERED_RAW — gitlab_board_get",
	"IssueBoards.GetIssueBoardLists":                    "COVERED_RAW — gitlab_board_list_lists",
	"GroupIssueBoards.CreateGroupIssueBoard":            "COVERED_RAW — groupboards raw",
	"GroupIssueBoards.GetGroupIssueBoard":               "COVERED_RAW — gitlab_group_board_get",
	"GroupIssueBoards.ListGroupIssueBoards":             "COVERED_RAW — gitlab_group_board_list",
	"GroupIssueBoards.UpdateIssueBoard":                 "COVERED_RAW — gitlab_group_board_update",
	"Projects.AddProjectHook":                           "COVERED_RAW — gitlab_project_hook_add",
	"Projects.EditProjectHook":                          "COVERED_RAW — gitlab_project_hook_edit",
	"Projects.GetProjectHook":                           "COVERED_RAW — gitlab_project_hook_get",
	"Projects.ListProjectHooks":                         "COVERED_RAW — gitlab_project_hook_list",
	"Projects.ListUserContributedProjects":              "COVERED_RAW — gitlab_project_list_user_contributed",
	"Projects.ListUserProjects":                         "COVERED_RAW — gitlab_project_list_user_projects",
	"Projects.ListUserStarredProjects":                  "COVERED_RAW — gitlab_project_list_user_starred",
	"ProjectImportExport.ImportFromFile":                "COVERED_RAW — gitlab_import_project_from_file",
	"ProjectImportExport.ImportStatus":                  "COVERED_RAW — gitlab_get_project_import_status",
	"Features.SetFeatureFlag":                           "COVERED_RAW — gitlab_set_feature_flag (raw POST)",
	"Repositories.Archive":                              "COVERED_RAW — gitlab_repository_archive (URL-built)",
	"GenericPackages.DownloadPackageFile":               "COVERED_RAW — gitlab_package_download (streamed)",

	// COVERED_GENERIC — superseding/generic variant covers the same capability.
	"MergeRequests.GetMergeRequestApprovals":                      "COVERED_GENERIC — mrapprovals config/state",
	"MergeRequests.GetMergeRequestChanges":                        "COVERED_GENERIC — gitlab_mr_changes_get (ListMergeRequestDiffs)",
	"ProjectMembers.ListProjectMembers":                           "COVERED_GENERIC — gitlab_project_members_list (ListAllProjectMembers)",
	"Groups.ListGroupMembers":                                     "COVERED_GENERIC — gitlab_group_members_list (ListAllGroupMembers)",
	"Groups.ListSubGroups":                                        "COVERED_GENERIC — gitlab_subgroups_list (ListDescendantGroups)",
	"Groups.DeleteGroupLDAPLink":                                  "COVERED_GENERIC — groupldap ForProvider/WithCNOrFilter variants",
	"PersonalAccessTokens.RotatePersonalAccessTokenByID":          "COVERED_GENERIC — gitlab_personal_access_token_rotate",
	"ExternalStatusChecks.CreateExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.DeleteExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.ListMergeStatusChecks":                  "COVERED_GENERIC — deprecated; ListProjectMergeRequestExternalStatusChecks",
	"ExternalStatusChecks.RetryFailedStatusCheckForAMergeRequest": coveredGenericDeprPrj,
	"ExternalStatusChecks.SetExternalStatusCheckStatus":           "COVERED_GENERIC — deprecated; SetProjectMergeRequestExternalStatusCheckStatus",
	"ExternalStatusChecks.UpdateExternalStatusCheck":              coveredGenericDeprPrj,
	"ErrorTracking.EnableDisableErrorTracking":                    "COVERED_GENERIC — deprecated; gitlab_enable_disable_error_tracking",
	"ErrorTracking.CreateErrorTrackingSettings":                   "COVERED_GENERIC — same settings resource as UpdateErrorTrackingSettings",

	// INTENTIONAL_SKIP_BINARY — raw file/archive/state bytes, unsuitable for JSON tools.
	"GroupMarkdownUploads.DownloadGroupMarkdownUploadByID":                    skipBinaryBytes,
	"GroupMarkdownUploads.DownloadGroupMarkdownUploadBySecretAndFilename":     skipBinaryBytes,
	"ProjectMarkdownUploads.DownloadProjectMarkdownUploadByID":                skipBinaryBytes,
	"ProjectMarkdownUploads.DownloadProjectMarkdownUploadBySecretAndFilename": skipBinaryBytes,
	"GroupRelationsExport.ExportDownload":                                     "INTENTIONAL_SKIP_BINARY — export archive bytes",
	"SecureFiles.DownloadSecureFile":                                          "INTENTIONAL_SKIP_BINARY — secure file bytes",
	"TerraformStates.Download":                                                "INTENTIONAL_SKIP_BINARY — tfstate bytes",
	"TerraformStates.DownloadLatest":                                          "INTENTIONAL_SKIP_BINARY — tfstate bytes",
	"Groups.DownloadAvatar":                                                   "INTENTIONAL_SKIP_BINARY — group avatar image bytes (*bytes.Reader); upload IS exposed",

	// INTENTIONAL_SKIP_OTHER
	"Jobs.GetJobTokensJob":       "INTENTIONAL_SKIP_OTHER — CI-job-token self-lookup; not usable with a PAT",
	"Repositories.StreamArchive": "INTENTIONAL_SKIP_OTHER — streaming dup of Repositories.Archive (gitlab_repository_archive)",

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

func coverageForService(use *serviceUsage) serviceCoverage {
	apiMethods := apiMethodNames(use.named)
	service := strings.TrimSuffix(use.named.Obj().Name(), "ServiceInterface")
	covered := 0
	var missing []string
	for _, method := range apiMethods {
		if _, ok := use.called[method]; ok {
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
		Service:        use.named.Obj().Name(),
		Packages:       sortedSet(use.pkgs),
		APIMethods:     len(apiMethods),
		CoveredMethods: covered,
		MissingMethods: missing,
	}
}

// apiMethodNames returns the exported methods of a client-go service interface
// whose signature ends in a variadic ...RequestOptionFunc (the REST endpoint
// marker).
func apiMethodNames(named *types.Named) []string {
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	var names []string
	for method := range iface.Methods() {
		if !method.Exported() {
			continue
		}
		if sig, isSig := method.Type().(*types.Signature); isSig && signatureIsAPICall(sig) {
			names = append(names, method.Name())
		}
	}
	sort.Strings(names)
	return names
}

func signatureIsAPICall(sig *types.Signature) bool {
	if !sig.Variadic() || sig.Params().Len() == 0 {
		return false
	}
	last := sig.Params().At(sig.Params().Len() - 1)
	slice, ok := last.Type().(*types.Slice)
	if !ok {
		return false
	}
	named, ok := slice.Elem().(*types.Named)
	if !ok {
		return false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), shared.ClientGoPkgPath) {
		return false
	}
	return strings.Contains(named.Obj().Name(), requestOptionMarker)
}

// clientGoServiceInterface returns the named interface if t is a client-go
// service interface (e.g. BranchesServiceInterface).
func clientGoServiceInterface(t types.Type) (*types.Named, bool) {
	if t == nil {
		return nil, false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, false
	}
	if _, isIface := named.Underlying().(*types.Interface); !isIface {
		return nil, false
	}
	if named.Obj().Pkg() == nil || !strings.Contains(named.Obj().Pkg().Path(), shared.ClientGoPkgPath) {
		return nil, false
	}
	return named, true
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

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func shortPackage(pkgPath string) string {
	_, after, ok := strings.Cut(pkgPath, shared.ToolsPkgInfix)
	if !ok {
		if last := strings.LastIndex(pkgPath, "/"); last >= 0 {
			return pkgPath[last+1:]
		}
		return pkgPath
	}
	return after
}

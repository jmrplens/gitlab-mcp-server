// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actions

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

const (
	coveredGraphQLEpicDsc = "COVERED_GRAPHQL. Epicdiscussions pkg"
	coveredGraphQLEpicNts = "COVERED_GRAPHQL. Epicnotes pkg"
	skipBinaryBytes       = "INTENTIONAL_SKIP_BINARY. File bytes"
	coveredGraphQLEpics   = "COVERED_GRAPHQL. Epics pkg"
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
	slices.SortFunc(services, func(a, b serviceCoverage) int { return strings.Compare(a.Service, b.Service) })

	var staleAcceptances []string
	for _, use := range usage {
		service := use.Service()
		for method := range use.Called {
			key := service + "." + method
			if adjudicated(key) {
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

// acceptedMissingMethods adjudicates SDK service methods that the scanner
// reports as uncovered but which are legitimately handled another way (so they
// are NOT genuine R-ACTION gaps). The scanner sees every selector naming a
// method of a client-go service interface, called in place or handed on as a
// method value, so it misses: RAW REST handlers (client.GL().NewRequest+Do with
// a path string), GraphQL handlers (ADR-0006), and capabilities covered by a
// superseding/generic variant. It also can't know which endpoints are
// INTENTIONALLY not exposed (binary file transfer, CI-job-token self-lookup).
// Each entry carries a category+rationale. Key = "<Service>.<Method>" (service
// name without the ServiceInterface suffix). Methods NOT listed here and still
// uncovered are the genuine new-tool backlog, and an entry whose method a
// handler reaches is reported as stale.
var acceptedMissingMethods = map[string]string{
	// COVERED_VARIANT — an alternative SDK binding for an endpoint another
	// covered method already drives; one action per endpoint is the rule.
	"RepositoryFiles.GetRawFile":      "COVERED_VARIANT. Same raw-file endpoint; gitlab_file_raw calls the streaming GetRawFileReader (client-go v2.58.0) so UPLOAD_MAX_FILE_SIZE is enforced without buffering the blob",
	"DraftNotes.PublishAllDraftNotes": "COVERED_VARIANT. Same POST .../draft_notes/bulk_publish endpoint; mr_review.draft_note_publish_all calls PublishAllDraftNotesWithOptions, the only binding that carries the route's note, internal and reviewer_state",
	"Jobs.CancelJob":                  "COVERED_VARIANT. Same POST /projects/:id/jobs/:job_id/cancel endpoint; job.cancel calls CancelJobWithOptions, the only binding that carries the route's force, with a non-nil options value so a call without force sends {} instead of the null CancelJob sends (entry 51 of docs/development/upstream-bugs.md)",
	"Jobs.GetJobArtifacts":            "COVERED_VARIANT. Same GET /projects/:id/jobs/:job_id/artifacts endpoint; job.artifacts calls GetJobArtifactsWithOptions, the only binding that carries the route's file_type (declared from GitLab 19.4)",
	"GroupMembers.ShareWithGroup":     "COVERED_VARIANT. Same POST /groups/:id/share endpoint; group.group_member_share and group.share_with_group call Groups.ShareGroupWithGroup, the only binding that carries the route's member_role_id and leaves out an expires_at the caller did not set",

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

	// SUPERSEDED_UPSTREAM: the Orbit methods that send the REST routes
	// GitLab.com removed on 2026-10-15, -27 and -28, each replaced by what its
	// action reads now: GraphQL, or the Orbit MCP endpoint, which client-go has
	// no method for and orbit.callMCP posts to through NewRequest (a waits
	// entry, row 150). Not COVERED_GRAPHQL: R-ROUTE carries that decision
	// onto a route only with a GraphQL action of the self-managed catalog,
	// which holds no Orbit action, while GitLab replacing the route carries
	// on its own. Rows 144 to 148 of docs/development/upstream-bugs.md.
	"Orbit.GetStatus":      "SUPERSEDED_UPSTREAM. GetStatus sends GET /orbit/status, which gitlab-org/gitlab!265633 removed in favor of GraphQL Query.orbit.serviceStatus, which orbit.status reads (upstream-bugs row 144)",
	"Orbit.GetSchema":      "SUPERSEDED_UPSTREAM. GetSchema sends GET /orbit/schema, which gitlab-org/gitlab!265633 removed in favor of GraphQL Query.orbit.schema, which orbit.schema reads with get_graph_schema of POST /orbit/mcp for its llm text (upstream-bugs row 145)",
	"Orbit.GetTools":       "SUPERSEDED_UPSTREAM. GetTools sends GET /orbit/tools, which gitlab-org/gitlab!265998 removed in favor of the Orbit MCP endpoint; orbit.tools sends tools/list to POST /orbit/mcp through orbit.callMCP (upstream-bugs rows 146 and 150)",
	"Orbit.GetDsl":         "SUPERSEDED_UPSTREAM. GetDsl sends GET /orbit/schema/dsl, which GitLab.com answers 404 since it read every query as GQL; orbit.dsl sends list_commands to POST /orbit/mcp through orbit.callMCP (upstream-bugs rows 147 and 150)",
	"Orbit.GetGraphStatus": "SUPERSEDED_UPSTREAM. GetGraphStatus sends GET /orbit/graph_status, which gitlab-org/gitlab!264745 removed in favor of GraphQL Group.orbit and Project.orbit, which orbit.graph_status reads (upstream-bugs row 148)",

	// COVERED_RAW is not declared here: a method a raw request, a URL the
	// handler builds or a generic slug dispatch stands in for is replaced by
	// an entry of the waits table, which holds the workaround to the pinned
	// SDK, and coveredRaw derives the adjudication from it.

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
	// This used to read "COVERED_RAW. gitlab_mr_approval_config", and it was
	// the paper trail behind twenty phantom output fields: that action calls
	// the GET at this path and never the POST, but the claim of coverage made
	// the POST's 24-field response look like the shape the action should
	// publish. Set approval requirements through the approval rules instead.
	"MergeRequestApprovals.ChangeApprovalConfiguration": "INTENTIONAL_SKIP_OTHER. Deprecated in GitLab 16.0; approval rules supersede it (gitlab_mr_approval_rule_create/update)",
}

// AcceptedMissingMethods returns the adjudications acceptedMissingMethods
// writes, as a copy a caller may keep, keyed "Service.Method" and each
// opening with its category ("COVERED_GRAPHQL. Epics pkg"). R-ROUTE reads
// them to decide whether a route no action reaches inherits the decision
// about the methods that send it. The COVERED_RAW adjudications are not
// among them: they are derived from the waits table, which a reader takes
// them from with the entry that says why.
func AcceptedMissingMethods() map[string]string { return maps.Clone(acceptedMissingMethods) }

// coveredRawCategory is how a COVERED_RAW adjudication opens, the category
// the rest of acceptedMissingMethods spells its entries with.
const coveredRawCategory = "COVERED_RAW. "

// coveredRaw adjudicates the methods a workaround of the waits table stands
// in for, derived from the table rather than written here a second time: the
// table is where the sdk scope holds each workaround to the pinned SDK and to
// the tree, so the method retires from here the day its entry does.
var coveredRaw = func() map[string]string {
	out := map[string]string{}
	for key, reason := range waits.CoveredRawMethods(waits.Entries()) {
		out[key] = coveredRawCategory + reason
	}
	return out
}()

// adjudicated reports whether either table answers an SDK method the scanner
// reports as uncovered, keyed "Service.Method".
func adjudicated(key string) bool {
	_, accepted := acceptedMissingMethods[key]
	_, replaced := coveredRaw[key]
	return accepted || replaced
}

// isAcceptedMissingMethod reports whether an uncovered SDK method is adjudicated as
// covered-another-way or intentionally not exposed (so not a genuine R-ACTION gap).
func isAcceptedMissingMethod(service, method string) bool {
	return adjudicated(service + "." + method)
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

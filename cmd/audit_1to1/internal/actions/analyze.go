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

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

const (
	coveredGraphQLEpicDsc = "COVERED_GRAPHQL. Epicdiscussions pkg"
	coveredGraphQLEpicNts = "COVERED_GRAPHQL. Epicnotes pkg"
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
	// AdjudicatedTwice lists each method more than one of the three tables
	// answers, with the tables that do, since a method is adjudicated in
	// one place: COVERED_RAW by the waits entry that replaces it,
	// COVERED_GRAPHQL by the KEEP decision that reads in its place, and the
	// rest in acceptedMissingMethods.
	AdjudicatedTwice []string `json:"adjudicated_twice,omitempty"`
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
		AdjudicatedTwice: adjudicatedTwice(),
	}, nil
}

// adjudicatedTwice lists each method more than one table answers, as
// "Service.Method (table, table)" with the tables in the order adjudicated
// reads them, sorted by method.
func adjudicatedTwice() []string {
	tables := []struct {
		name   string
		answer map[string]string
	}{
		{"acceptedMissingMethods", acceptedMissingMethods},
		{"the waits table", coveredRaw},
		{"graphqlDecisions", coveredGraphQL},
	}
	answeredBy := map[string][]string{}
	for _, table := range tables {
		for key := range table.answer {
			answeredBy[key] = append(answeredBy[key], table.name)
		}
	}
	var twice []string
	for key, names := range answeredBy {
		if len(names) > 1 {
			twice = append(twice, key+" ("+strings.Join(names, ", ")+")")
		}
	}
	sort.Strings(twice)
	return twice
}

// acceptedMissingMethods adjudicates SDK service methods that the scanner
// reports as uncovered but which are legitimately handled another way (so they
// are NOT genuine R-ACTION gaps). The scanner sees every selector naming a
// method of a client-go service interface, called in place or handed on as a
// method value, so it misses: RAW REST handlers (client.GL().NewRequest+Do with
// a path string), GraphQL handlers (ADR-0006), and capabilities covered by a
// superseding/generic variant. It also can't know which endpoints a personal
// access token cannot use (a CI job token's self-lookup). Each entry carries
// a category+rationale. Key = "<Service>.<Method>" (service name without the
// ServiceInterface suffix). Methods NOT listed here and still uncovered are
// the genuine new-tool backlog, and an entry whose method a handler reaches
// is reported as stale. A route no action sends is not settled here: a
// download, a route another one covers only generically and a deprecated
// write GitLab still serves are actions to implement, which R-ROUTE tracks,
// and its gate refuses a tracked route whose method this table decides
// COVERED_GENERIC or INTENTIONAL_SKIP_*.
var acceptedMissingMethods = map[string]string{
	// COVERED_VARIANT — an alternative SDK binding for an endpoint another
	// covered method already drives; one action per endpoint is the rule.
	"RepositoryFiles.GetRawFile":      "COVERED_VARIANT. Same raw-file endpoint; gitlab_file_raw calls the streaming GetRawFileReader (client-go v2.58.0) so UPLOAD_MAX_FILE_SIZE is enforced without buffering the blob",
	"DraftNotes.PublishAllDraftNotes": "COVERED_VARIANT. Same POST .../draft_notes/bulk_publish endpoint; mr_review.draft_note_publish_all calls PublishAllDraftNotesWithOptions, the only binding that carries the route's note, internal and reviewer_state",
	"Jobs.CancelJob":                  "COVERED_VARIANT. Same POST /projects/:id/jobs/:job_id/cancel endpoint; job.cancel calls CancelJobWithOptions, the only binding that carries the route's force, with a non-nil options value so a call without force sends {} instead of the null CancelJob sends (entry 51 of docs/development/upstream-bugs.md)",
	"Jobs.GetJobArtifacts":            "COVERED_VARIANT. Same GET /projects/:id/jobs/:job_id/artifacts endpoint; job.artifacts calls GetJobArtifactsWithOptions, the only binding that carries the route's file_type (declared from GitLab 19.4)",
	"GroupMembers.ShareWithGroup":     "COVERED_VARIANT. Same POST /groups/:id/share endpoint; group.group_member_share and group.share_with_group call Groups.ShareGroupWithGroup, the only binding that carries the route's member_role_id and leaves out an expires_at the caller did not set",

	// COVERED_GRAPHQL: ADR-0006 GraphQL handlers (no SDK service call),
	// written here for the epic packages, which the sdk scope's GraphQL rule
	// maps to no service. A method an operation of a package it does map
	// reads in place of is named by that operation's KEEP decision in
	// graphqlDecisions instead, and coveredGraphQL derives it from there.
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
	"PersonalAccessTokens.RotatePersonalAccessTokenByID":          "COVERED_GENERIC. gitlab_personal_access_token_rotate",
	"ExternalStatusChecks.CreateExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.DeleteExternalStatusCheck":              coveredGenericDeprPrj,
	"ExternalStatusChecks.ListMergeStatusChecks":                  "COVERED_GENERIC. Deprecated; ListProjectMergeRequestExternalStatusChecks",
	"ExternalStatusChecks.RetryFailedStatusCheckForAMergeRequest": coveredGenericDeprPrj,
	"ExternalStatusChecks.SetExternalStatusCheckStatus":           "COVERED_GENERIC. Deprecated; SetProjectMergeRequestExternalStatusCheckStatus",
	"ExternalStatusChecks.UpdateExternalStatusCheck":              coveredGenericDeprPrj,
	"ErrorTracking.EnableDisableErrorTracking":                    "COVERED_GENERIC. Deprecated; gitlab_enable_disable_error_tracking",

	// INTENTIONAL_SKIP_OTHER
	"Jobs.GetJobTokensJob": "INTENTIONAL_SKIP_OTHER. CI-job-token self-lookup; not usable with a PAT",
}

// AcceptedMissingMethods returns the adjudications acceptedMissingMethods
// writes, as a copy a caller may keep, keyed "Service.Method" and each
// opening with its category ("COVERED_GRAPHQL. Epics pkg"). R-ROUTE reads
// them to decide whether a route no action reaches inherits the decision
// about the methods that send it. The derived adjudications are not among
// them: the COVERED_RAW ones come from the waits table and the COVERED_GRAPHQL
// ones a KEEP decision names from graphqlDecisions, and a reader takes them
// from there with the entry or the decision that says why.
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

// coveredGraphQLCategory is how a COVERED_GRAPHQL adjudication opens, the
// category acceptedMissingMethods spells the epic methods with.
const coveredGraphQLCategory = "COVERED_GRAPHQL. "

// coveredGraphQL adjudicates the methods a KEEP decision of the sdk scope's
// graphqlDecisions reads in place of, derived from the decision rather than
// written here a second time: the decision is where the sdk scope holds the
// operation to the tree and the method to the pinned SDK, so the method
// returns to this stream's list the day its decision retires.
var coveredGraphQL = func() map[string]string {
	out := map[string]string{}
	for key, reason := range sdk.CoveredGraphQLMethods() {
		out[key] = coveredGraphQLCategory + reason
	}
	return out
}()

// adjudicated reports whether any of the three tables answers an SDK method
// the scanner reports as uncovered, keyed "Service.Method".
func adjudicated(key string) bool {
	_, accepted := acceptedMissingMethods[key]
	_, replaced := coveredRaw[key]
	_, kept := coveredGraphQL[key]
	return accepted || replaced || kept
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

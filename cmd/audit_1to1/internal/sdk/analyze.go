// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/enums"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// Service and operation statuses.
const (
	statusCovered       = "covered"
	statusDeclared      = "declared"
	statusUndeclared    = "undeclared"
	statusAdjudicated   = "adjudicated"
	statusUnadjudicated = "unadjudicated"
)

type report struct {
	SchemaVersion     int                `json:"schema_version"`
	ClientGoPath      string             `json:"client_go_path"`
	Summary           reportSummary      `json:"summary"`
	Services          []sdkService       `json:"services"`
	GraphQLOperations []graphqlOperation `json:"graphql_operations"`
	// EnumFields lists the fields of an SDK enum type the actions expose,
	// each held against the SDK's constant set (R-ENUM).
	EnumFields []enums.Finding `json:"enum_fields"`
	// OptionStructs holds every client-go struct a request a handler sends
	// carries to an input, and every copy of one struct into another to the
	// fields the two share.
	OptionStructs optionStructsReport `json:"option_structs"`
	// UpstreamWaits holds every workaround waiting on client-go to the
	// predicate that says it is still needed, and every raw request,
	// request option written by hand and body edit the walk finds to an
	// entry that answers it.
	UpstreamWaits upstreamWaitsReport `json:"upstream_waits"`
	// Captures holds every struct a tool package decodes off a captured
	// response to the client-go structs decoded from the same response,
	// every key both read to a declaration that says why it is read twice,
	// and every such struct paired with none to why it has no SDK half.
	Captures capturesReport `json:"captures"`
	// StaleDeclarations joins the six tables' stale entries: a service
	// declaration, a GraphQL decision, an enum exemption, an option struct
	// declaration, an upstream wait or a capture declaration that no longer
	// describes the code.
	StaleDeclarations []string `json:"stale_declarations,omitempty"`
}

type reportSummary struct {
	SDKServices             int `json:"sdk_services"`
	ServicesCovered         int `json:"services_covered"`
	ServicesDeclared        int `json:"services_declared"`
	ServicesUndeclared      int `json:"services_undeclared"`
	GraphQLOperations       int `json:"graphql_operations"`
	GraphQLAdjudicated      int `json:"graphql_adjudicated"`
	GraphQLUnadjudicated    int `json:"graphql_unadjudicated"`
	EnumFields              int `json:"enum_fields"`
	EnumFieldsWithGaps      int `json:"enum_fields_with_gaps"`
	EnumMissingValues       int `json:"enum_missing_values"`
	EnumExtraValues         int `json:"enum_extra_values"`
	OptionStructs           int `json:"option_structs"`
	OptionPaired            int `json:"option_structs_paired"`
	OptionConverted         int `json:"option_structs_converted"`
	OptionCopied            int `json:"option_structs_copied"`
	OptionDeclared          int `json:"option_structs_declared"`
	OptionUnpaired          int `json:"option_structs_unpaired"`
	OptionCopyMissing       int `json:"option_copy_missing_fields"`
	OptionNestedMissing     int `json:"option_nested_missing_fields"`
	UpstreamWaits           int `json:"upstream_waits"`
	WaitsHolding            int `json:"upstream_waits_holding"`
	WaitsStalePredicate     int `json:"upstream_waits_stale_predicate"`
	WaitsObsolete           int `json:"upstream_waits_obsolete"`
	Workarounds             int `json:"workarounds"`
	WorkaroundsUndeclared   int `json:"workarounds_undeclared"`
	BodyEditKeysUnanswered  int `json:"body_edit_keys_unanswered"`
	BodyEditsUnread         int `json:"body_edits_unread"`
	SupersetNeedsUnanswered int `json:"superset_needs_unanswered"`
	OptionMethodsUnanswered int `json:"option_methods_unanswered"`
	CapturedTypes           int `json:"captured_types"`
	CapturedKeysShared      int `json:"captured_keys_shared"`
	CapturesKept            int `json:"captures_kept"`
	CapturesRedundant       int `json:"captures_redundant"`
	CapturesStalePredicate  int `json:"captures_stale_predicate"`
	CapturesObsolete        int `json:"captures_obsolete"`
	CapturesAnswerNoStruct  int `json:"captures_answer_no_struct"`
	CapturesUnpaired        int `json:"captures_unpaired"`
	StaleDeclarations       int `json:"stale_declarations"`
}

// clean reports whether the audited tree has no finding, i.e. whether the gate
// passes.
func (s reportSummary) clean() bool {
	return s.ServicesUndeclared == 0 && s.GraphQLUnadjudicated == 0 &&
		s.EnumMissingValues == 0 && s.EnumExtraValues == 0 &&
		s.OptionUnpaired == 0 && s.OptionCopyMissing == 0 && s.OptionNestedMissing == 0 &&
		s.WaitsStalePredicate == 0 && s.WaitsObsolete == 0 && s.WorkaroundsUndeclared == 0 && s.BodyEditKeysUnanswered == 0 &&
		s.BodyEditsUnread == 0 && s.SupersetNeedsUnanswered == 0 && s.OptionMethodsUnanswered == 0 &&
		s.CapturesRedundant == 0 && s.CapturesStalePredicate == 0 && s.CapturesObsolete == 0 && s.CapturesUnpaired == 0 &&
		s.StaleDeclarations == 0
}

// Seams for the failures the real tree cannot produce: the JSON encoder never
// fails on a report of strings and ints, the service enumeration cannot fail
// once the root package resolved (resolving it is what checks the Client
// struct), and the enum rule loads the same packages this scope already
// loaded. Each is a variable so a test can reach the branch behind it.
var (
	marshalIndent = json.MarshalIndent
	sdkServices   = collectSDKServices
	analyzeEnums  = enums.Analyze
)

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline) together with the gate outcome.
// gapsOnly keeps only the findings, matching the other scopes' flag.
func Run(root string, gapsOnly bool) (content []byte, clean bool, err error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(rep, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), rep.Summary.clean(), nil
}

func buildReport(root string, gapsOnly bool) (report, error) {
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		return report{}, err
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		return report{}, err
	}
	declared, err := sdkServices(clientGo)
	if err != nil {
		return report{}, err
	}
	// A service a workaround replaces whole is declared COVERED_RAW by the
	// entry, which is where its reason lives.
	entries := waits.Entries()
	serviceDeclarations, declaredTwice := withCoveredRawServices(declaredServices, entries)
	usage := shared.CollectServiceUsage(pkgs)
	services := adjudicateServices(declared, usage, serviceDeclarations)

	graphQL, err := shared.GraphQLInterface(clientGo)
	if err != nil {
		return report{}, err
	}
	operations := buildGraphQLOperations(collectGraphQLSites(pkgs, graphQL), serviceIndex(services), graphqlServiceAliases, graphqlDecisions)

	// The enum rule loads the same packages (memoized per root) and reads
	// the catalog compiled into this binary; with gapsOnly it already keeps
	// only the fields with a gap.
	enumRep, err := analyzeEnums(root, gapsOnly)
	if err != nil {
		return report{}, err
	}

	// The option structs are held to the inputs the field diff pairs them
	// with, read from the same packages the structs scope reads.
	options, optionStale := buildOptionStructs(collectOptionUniverse(pkgs), structs.DiffInputPairs(pkgs), optionStructDeclarations)

	upstream, waitsStale, err := analyzeUpstreamWaits(root, pkgs, clientGo, usage, entries)
	if err != nil {
		return report{}, err
	}

	captures, capturesStale := analyzeCaptures(pkgs, clientGo, captureDeclarations)

	stale := mergeStale(staleServiceDeclarations(services, serviceDeclarations), declaredTwice, staleGraphQLDecisions(operations, graphqlDecisions), enumRep.StaleExemptions, optionStale, waitsStale, capturesStale)
	summary := summarize(services, operations, enumRep.Summary, stale)
	summarizeOptionStructs(&summary, options)
	summarizeUpstreamWaits(&summary, upstream)
	summarizeCaptures(&summary, captures)

	if gapsOnly {
		services = keepUndeclaredServices(services)
		operations = keepUnadjudicatedOperations(operations)
		options = keepOptionFindings(options)
		upstream = keepWaitFindings(upstream)
		captures = keepCaptureFindings(captures)
	}
	return report{
		SchemaVersion:     shared.SchemaVersion,
		ClientGoPath:      shared.ClientGoPkgPath,
		Summary:           summary,
		Services:          services,
		GraphQLOperations: operations,
		EnumFields:        flattenEnumFindings(enumRep),
		OptionStructs:     options,
		UpstreamWaits:     upstream,
		Captures:          captures,
		StaleDeclarations: stale,
	}, nil
}

// flattenEnumFindings lists the enum report's findings in one slice, package
// order preserved, so the gate's report reads field by field.
func flattenEnumFindings(rep enums.Report) []enums.Finding {
	fields := make([]enums.Finding, 0)
	for _, pkg := range rep.Packages {
		fields = append(fields, pkg.Findings...)
	}
	return fields
}

// mergeStale joins the stale lists into one sorted list, so the report reads
// as a single roll of claims that no longer hold rather than three.
func mergeStale(lists ...[]string) []string {
	stale := make([]string, 0)
	for _, list := range lists {
		stale = append(stale, list...)
	}
	sort.Strings(stale)
	return stale
}

func keepUndeclaredServices(services []sdkService) []sdkService {
	out := make([]sdkService, 0)
	for _, service := range services {
		if service.Status == statusUndeclared {
			out = append(out, service)
		}
	}
	return out
}

func keepUnadjudicatedOperations(operations []graphqlOperation) []graphqlOperation {
	out := make([]graphqlOperation, 0)
	for _, op := range operations {
		if op.Status == statusUnadjudicated {
			out = append(out, op)
		}
	}
	return out
}

func summarize(services []sdkService, operations []graphqlOperation, enumSummary enums.Summary, stale []string) reportSummary {
	s := reportSummary{
		SDKServices:        len(services),
		EnumFields:         enumSummary.Fields,
		EnumFieldsWithGaps: enumSummary.FieldsWithGaps,
		EnumMissingValues:  enumSummary.MissingValues,
		EnumExtraValues:    enumSummary.ExtraValues,
		StaleDeclarations:  len(stale),
	}
	for _, service := range services {
		switch service.Status {
		case statusCovered:
			s.ServicesCovered++
		case statusDeclared:
			s.ServicesDeclared++
		default:
			s.ServicesUndeclared++
		}
	}
	s.GraphQLOperations = len(operations)
	for _, op := range operations {
		if op.Status == statusAdjudicated {
			s.GraphQLAdjudicated++
			continue
		}
		s.GraphQLUnadjudicated++
	}
	return s
}

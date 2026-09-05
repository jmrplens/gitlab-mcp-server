package sdk

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/enums"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
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
	// StaleDeclarations joins the three tables' stale entries: a service
	// declaration, a GraphQL decision or an enum exemption that no longer
	// describes the code.
	StaleDeclarations []string `json:"stale_declarations,omitempty"`
}

type reportSummary struct {
	SDKServices          int `json:"sdk_services"`
	ServicesCovered      int `json:"services_covered"`
	ServicesDeclared     int `json:"services_declared"`
	ServicesUndeclared   int `json:"services_undeclared"`
	GraphQLOperations    int `json:"graphql_operations"`
	GraphQLAdjudicated   int `json:"graphql_adjudicated"`
	GraphQLUnadjudicated int `json:"graphql_unadjudicated"`
	EnumFields           int `json:"enum_fields"`
	EnumFieldsWithGaps   int `json:"enum_fields_with_gaps"`
	EnumMissingValues    int `json:"enum_missing_values"`
	EnumExtraValues      int `json:"enum_extra_values"`
	StaleDeclarations    int `json:"stale_declarations"`
}

// clean reports whether the audited tree has no finding, i.e. whether the gate
// passes.
func (s reportSummary) clean() bool {
	return s.ServicesUndeclared == 0 && s.GraphQLUnadjudicated == 0 &&
		s.EnumMissingValues == 0 && s.EnumExtraValues == 0 && s.StaleDeclarations == 0
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
	services := adjudicateServices(declared, shared.CollectServiceUsage(pkgs), declaredServices)

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

	stale := mergeStale(staleServiceDeclarations(services, declaredServices), staleGraphQLDecisions(operations, graphqlDecisions), enumRep.StaleExemptions)
	summary := summarize(services, operations, enumRep.Summary, stale)

	if gapsOnly {
		services = keepUndeclaredServices(services)
		operations = keepUnadjudicatedOperations(operations)
	}
	return report{
		SchemaVersion:     shared.SchemaVersion,
		ClientGoPath:      shared.ClientGoPkgPath,
		Summary:           summary,
		Services:          services,
		GraphQLOperations: operations,
		EnumFields:        flattenEnumFindings(enumRep),
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

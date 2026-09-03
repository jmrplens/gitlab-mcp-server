// Package sdk holds the two 1:1 audit rules whose universe comes from the SDK
// rather than from this repository's own call sites (R-SERVICE, R-GRAPHQL).
//
// The action-coverage scope answers "which methods of a service we call are
// uncovered". It cannot answer "is there a service we call nothing on", because
// its universe is built by walking our call expressions: a service nothing
// references never enters the map, so an entire upstream service can be added
// and the audit still report zero gaps. That is exactly what happened when
// WorkItemSavedViewsService arrived with seven methods.
//
// So this scope enumerates the services from the client-go Client struct and
// holds each one to a decision: covered by a call, or declared an exception
// with a category and a reason. A service that is neither is a finding.
//
// It carries a second rule for the same reason. ADR-0006 admits raw
// GraphQL.Do() for domains WITHOUT a client-go service wrapper. The wrapper
// appearing later is what retires that exemption, and nothing was checking, so
// the exemption was permanent in practice. Every raw GraphQL operation whose
// package maps to a client-go service is therefore held to a decision too,
// per operation rather than per package: packages that use GraphQL for one
// operation and the SDK for the rest are the norm, so a package-level verdict
// would be mostly noise.
//
// Unlike the three candidate-backlog scopes, this one is a gate: Run reports
// whether the tree is clean, and the command exits non-zero when it is not.
package sdk

import (
	"encoding/json"
	"fmt"
	"sort"

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
	StaleDeclarations []string           `json:"stale_declarations,omitempty"`
}

type reportSummary struct {
	SDKServices          int `json:"sdk_services"`
	ServicesCovered      int `json:"services_covered"`
	ServicesDeclared     int `json:"services_declared"`
	ServicesUndeclared   int `json:"services_undeclared"`
	GraphQLOperations    int `json:"graphql_operations"`
	GraphQLAdjudicated   int `json:"graphql_adjudicated"`
	GraphQLUnadjudicated int `json:"graphql_unadjudicated"`
	StaleDeclarations    int `json:"stale_declarations"`
}

// clean reports whether the audited tree has no finding, i.e. whether the gate
// passes.
func (s reportSummary) clean() bool {
	return s.ServicesUndeclared == 0 && s.GraphQLUnadjudicated == 0 && s.StaleDeclarations == 0
}

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline) together with the gate outcome.
// gapsOnly keeps only the findings, matching the other scopes' flag.
func Run(root string, gapsOnly bool) (content []byte, clean bool, err error) {
	rep, err := buildReport(root, gapsOnly)
	if err != nil {
		return nil, false, err
	}
	content, err = json.MarshalIndent(rep, "", "  ")
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
	declared, err := collectSDKServices(clientGo)
	if err != nil {
		return report{}, err
	}
	services := adjudicateServices(declared, shared.CollectServiceUsage(pkgs), declaredServices)

	graphQL, err := shared.GraphQLInterface(clientGo)
	if err != nil {
		return report{}, err
	}
	operations := buildGraphQLOperations(collectGraphQLSites(pkgs, graphQL), serviceIndex(services), graphqlServiceAliases, graphqlDecisions)

	stale := mergeStale(staleServiceDeclarations(services, declaredServices), staleGraphQLDecisions(operations, graphqlDecisions))
	summary := summarize(services, operations, stale)

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
		StaleDeclarations: stale,
	}, nil
}

// mergeStale joins the two stale lists into one sorted list, so the report
// reads as a single roll of claims that no longer hold rather than two.
func mergeStale(services, operations []string) []string {
	stale := make([]string, 0, len(services)+len(operations))
	stale = append(stale, services...)
	stale = append(stale, operations...)
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

func summarize(services []sdkService, operations []graphqlOperation, stale []string) reportSummary {
	s := reportSummary{SDKServices: len(services), StaleDeclarations: len(stale)}
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

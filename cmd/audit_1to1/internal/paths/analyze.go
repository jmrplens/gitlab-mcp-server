package paths

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// Report is this scope's native JSON shape.
type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Inventory     string  `json:"inventory"`
	Summary       Summary `json:"summary"`
	// GraphQLRefusals are the documents the pinned schema will not accept.
	GraphQLRefusals []GraphQLRefusal `json:"graphql_refusals"`
	// SilentOwners are the packages the catalog owns actions in that were seen
	// issuing nothing, each with its status.
	SilentOwners []SilentOwner `json:"silent_owners"`
	// StaleDeclarations are the claims about a silent package that no longer
	// describe the tree.
	StaleDeclarations []string `json:"stale_declarations,omitempty"`
	// Endpoints is the documentation comparison, which is a candidate list and
	// never a gate.
	Endpoints EndpointCheck `json:"endpoints"`
}

// Summary is the count of everything the report holds.
type Summary struct {
	InventoryRows     int `json:"inventory_rows"`
	GraphQLDocuments  int `json:"graphql_documents"`
	GraphQLRefused    int `json:"graphql_refused"`
	CatalogActions    int `json:"catalog_actions"`
	ActionsObserved   int `json:"actions_observed"`
	ActionsSilent     int `json:"actions_silent"`
	ActionsUnmapped   int `json:"actions_unmapped"`
	SilentPackages    int `json:"silent_packages"`
	UndeclaredSilent  int `json:"undeclared_silent_packages"`
	UndeclaredActions int `json:"undeclared_silent_actions"`
	StaleDeclarations int `json:"stale_declarations"`
	// UndocumentedEndpoints is 0 when the documentation comparison did not run,
	// which Endpoints.Ran is what tells the two apart.
	UndocumentedEndpoints int `json:"undocumented_endpoints"`
}

// clean reports whether the audited tree has no finding, which is to say
// whether the gate passes.
//
// The documentation comparison is deliberately absent: it is a candidate list,
// for the reasons in [EndpointCheck].
func (s Summary) clean() bool {
	return s.GraphQLRefused == 0 && s.UndeclaredSilent == 0 && s.StaleDeclarations == 0
}

// GraphQLRefusal is one document the schema will not accept.
type GraphQLRefusal struct {
	Package  string   `json:"package"`
	Document string   `json:"document"`
	Position string   `json:"position"`
	Reasons  []string `json:"reasons"`
}

// Seams for the failures the real tree cannot produce: the JSON encoder never
// fails on a report of strings and ints, and the two inputs are compiled into
// this binary or committed beside it. Each is a variable a test restores.
var (
	marshalIndent  = json.MarshalIndent
	readInventory  = requestinventory.Read
	catalogActions = requestinventory.Actions
	auditGraphQL   = graphqldocs.Audit
	auditEndpoints = checkEndpoints
)

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline) together with the gate outcome.
//
// A nil fetcher leaves the documentation comparison out, which is the default:
// it needs the network and two minutes of it on a cold cache, and it gates
// nothing, so a run that only wants the gate should not pay for it.
//
// gapsOnly keeps only the findings, matching the other scopes' flag.
func Run(ctx context.Context, root string, gapsOnly bool, fetcher *apidocs.Fetcher) (content []byte, clean bool, err error) {
	report, err := buildReport(ctx, root, gapsOnly, fetcher)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(report, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), report.Summary.clean(), nil
}

func buildReport(ctx context.Context, root string, gapsOnly bool, fetcher *apidocs.Fetcher) (Report, error) {
	inventory, err := readInventory(root)
	if err != nil {
		return Report{}, err
	}
	actions, err := catalogActions()
	if err != nil {
		return Report{}, fmt.Errorf("build action catalog: %w", err)
	}

	documents, err := auditGraphQL(graphqldocs.Options{Dir: root})
	if err != nil {
		return Report{}, fmt.Errorf("read the GraphQL documents: %w", err)
	}

	coverage, owners := observed(root, inventory.Requests, actions)
	stale := staleDeclarations(owners)
	undeclaredPackages, undeclaredActions := undeclaredSilent(owners)

	endpoints := EndpointCheck{}
	if fetcher != nil {
		if endpoints, err = auditEndpoints(ctx, fetcher, inventory.Requests); err != nil {
			return Report{}, fmt.Errorf("compare the recorded endpoints with the API documentation: %w", err)
		}
	}

	report := Report{
		SchemaVersion:     shared.SchemaVersion,
		Inventory:         requestinventory.Path,
		GraphQLRefusals:   refusals(documents),
		SilentOwners:      owners,
		StaleDeclarations: stale,
		Endpoints:         endpoints,
		Summary: Summary{
			InventoryRows:         len(inventory.Requests),
			GraphQLDocuments:      len(documents.Documents),
			GraphQLRefused:        len(documents.Refusals),
			CatalogActions:        coverage.Total,
			ActionsObserved:       coverage.Covered,
			ActionsSilent:         coverage.Silent,
			ActionsUnmapped:       coverage.Unmapped,
			SilentPackages:        len(coverage.SilentOwners),
			UndeclaredSilent:      undeclaredPackages,
			UndeclaredActions:     undeclaredActions,
			StaleDeclarations:     len(stale),
			UndocumentedEndpoints: len(endpoints.Undocumented),
		},
	}
	if gapsOnly {
		report.SilentOwners = keepUndeclared(report.SilentOwners)
	}
	return report, nil
}

// refusals renders the refused documents as the report lists them.
//
// The position stays absolute, as the loader gives it. A text gate can trim it
// against the root it was pointed at, which is what cmd/audit_graphql_documents
// does; a JSON report is read by whoever opens the file, and a path that has
// been made relative to somebody else's checkout is worse than a long one.
func refusals(result graphqldocs.Result) []GraphQLRefusal {
	found := make([]GraphQLRefusal, 0, len(result.Refusals))
	for _, refusal := range result.Refusals {
		found = append(found, GraphQLRefusal{
			Package:  refusal.Document.Package,
			Document: refusal.Document.Label(),
			Position: refusal.Document.Position.String(),
			Reasons:  refusal.Reasons,
		})
	}
	return found
}

// keepUndeclared drops the owners that are not findings, which is what
// -gaps-only asks for: a declared silence and an unmapped owner are both
// context rather than work.
func keepUndeclared(owners []SilentOwner) []SilentOwner {
	kept := make([]SilentOwner, 0)
	for _, owner := range owners {
		if owner.Status == statusUndeclared {
			kept = append(kept, owner)
		}
	}
	return kept
}

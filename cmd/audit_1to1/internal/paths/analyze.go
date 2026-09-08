package paths

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
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
	// Shapes is what GitLab's own OpenAPI document says the endpoints we call
	// return, compared with what we publish. A report and not a gate, for the
	// reason [ShapeCheck] records.
	Shapes ShapeCheck `json:"shapes"`
}

// Summary is the count of everything the report holds.
type Summary struct {
	InventoryRows    int `json:"inventory_rows"`
	GraphQLDocuments int `json:"graphql_documents"`
	GraphQLRefused   int `json:"graphql_refused"`
	CatalogActions   int `json:"catalog_actions"`
	// ActionsObserved counts the actions whose owning package was seen issuing
	// some request, which is not the same as the action's own request having
	// been seen: the grain is the package, because nothing on the wire names
	// an action. Grain says so beside the number, since the number is what a
	// reader quotes.
	ActionsObserved   int    `json:"actions_observed"`
	Grain             string `json:"actions_observed_grain"`
	ActionsSilent     int    `json:"actions_silent"`
	ActionsUnmapped   int    `json:"actions_unmapped"`
	SilentPackages    int    `json:"silent_packages"`
	UndeclaredSilent  int    `json:"undeclared_silent_packages"`
	UndeclaredActions int    `json:"undeclared_silent_actions"`
	StaleDeclarations int    `json:"stale_declarations"`
	// UndocumentedEndpoints is 0 when the documentation comparison did not run,
	// which Endpoints.Ran is what tells the two apart.
	UndocumentedEndpoints int `json:"undocumented_endpoints"`
	// UndeclaredEndpoints counts the undocumented ones no declaration in
	// endpoint_declarations.go accounts for, which is the half that gates.
	UndeclaredEndpoints int `json:"undeclared_endpoints"`
	// UntemplatedSegments counts the distinct literal path segments the
	// inventory carries where GitLab's document has a placeholder. It measures
	// this repository's own recording rather than the server: every one is a
	// fixture value the templating did not recognize as an identifier.
	UntemplatedSegments int `json:"untemplated_segments"`
	// UnpublishedFields counts the output fields no endpoint of their package
	// declares. A lower bound, for the reason unpublishedFields records.
	UnpublishedFields int `json:"unpublished_fields"`
	// UnsurfacedFields counts the response fields GitLab's document lists for
	// a package's endpoints that no output type of the package publishes,
	// split by what the conditions record says: sent by every GitLab, or
	// only when a condition holds. The unknown remainder is the difference.
	// A lower bound at the same grain, for the reason [SentCheck] records.
	UnsurfacedFields int `json:"unsurfaced_fields"`
	UnsurfacedAlways int `json:"unsurfaced_sent_always"`
	UnsurfacedWhen   int `json:"unsurfaced_sent_when"`
	// UnsurfacedDeclared counts the findings a declaration in
	// sent_declarations.go accounts for: fields the document lists and the
	// endpoint does not send. They stay in the three counts above, since
	// those say what the conditions record answered, and this says how many
	// of them a reader need not act on.
	UnsurfacedDeclared int `json:"unsurfaced_declared"`
	// The typed counts are the same comparison held at type grain, where an
	// output type is judged only against the endpoints its client-go struct
	// models. They are published beside the package-grain count rather than
	// instead of it, so a reader can see how much of that number the sharper
	// join keeps. See [TypedShapeCheck].
	TypedCompared         int `json:"typed_types_compared"`
	TypedNoPairing        int `json:"typed_types_without_pairing"`
	TypedNoRoute          int `json:"typed_types_without_route"`
	TypedNoSchema         int `json:"typed_types_without_schema"`
	TypedUnpublishedField int `json:"typed_unpublished_fields"`
	// TypedUndeclaredFields counts the typed findings no declaration in
	// shape_declarations.go accounts for, which is the half a reader is being
	// asked to act on. It spans both levels, so a run whose top-level findings
	// are all declared still reports the nested ones that are not; read it
	// against TypedUnpublishedField and TypedNestedUnpublished to see which
	// level the number is coming from.
	TypedUndeclaredFields int `json:"typed_undeclared_fields"`
	// TypedNestedCompared and TypedNestedUnpublished are the same comparison
	// one level down: a nested output type held against the properties GitLab's
	// document gives the object it sits under. See [TypedShapeCheck.Nested].
	TypedNestedCompared    int `json:"typed_nested_types_compared"`
	TypedNestedUnpublished int `json:"typed_nested_unpublished_fields"`
	// TypedUnsurfacedFields counts the reverse findings at type grain: the
	// response fields the operations a type models declare that the type does
	// not publish, split the same way the package-grain count is. See
	// [TypedShapeCheck.Unsurfaced].
	TypedUnsurfacedFields   int `json:"typed_unsurfaced_fields"`
	TypedUnsurfacedAlways   int `json:"typed_unsurfaced_sent_always"`
	TypedUnsurfacedWhen     int `json:"typed_unsurfaced_sent_when"`
	TypedUnsurfacedDeclared int `json:"typed_unsurfaced_declared"`
}

// observedGrain is what [Summary.Grain] says, spelled once.
const observedGrain = "package: an action counts as observed when the package that owns it issued some request, not when its own request was seen"

// clean reports whether the audited tree has no finding, which is to say
// whether the gate passes.
//
// An unmapped action counts, because an owner that names no package under
// internal/tools is a hole in this gate rather than a curiosity: the catalog
// fills a spec group's missing owner in with "tools", nothing checks that an
// owner names a real package, and an action classified unmapped is one the
// silence check can never fail. Leaving it out meant the gate could be
// silenced by omitting a field.
//
// The documentation comparison is deliberately absent from the gate unless it
// found something undeclared: it is a candidate list, for the reasons in
// [EndpointCheck].
func (s Summary) clean() bool {
	return s.GraphQLRefused == 0 &&
		s.UndeclaredSilent == 0 &&
		s.StaleDeclarations == 0 &&
		s.ActionsUnmapped == 0 &&
		s.UndeclaredEndpoints == 0
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

	shapes := shapeCheck(root, inventory.Requests, publishedTypes(root))
	sentAlways, sentWhen, sentDeclared := unsurfacedCounts(shapes.Sent.Unsurfaced)
	typedSentAlways, typedSentWhen, typedSentDeclared := unsurfacedCounts(shapes.Typed.Unsurfaced)

	stale = append(stale, endpoints.staleDeclarations()...)
	stale = append(stale, shapes.Typed.staleDeclarations()...)
	stale = append(stale, shapes.Sent.staleDeclarations()...)
	sort.Strings(stale)

	report := Report{
		SchemaVersion:     shared.SchemaVersion,
		Inventory:         requestinventory.Path,
		GraphQLRefusals:   refusals(documents),
		SilentOwners:      owners,
		StaleDeclarations: stale,
		Endpoints:         endpoints,
		Shapes:            shapes,
		Summary: Summary{
			InventoryRows:           len(inventory.Requests),
			GraphQLDocuments:        len(documents.Documents),
			GraphQLRefused:          len(documents.Refusals),
			CatalogActions:          coverage.Total,
			ActionsObserved:         coverage.Covered,
			Grain:                   observedGrain,
			ActionsSilent:           coverage.Silent,
			ActionsUnmapped:         coverage.Unmapped,
			SilentPackages:          len(coverage.SilentOwners),
			UndeclaredSilent:        undeclaredPackages,
			UndeclaredActions:       undeclaredActions,
			StaleDeclarations:       len(stale),
			UndocumentedEndpoints:   len(endpoints.Undocumented),
			UndeclaredEndpoints:     endpoints.undeclared(),
			UntemplatedSegments:     len(shapes.Untemplated),
			UnpublishedFields:       len(shapes.Unpublished),
			UnsurfacedFields:        len(shapes.Sent.Unsurfaced),
			UnsurfacedAlways:        sentAlways,
			UnsurfacedWhen:          sentWhen,
			UnsurfacedDeclared:      sentDeclared,
			TypedUnsurfacedFields:   len(shapes.Typed.Unsurfaced),
			TypedUnsurfacedAlways:   typedSentAlways,
			TypedUnsurfacedWhen:     typedSentWhen,
			TypedUnsurfacedDeclared: typedSentDeclared,
			TypedCompared:           shapes.Typed.Compared,
			TypedNoPairing:          shapes.Typed.SkippedNoPairing,
			TypedNoRoute:            shapes.Typed.SkippedNoRoute,
			TypedNoSchema:           shapes.Typed.SkippedNoSchema,
			TypedUnpublishedField:   len(shapes.Typed.Unpublished),
			TypedUndeclaredFields:   shapes.Typed.undeclared(),
			TypedNestedCompared:     shapes.Typed.NestedCompared,
			TypedNestedUnpublished:  len(shapes.Typed.Nested),
		},
	}
	if gapsOnly {
		report.SilentOwners = keepUndeclared(report.SilentOwners)
		report.Endpoints.Undocumented = keepUndeclaredEndpoints(report.Endpoints.Undocumented)
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
// -gaps-only asks for: a declared silence is context rather than work.
//
// An unmapped owner stays, because it is a finding: the catalog names a
// package that is not one, so nothing could have recorded a request for those
// actions and no declaration excuses it. It used to be filtered out here, and
// with the gate ignoring it too, an action whose spec group forgot to name an
// owner was neither counted nor printed.
func keepUndeclared(owners []SilentOwner) []SilentOwner {
	kept := make([]SilentOwner, 0)
	for _, owner := range owners {
		if owner.Status != statusDeclared {
			kept = append(kept, owner)
		}
	}
	return kept
}

// keepUndeclaredEndpoints drops the undocumented endpoints a declaration
// accounts for, on the same terms.
func keepUndeclaredEndpoints(found []Endpoint) []Endpoint {
	kept := make([]Endpoint, 0)
	for _, endpoint := range found {
		if !endpoint.declared() {
			kept = append(kept, endpoint)
		}
	}
	return kept
}

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
	// Pagination is whether an action that hands a model a list also hands it
	// the way to ask for the rest of it. A report and not a gate, for the reason
	// [PaginationCheck] records.
	Pagination PaginationCheck `json:"pagination"`
	// SDKGraphQL is the same "does the document validate" question asked of the
	// documents client-go builds inside its own module, which nothing here
	// could see before. A report and not a gate, for the reason
	// [SDKGraphQLCheck] records.
	SDKGraphQL SDKGraphQLCheck `json:"sdk_graphql"`
	// AlwaysSent is the first of the two checks here that read values rather
	// than names: a param the SDK writes on every call that GitLab lets a
	// caller leave out. A report and not a gate, for the reason
	// [AlwaysSentCheck] records.
	AlwaysSent AlwaysSentCheck `json:"always_sent"`
	// Identifiers is the other: a placeholder the recorder only ever saw one
	// value behind, which is where a hard-coded identifier can hide. A report
	// and not a gate, for the reason [IdentifierCheck] records.
	Identifiers IdentifierCheck `json:"identifier_values"`
	// E2E is what a recorded end-to-end run says about which actions were seen
	// issuing a request, which is the one grain finer than the package the
	// observation check above is held at. Empty unless a shard directory was
	// named, and a report rather than a gate either way; see [E2EObservation].
	E2E E2EObservation `json:"e2e_observation"`
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
	// TypedUnsurfacedNotInSDK counts how many of those fields client-go's own
	// struct does not carry either, which is the half whose answer is an
	// upstream merge request rather than an edit here. The remainder is a
	// field the SDK already gives us and only this server does not publish.
	TypedUnsurfacedNotInSDK int `json:"typed_unsurfaced_not_in_sdk"`
	// The pagination counts are R-PAGE, the one question here that is about a
	// response header rather than a field: PaginatedRoutes and KeysetRoutes are
	// what the record says GitLab pages at all, and the collection counts are
	// how the actions reading a list divide against them. See
	// [PaginationCheck].
	PaginatedRoutes        int `json:"paginated_routes"`
	KeysetRoutes           int `json:"keyset_routes"`
	CollectionActions      int `json:"collection_actions"`
	CollectionsPaginated   int `json:"collection_actions_publishing_pagination"`
	CollectionsUnasked     int `json:"collection_actions_not_asked_about"`
	CollectionsUnpaginated int `json:"collection_actions_without_pagination"`
	// CollectionsUndeclared is the half of those no declaration accounts for,
	// which is what a reader is asked to act on.
	CollectionsUndeclared int `json:"collection_actions_without_pagination_undeclared"`
	// PaginationGrain says what the collection counts were asked at, for the
	// same reason Grain does above: the number is what gets quoted.
	PaginationGrain string `json:"pagination_endpoint_grain"`
	// SDKGraphQLDocuments and SDKGraphQLRefused count the documents client-go
	// builds inside its own module and the ones the pinned schema will not
	// accept. They are beside GraphQLDocuments and GraphQLRefused rather than
	// folded into them, because a refusal there is a merge request against
	// somebody else's repository and one here is an edit; see
	// [SDKGraphQLCheck].
	SDKGraphQLDocuments int `json:"sdk_graphql_documents"`
	SDKGraphQLRefused   int `json:"sdk_graphql_refused"`
	// The end-to-end counts are the observation question asked per action
	// instead of per package, and they are present only when a shard directory
	// was named. Observed is the claim a dropped span cannot invent; Silent is
	// the lead. See [E2EObservation].
	E2EActionsObserved int `json:"e2e_actions_issuing_requests"`
	E2EActionsSilent   int `json:"e2e_actions_that_ran_and_issued_nothing"`
	// The always-sent counts are the first of the two value questions: how many
	// params the SDK writes unconditionally, how many of those GitLab requires
	// anyway, and the remainder a caller cannot decline to send. See
	// [AlwaysSentCheck].
	AlwaysSentFields   int `json:"always_sent_fields"`
	AlwaysSentRequired int `json:"always_sent_required_by_gitlab"`
	AlwaysSentOptional int `json:"always_sent_but_optional"`
	// The identifier counts are the other: how many placeholder positions the
	// recording reached, and how many of them it only ever stood one value
	// behind. Zero on an inventory recorded before the counts existed, which
	// Identifiers.Ran is what tells apart. See [IdentifierCheck].
	IdentifierPlaceholders int `json:"identifier_placeholders"`
	IdentifierSingleValued int `json:"identifier_placeholders_with_one_value"`
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
//
// So are the two shape comparisons and the pagination check, each for its own
// recorded reason. What their declaration tables do reach the gate through is
// StaleDeclarations: a finding is a candidate and a claim that has stopped being
// true is not.
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

// Options is what a run of this scope needs beyond the tree.
//
// It is a struct rather than three parameters because two of the three are
// inputs the run may not have: an API-doc fetcher costs the network, and an
// end-to-end record exists only after a Docker session wrote one. Naming them
// at the call site is what keeps "not asked for" and "asked for and empty"
// from being the same argument.
type Options struct {
	// GapsOnly keeps only the findings, matching the other scopes' flag.
	GapsOnly bool
	// Fetcher enables the endpoint comparison. Nil leaves it out, which is the
	// default: it needs the network and two minutes of it on a cold cache, and
	// it gates nothing, so a run that only wants the gate should not pay for
	// it.
	Fetcher *apidocs.Fetcher
	// E2ECallsDir is the shard directory an end-to-end run recorded its calls
	// into, which lets the observation question be asked per action rather than
	// per package. Empty leaves that out, which is every run outside a Docker
	// session; see [E2EObservation].
	E2ECallsDir string
}

// Run builds the report for the given repository root and returns it as
// indented JSON (with a trailing newline) together with the gate outcome.
func Run(ctx context.Context, root string, opts Options) (content []byte, clean bool, err error) {
	report, err := buildReport(ctx, root, opts)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(report, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), report.Summary.clean(), nil
}

func buildReport(ctx context.Context, root string, opts Options) (Report, error) {
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
	if opts.Fetcher != nil {
		if endpoints, err = auditEndpoints(ctx, opts.Fetcher, inventory.Requests); err != nil {
			return Report{}, fmt.Errorf("compare the recorded endpoints with the API documentation: %w", err)
		}
	}

	shapes := shapeCheck(root, inventory.Requests, publishedTypes(root))
	sentAlways, sentWhen, sentDeclared := unsurfacedCounts(shapes.Sent.Unsurfaced)
	typedSentAlways, typedSentWhen, typedSentDeclared := unsurfacedCounts(shapes.Typed.Unsurfaced)
	pagination := paginationCheck(root, inventory.Requests, actions)
	sdkGraphQL := sdkGraphQLCheck(root)
	alwaysSent := alwaysSentCheck(root, inventory.Requests)
	identifiers := identifierCheck(inventory.Requests)
	e2e := e2eObservation(opts.E2ECallsDir, actions)

	stale = append(stale, endpoints.staleDeclarations()...)
	stale = append(stale, shapes.Typed.staleDeclarations()...)
	stale = append(stale, shapes.Sent.staleDeclarations()...)
	stale = append(stale, pagination.staleDeclarations()...)
	sort.Strings(stale)

	report := Report{
		SchemaVersion:     shared.SchemaVersion,
		Inventory:         requestinventory.Path,
		GraphQLRefusals:   refusals(documents),
		SilentOwners:      owners,
		StaleDeclarations: stale,
		Endpoints:         endpoints,
		Shapes:            shapes,
		Pagination:        pagination,
		SDKGraphQL:        sdkGraphQL,
		AlwaysSent:        alwaysSent,
		Identifiers:       identifiers,
		E2E:               e2e,
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
			TypedUnsurfacedNotInSDK: notModelledBySDK(shapes.Typed.Unsurfaced),
			TypedCompared:           shapes.Typed.Compared,
			TypedNoPairing:          shapes.Typed.SkippedNoPairing,
			TypedNoRoute:            shapes.Typed.SkippedNoRoute,
			TypedNoSchema:           shapes.Typed.SkippedNoSchema,
			TypedUnpublishedField:   len(shapes.Typed.Unpublished),
			TypedUndeclaredFields:   shapes.Typed.undeclared(),
			TypedNestedCompared:     shapes.Typed.NestedCompared,
			TypedNestedUnpublished:  len(shapes.Typed.Nested),
			PaginatedRoutes:         pagination.Routes.Offset,
			KeysetRoutes:            pagination.Routes.Keyset,
			CollectionActions:       pagination.Collections.Actions,
			CollectionsPaginated:    pagination.Collections.Paginated,
			CollectionsUnasked:      pagination.Collections.Unasked,
			CollectionsUnpaginated:  pagination.Collections.Unpaginated,
			CollectionsUndeclared:   pagination.Collections.Undeclared,
			PaginationGrain:         paginationGrain,
			SDKGraphQLDocuments:     sdkGraphQL.Documents,
			SDKGraphQLRefused:       len(sdkGraphQL.Refusals),
			E2EActionsObserved:      len(e2e.Issuing),
			E2EActionsSilent:        len(e2e.Silent),
			AlwaysSentFields:        alwaysSent.Fields,
			AlwaysSentRequired:      alwaysSent.Required,
			AlwaysSentOptional:      len(alwaysSent.Optional),
			IdentifierPlaceholders:  identifiers.Placeholders,
			IdentifierSingleValued:  identifiers.Single,
		},
	}
	if opts.GapsOnly {
		report.SilentOwners = keepUndeclared(report.SilentOwners)
		report.Endpoints.Undocumented = keepUndeclaredEndpoints(report.Endpoints.Undocumented)
		report.Pagination.Unpaginated = keepUndeclaredCollections(report.Pagination.Unpaginated)
		// The SDK document listing and the per-action observation keep only
		// their findings too: the document set and the observed actions are
		// context, and -gaps-only asks for the work.
		report.SDKGraphQL.Templates = nil
		report.E2E.Issuing = nil
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

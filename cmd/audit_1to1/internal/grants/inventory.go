package grants

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// InventoryCheck holds the derivation to what the unit suite was seen sending,
// at the grain both can be joined on: the package. It reports and never
// gates, for the reason R-PATH's observation check gives about the same
// record. The inventory names the package that built the client and never the
// action, so a row reached here is reached by some action of that package and
// not necessarily by the one a reader has in mind; and a row the derivation
// does not reach is either a request no handler sends (a test exercising a
// helper directly) or one the derivation missed, which only a reader can tell
// apart.
type InventoryCheck struct {
	Grain string `json:"grain"`
	// RowsOutsideOwners counts the rows recorded by a package that owns no
	// catalog action (the completions, the elicitation flows, the test
	// support), which the derivation has nothing to say about.
	RowsOutsideOwners int `json:"rows_outside_owning_packages"`
	// The row counts are the inventory's REST and GraphQL rows under an
	// owning package, and how many of them an action of that package derives.
	RESTRows           int `json:"rest_rows"`
	RESTRowsDerived    int `json:"rest_rows_derived"`
	GraphQLRows        int `json:"graphql_rows"`
	GraphQLRowsDerived int `json:"graphql_rows_derived"`
	// The derived counts are the distinct requests each owning package's
	// actions derive, and how many of them the inventory recorded under that
	// package.
	DerivedREST            int `json:"derived_rest_requests"`
	DerivedRESTRecorded    int `json:"derived_rest_requests_recorded"`
	DerivedGraphQL         int `json:"derived_graphql_requests"`
	DerivedGraphQLRecorded int `json:"derived_graphql_requests_recorded"`
	// NotDerived are the rows no action of their package derives, and
	// NotRecorded the derived requests no row of their package records. Both
	// are leads.
	NotDerived  []RowLead     `json:"rows_not_derived,omitempty"`
	NotRecorded []RequestLead `json:"derived_not_recorded,omitempty"`
}

// RowLead is one inventory row the derivation does not reach.
type RowLead struct {
	Package string `json:"package"`
	Request string `json:"request"`
	Why     string `json:"why"`
}

// RequestLead is one derived request no row of its package records.
type RequestLead struct {
	Package string   `json:"package"`
	Request string   `json:"request"`
	Actions []string `json:"actions"`
}

// inventoryGrain is what [InventoryCheck.Grain] says, spelled once.
const inventoryGrain = "package: a recorded row counts as derived when an action its package owns derives the same route or GraphQL operation; nothing on the wire names an action"

// The reasons a row is not derived.
const (
	whyNoRoute    = "no route of the live record meets it"
	whyNotDerived = "no action the package owns derives it"
)

// packageRequests are the requests one owning package's actions derive.
type packageRequests struct {
	// keys are the requests by [requestKey], with the actions behind each.
	keys map[string][]string
	// declared indexes the derivation's own spelling of each route a
	// declaration placed (a HEAD GitLab answers from its GET, a slug route
	// GitLab mounts once per value), so a recorded row is placed by the
	// rule that placed the request and not only by the route the record
	// holds for it; byDeclared maps each spelling back onto its key.
	declared   []apilive.Route
	index      *apilive.RouteIndex
	byDeclared map[string]string
}

// derivedRequests are, per owning package as the inventory spells it, what
// its actions derive.
type derivedRequests map[string]*packageRequests

// place finds the key a REST row is derived under through a declared
// spelling, a placeholder of which may stand for the row's literal.
func (requests *packageRequests) place(row requestinventory.Row) (string, bool) {
	if requests == nil {
		return "", false
	}
	route, ok := requests.index.Match(row.Method, row.Path)
	if !ok {
		return "", false
	}
	return requests.byDeclared[apilive.RouteName(route)], true
}

// derives reports whether the package derives a request under key.
func (requests *packageRequests) derives(key string) bool {
	return requests != nil && requests.keys[key] != nil
}

// inventoryCheck joins the inventory's rows to the derived requests of the
// package that recorded them, in both directions.
func inventoryCheck(rows []requestinventory.Row, record actionrequests.Record, live *apilive.Document, owners map[string]string) InventoryCheck {
	derived := derivedByPackage(record, owners)
	check := InventoryCheck{Grain: inventoryGrain}
	seen := check.joinRows(rows, derived, apilive.NewRouteIndex(live.Routes), owners)
	check.countDerived(derived, seen)
	slices.SortFunc(check.NotDerived, func(a, b RowLead) int {
		return strings.Compare(a.Package+" "+a.Request, b.Package+" "+b.Request)
	})
	slices.SortFunc(check.NotRecorded, func(a, b RequestLead) int {
		return strings.Compare(a.Package+" "+a.Request, b.Package+" "+b.Request)
	})
	return check
}

// joinRows counts the inventory's rows and joins each one under an owning
// package to the key its package derives it under, returning the keys each
// package was seen sending.
func (check *InventoryCheck) joinRows(rows []requestinventory.Row, derived derivedRequests, index *apilive.RouteIndex, owners map[string]string) map[string]map[string]bool {
	owning := map[string]bool{}
	for _, owner := range owners {
		owning[requestinventory.PackageName(owner)] = true
	}
	seen := map[string]map[string]bool{}
	for _, row := range rows {
		if !owning[row.Package] {
			check.RowsOutsideOwners++
			continue
		}
		graphQL := row.Kind == requestinventory.KindGraphQL
		if graphQL {
			check.GraphQLRows++
		} else {
			check.RESTRows++
		}
		key, request, why := derived[row.Package].join(row, index)
		if why != "" {
			check.NotDerived = append(check.NotDerived, RowLead{Package: row.Package, Request: request, Why: why})
			continue
		}
		if graphQL {
			check.GraphQLRowsDerived++
		} else {
			check.RESTRowsDerived++
		}
		if seen[row.Package] == nil {
			seen[row.Package] = map[string]bool{}
		}
		seen[row.Package][key] = true
	}
	return seen
}

// countDerived counts the requests each owning package derives, how many of
// them a row of that package recorded, and lists the rest as leads.
func (check *InventoryCheck) countDerived(derived derivedRequests, seen map[string]map[string]bool) {
	for pkg, requests := range derived {
		for key, actions := range requests.keys {
			recorded := seen[pkg][key]
			// A GraphQL key begins with the operation kind where a route
			// begins with its verb.
			if graphQLOperation(key) {
				check.DerivedGraphQL++
				if recorded {
					check.DerivedGraphQLRecorded++
				}
			} else {
				check.DerivedREST++
				if recorded {
					check.DerivedRESTRecorded++
				}
			}
			if !recorded {
				check.NotRecorded = append(check.NotRecorded, RequestLead{Package: pkg, Request: key, Actions: actions})
			}
		}
	}
}

// join finds the key a row of the package is derived under, with the row as
// a reader finds it in the inventory and the reason it is not derived when it
// is not. A REST row no derived route meets is tried once more through the
// derivation's own spelling of the routes a declaration placed.
func (requests *packageRequests) join(row requestinventory.Row, index *apilive.RouteIndex) (key, request, why string) {
	key, request, why = rowKey(row, index)
	if requests.derives(key) {
		return key, request, ""
	}
	if row.Kind != requestinventory.KindGraphQL {
		if placed, ok := requests.place(row); ok {
			return placed, request, ""
		}
	}
	if why == "" {
		why = whyNotDerived
	}
	return key, request, why
}

// derivedByPackage collects, per owning package, the requests its actions
// derive. An unresolved request has no key to join on, and an action the
// catalog does not name has no package.
func derivedByPackage(record actionrequests.Record, owners map[string]string) derivedRequests {
	derived := derivedRequests{}
	for _, action := range record.Actions {
		owner, ok := owners[action.ID]
		if !ok {
			continue
		}
		pkg := requestinventory.PackageName(owner)
		for i := range action.Requests {
			request := &action.Requests[i]
			key := requestKey(request)
			if key == "" {
				continue
			}
			requests := derived[pkg]
			if requests == nil {
				requests = &packageRequests{keys: map[string][]string{}, byDeclared: map[string]string{}}
				derived[pkg] = requests
			}
			requests.keys[key] = appendOnce(requests.keys[key], action.ID)
			if method, path, spelled := strings.Cut(request.Derived, " "); spelled {
				declared := apilive.Route{Method: method, Path: apilive.EndpointPrefix + path}
				requests.declared = append(requests.declared, declared)
				requests.byDeclared[apilive.RouteName(&declared)] = key
			}
		}
	}
	for _, requests := range derived {
		for key := range requests.keys {
			sort.Strings(requests.keys[key])
		}
		requests.index = apilive.NewRouteIndex(requests.declared)
	}
	return derived
}

// requestKey is what a derived request is joined on: a REST request's route
// as the live record names it, a GraphQL one's operation kind and root
// fields, and nothing for an unresolved one.
func requestKey(request *actionrequests.RecordRequest) string {
	switch request.Kind {
	case actionrequests.KindREST:
		return request.Route
	case actionrequests.KindGraphQL:
		kind, _, _ := strings.Cut(request.Operation, " ")
		return operationKey(kind, request.RootFields)
	default:
		return ""
	}
}

// rowKey is what an inventory row is joined on, the way [requestKey] spells
// it, with the row as a reader finds it in the inventory and the reason it
// cannot be joined when it cannot. A REST row is placed among the live
// record's routes by the rule the derivation places its own requests by,
// since the two spell a route's parameters differently and a recorded path
// may carry a literal where the route has a placeholder.
func rowKey(row requestinventory.Row, index *apilive.RouteIndex) (key, request, why string) {
	if row.Kind == requestinventory.KindGraphQL {
		kind, roots := parseOperationLabel(row.Operation)
		return operationKey(kind, roots), row.Operation, ""
	}
	request = row.Method + " " + row.Path
	route, ok := index.Match(row.Method, row.Path)
	if !ok {
		return "", request, whyNoRoute
	}
	return apilive.RouteName(route), request, ""
}

// parseOperationLabel reads the label the test transport records a GraphQL
// request under (internal/testutil's operationLabel): the operation kind,
// then its name when it has one, then its root fields joined by commas. The
// root fields are therefore the last word, whether or not a name stands
// between them and the kind.
func parseOperationLabel(label string) (kind string, roots []string) {
	words := strings.Fields(label)
	if len(words) < 2 {
		return label, nil
	}
	return words[0], strings.Split(words[len(words)-1], ",")
}

// operationKey spells a GraphQL operation by its kind and its root fields in
// order, which is what tells two of this server's documents apart: almost
// every one is anonymous, and the derivation names its operations after the
// document's constant rather than the name the document declares.
func operationKey(kind string, roots []string) string {
	sorted := slices.Clone(roots)
	sort.Strings(sorted)
	return kind + " " + strings.Join(sorted, ",")
}

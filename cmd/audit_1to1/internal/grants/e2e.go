package grants

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// E2ECheck holds the derivation to what an end-to-end run saw each action
// send, which is the one grain finer than the package: the harness stamps a
// trace into each call, the server's span names the action the dispatcher
// ran, and every GitLab request the handler made is a client span of that
// trace (e2ecalls.Dispatch.Requests), carrying the route of the server's
// table it reached (e2ecalls.Dispatch.Routes).
//
// It asks two questions. The count is a floor: the spans travel through a
// batching processor that drops silently when its queue overflows. The number
// a count is held to is the fewest requests any recorded way of running the
// action makes, its mandatory requests and the alternatives that way takes,
// since a trace ran one of those ways and sent at least what it makes. So a
// count at or above that number is consistent, and one below it is a lead,
// either an over-approximation of the derivation or a dropped span. The
// routes are the other direction and a positive claim: a route a trace saw
// sent was sent, so one the derivation does not name for the action is a
// lead that the derivation under-approximates, which is the one error that
// would let a grant check withhold too little. A route the table does not
// name at all arrives as its method alone and is a lead of its own, since
// nothing in the table can be judged against it. It reports and never gates,
// for the reasons R-PATH's end-to-end observation gives.
type E2ECheck struct {
	// Ran is whether a record was read at all; Error says why not, when one
	// was asked for and could not be read.
	Ran       bool   `json:"ran"`
	Directory string `json:"directory,omitempty"`
	Error     string `json:"error,omitempty"`
	Grain     string `json:"grain,omitempty"`
	// Compared counts the actions of the request record a dispatch ran and
	// the server did not decline, and Consistent those no lead below names.
	Compared   int `json:"actions_compared"`
	Consistent int `json:"actions_consistent"`
	// RoutesCompared counts the compared actions at least one of whose
	// dispatch lines named the routes its trace reached; a line written
	// before the harness recorded them is held to its count alone.
	RoutesCompared int `json:"actions_route_compared"`
	// FewerThanAnyPath are actions whose busiest trace carried fewer
	// requests than any recorded way of running them makes.
	FewerThanAnyPath []CountLead `json:"fewer_requests_than_any_path,omitempty"`
	// SentWhereNoneDerived are actions the derivation says send nothing that
	// a trace saw sending.
	SentWhereNoneDerived []CountLead `json:"requests_where_none_derived,omitempty"`
	// RoutesNotDerived are routes a trace of an action reached that the
	// derivation does not name for it.
	RoutesNotDerived []RouteLead `json:"routes_not_derived,omitempty"`
	// RoutesTheTableDoesNotName are requests whose route the server's table
	// holds no template for, by method, on actions whose derivation places
	// no route of that method by a pattern.
	RoutesTheTableDoesNotName []RouteLead `json:"routes_the_table_does_not_name,omitempty"`
	// NotInRecord are dispatched ids the request record does not hold, named
	// so a run read against a record it no longer describes says so.
	NotInRecord []string `json:"dispatched_ids_not_in_the_record,omitempty"`
}

// CountLead is one action whose observed request count disagrees with the
// derivation.
type CountLead struct {
	Action string `json:"action"`
	// Fewest is the fewest requests any recorded way of running the action
	// makes, and Observed the most one trace of the action carried.
	Fewest   int `json:"fewest_on_a_path"`
	Observed int `json:"observed"`
}

// RouteLead is one route a trace of an action reached that the derivation
// cannot account for: the method and template without the API prefix, the
// GraphQL endpoint as its own path, or the method alone for a route the
// server's table holds no template for.
type RouteLead struct {
	Action string `json:"action"`
	Route  string `json:"route"`
}

// e2eGrain is what [E2ECheck.Grain] says, spelled once.
const e2eGrain = "action: by count, consistent when the most requests one trace of the action carried is at least the fewest any recorded way of running it makes, which cannot say which routes were sent; and by route, where the dispatch line names them, every route a trace reached is one the derivation names for the action"

// graphQLRoute is how a dispatch line names a request to the GraphQL
// endpoint, which the table holds as its own path rather than as a template.
const graphQLRoute = "POST /api/graphql"

// observation is what the dispatch lines of one action said.
type observation struct {
	// requests is the most requests one trace carried.
	requests int
	// routed is whether any line named its routes.
	routed bool
	// routes is every route a line named, deduplicated.
	routes map[string]bool
}

// e2eCheck folds the dispatch lines of a recorded run into the per-action
// comparison. An empty directory is not an error: no end-to-end record was
// offered, which is every run outside a Docker session.
func e2eCheck(dir string, record actionrequests.Record) E2ECheck {
	if dir == "" {
		return E2ECheck{}
	}
	check := E2ECheck{Ran: true, Directory: dir, Grain: e2eGrain}
	records, err := readE2ECalls(dir)
	if err != nil {
		check.Error = fmt.Sprintf("read the end-to-end call record: %v", err)
		return check
	}
	actions := map[string]actionrequests.RecordAction{}
	for _, action := range record.Actions {
		actions[action.ID] = action
	}
	observed, unknown := observe(records, actions)
	check.NotInRecord = unknown
	ids := make([]string, 0, len(observed))
	for id := range observed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		check.Compared++
		action, seen := actions[id], observed[id]
		if seen.routed {
			check.RoutesCompared++
		}
		if !check.judge(action, seen) {
			check.Consistent++
		}
	}
	return check
}

// judge files every lead one action's observation raises, and reports whether
// it raised any.
func (check *E2ECheck) judge(action actionrequests.RecordAction, seen *observation) bool {
	lead := CountLead{Action: action.ID, Fewest: fewestRequests(action.Paths), Observed: seen.requests}
	led := false
	switch {
	case len(action.Requests) == 0 && lead.Observed > 0:
		check.SentWhereNoneDerived = append(check.SentWhereNoneDerived, lead)
		led = true
	case lead.Observed < lead.Fewest:
		check.FewerThanAnyPath = append(check.FewerThanAnyPath, lead)
		led = true
	}
	routes := make([]string, 0, len(seen.routes))
	for route := range seen.routes {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		switch accountFor(action, route) {
		case routeAccounted:
		case routeUnnamed:
			check.RoutesTheTableDoesNotName = append(check.RoutesTheTableDoesNotName, RouteLead{Action: action.ID, Route: route})
			led = true
		default:
			check.RoutesNotDerived = append(check.RoutesNotDerived, RouteLead{Action: action.ID, Route: route})
			led = true
		}
	}
	return led
}

// routeVerdict is what the derivation makes of one observed route.
type routeVerdict int

// The verdicts.
const (
	// routeNotDerived is a route the derivation does not name for the action.
	routeNotDerived routeVerdict = iota
	// routeAccounted is a route the derivation names, or one a declared
	// pattern covers.
	routeAccounted
	// routeUnnamed is a request whose route the table holds no template for.
	routeUnnamed
)

// accountFor judges one observed route against an action's derivation.
//
// A REST route is accounted for by a request whose route is it, or by a
// declaration's pattern covering it, where ":" stands for any one segment:
// the slug routes of the integrations, where the record holds one slug for
// a family, and the HEAD a client-go method sends to a route Grape declares
// for GET. The GraphQL endpoint is accounted for by any GraphQL request, and
// so is anything an unresolved request could have sent, since the walk could
// not read what that one is. A method alone is a route the table does not
// name, accounted for only where a declared pattern of that method says the
// derivation placed the route by a pattern rather than by the table.
func accountFor(action actionrequests.RecordAction, route string) routeVerdict {
	unnamed := !strings.Contains(route, " ")
	for _, request := range action.Requests {
		switch request.Kind {
		case actionrequests.KindUnresolved:
			return routeAccounted
		case actionrequests.KindGraphQL:
			if route == graphQLRoute {
				return routeAccounted
			}
		default:
			if unnamed {
				if request.Derived != "" && strings.HasPrefix(request.Derived, route+" ") {
					return routeAccounted
				}
				continue
			}
			if request.Route == route || (request.Derived != "" && patternCovers(request.Derived, route)) {
				return routeAccounted
			}
		}
	}
	if unnamed {
		return routeUnnamed
	}
	return routeNotDerived
}

// patternCovers reports whether a declared pattern ("PUT /projects/:/x/:")
// covers an observed route ("PUT /projects/:id/x/slack"): the same method,
// the same number of segments, and every segment the pattern spells equal to
// the route's, a bare ":" taking any one.
func patternCovers(pattern, route string) bool {
	patternMethod, patternPath, _ := strings.Cut(pattern, " ")
	routeMethod, routePath, _ := strings.Cut(route, " ")
	if patternMethod != routeMethod {
		return false
	}
	want := strings.Split(patternPath, "/")
	got := strings.Split(routePath, "/")
	if len(want) != len(got) {
		return false
	}
	for i, segment := range want {
		if segment != ":" && segment != got[i] {
			return false
		}
	}
	return true
}

// fewestRequests is the fewest requests any recorded way of running an action
// makes. A path holds the action's mandatory requests and the alternatives
// that way takes, and an optional request is on none, so it is never counted;
// an action with no recorded path is held to none.
func fewestRequests(paths [][]int) int {
	if len(paths) == 0 {
		return 0
	}
	fewest := len(paths[0])
	for _, path := range paths[1:] {
		fewest = min(fewest, len(path))
	}
	return fewest
}

// observe is, per action of the record a dispatch ran and the server did not
// decline, the most requests one of its dispatch lines carried and every
// route they named, with the dispatched ids the record does not hold. A trace
// written twice is read twice and changes nothing, since only the highest
// count is kept and the routes are a set; a declined dispatch is passed over,
// because the server declining to run an action is not a handler that sent
// less.
func observe(records []e2ecalls.Record, known map[string]actionrequests.RecordAction) (observed map[string]*observation, unknown []string) {
	observed = map[string]*observation{}
	for _, record := range records {
		dispatch := record.Dispatch
		if record.Type != e2ecalls.TypeDispatch || dispatch == nil || dispatch.Action == "" || dispatch.RefusalReason != "" {
			continue
		}
		if _, ok := known[dispatch.Action]; !ok {
			if !slices.Contains(unknown, dispatch.Action) {
				unknown = append(unknown, dispatch.Action)
			}
			continue
		}
		seen := observed[dispatch.Action]
		if seen == nil {
			seen = &observation{routes: map[string]bool{}}
			observed[dispatch.Action] = seen
		}
		seen.requests = max(seen.requests, dispatch.Requests)
		if len(dispatch.Routes) > 0 {
			seen.routed = true
		}
		for _, route := range dispatch.Routes {
			seen.routes[route] = true
		}
	}
	sort.Strings(unknown)
	return observed, unknown
}

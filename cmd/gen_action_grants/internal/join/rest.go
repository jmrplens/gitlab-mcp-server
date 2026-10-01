package join

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// requirement is one group before it is indexed: raw permission names held at
// one boundary type among any.
type requirement struct {
	perms []string
	any   finegrained.Boundary
}

// key identifies a requirement among the table's.
func (r requirement) key() string {
	return strings.Join(r.perms, ",") + "@" + r.any.String()
}

// routeIndex finds the record's route for a derived one.
type routeIndex struct {
	// byMethod maps a verb to every spelling of every route with that verb.
	byMethod map[string][]spelled
}

// spelled is one spelling of a record route: its path with optional segments
// taken or left out, and identifiers collapsed.
type spelled struct {
	segments []string
	route    *apilive.Route
}

// apiPrefix is what every route of the REST API is mounted under.
const apiPrefix = "/api/:version"

// newRouteIndex indexes the record's routes.
func newRouteIndex(routes []apilive.Route) *routeIndex {
	index := &routeIndex{byMethod: map[string][]spelled{}}
	for i := range routes {
		route := &routes[i]
		path, mounted := strings.CutPrefix(route.Path, apiPrefix)
		if !mounted {
			continue
		}
		for _, variant := range expandOptional(path) {
			index.byMethod[route.Method] = append(index.byMethod[route.Method], spelled{segments: segmentsOf(variant), route: route})
		}
	}
	return index
}

// expandOptional spells a Grape path with each optional group, written in
// parentheses, taken and left out: `:id/(-/)epics` is both `:id/epics` and
// `:id/-/epics`. An escaped parenthesis is a literal, not a group.
func expandOptional(path string) []string {
	open := -1
	for i := 0; i < len(path); i++ {
		if path[i] == '\\' {
			i++
			continue
		}
		if path[i] == '(' {
			open = i
			break
		}
	}
	if open < 0 {
		return []string{path}
	}
	depth := 0
	for end := open; end < len(path); end++ {
		switch path[end] {
		case '\\':
			end++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				inner := path[open+1 : end]
				rest := path[end+1:]
				var out []string
				for _, tail := range expandOptional(rest) {
					out = append(out, path[:open]+tail)
					for _, middle := range expandOptional(inner) {
						out = append(out, path[:open]+middle+tail)
					}
				}
				return out
			}
		}
	}
	return []string{path}
}

// segmentsOf splits a path into segments, each identifier or splat collapsed
// to the placeholder.
func segmentsOf(path string) []string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			segments[i] = ":"
		}
	}
	return segments
}

// match finds the record route a derived route is: the one spelling whose
// segments equal the derived ones, a record placeholder standing for a
// derived literal where it must; the closest, by literal segments in common,
// when several do. It reports false for a route the record lacks.
func (index *routeIndex) match(method, path string) (*apilive.Route, bool) {
	derived := segmentsOf(path)
	var best *apilive.Route
	bestScore := -1
	for _, candidate := range index.byMethod[method] {
		score, ok := agree(candidate.segments, derived)
		if ok && score > bestScore {
			best, bestScore = candidate.route, score
		}
	}
	return best, best != nil
}

// agree reports whether a record spelling meets a derived path, and how many
// literal segments they share.
func agree(record, derived []string) (int, bool) {
	if len(record) != len(derived) {
		return 0, false
	}
	score := 0
	for i := range record {
		switch {
		case record[i] == derived[i]:
			if record[i] != ":" {
				score++
			}
		case record[i] == ":":
		default:
			return 0, false
		}
	}
	return score, true
}

// liveName spells a record route the way the table names it: the verb and the
// path under the API prefix, parameters named as GitLab names them.
func liveName(route *apilive.Route) string {
	return route.Method + " " + strings.TrimPrefix(route.Path, apiPrefix)
}

// restRequirements reads what a route demands: its groups, whether the grant
// decides it at all, and why no fine-grained token passes it when none does.
func restRequirements(route *apilive.Route) (groups []requirement, skip bool, denied finegrained.Cause) {
	switch route.FineGrained() {
	case apilive.RouteSkipped:
		return nil, true, ""
	case apilive.RouteTodo:
		return nil, false, finegrained.CauseRESTTodo
	case apilive.RouteUndeclared:
		return nil, false, finegrained.CauseRESTUndeclared
	}
	auth := route.Authorization
	groups = append(groups, requirement{perms: sorted(auth.Permissions), any: primaryBoundary(auth)})
	for _, scope := range auth.AdditionalScopes {
		any := boundaryOf(scope.BoundaryType)
		if scope.Boundary != nil || any == 0 {
			any = finegrained.AllBoundaries
		}
		groups = append(groups, requirement{perms: sorted(scope.Permissions), any: any})
	}
	return groups, false, ""
}

// primaryBoundary reads the boundary types a route's primary requirement may
// be held at. A callable boundary wins over everything else and is resolved
// per request, so it is read as the types the route declares beside it, or
// all four when it declares none.
func primaryBoundary(auth *apilive.RouteAuthorization) finegrained.Boundary {
	declared := boundaryOf(auth.BoundaryType)
	for _, alternative := range auth.Boundaries {
		declared |= boundaryOf(alternative.BoundaryType)
		if alternative.Boundary != nil && alternative.BoundaryType == "" {
			return finegrained.AllBoundaries
		}
	}
	if declared == 0 {
		return finegrained.AllBoundaries
	}
	return declared
}

// boundaryOf reads one boundary type, nothing for an empty or unknown one.
func boundaryOf(name string) finegrained.Boundary {
	boundary, _ := finegrained.ParseBoundary(name)
	return boundary
}

// sorted returns a sorted copy of names, which is the order GitLab compares a
// GraphQL requirement group's permissions in and the one a table keys by.
func sorted(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}

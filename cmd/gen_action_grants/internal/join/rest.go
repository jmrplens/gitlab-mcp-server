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

// Where a derived route is found among the record's and what the table calls
// it once found are apilive's to answer ([apilive.RouteIndex] and
// [apilive.RouteName]), since R-GRANT places the routes the unit suite
// recorded by the same rule and the two must agree on which route a request
// is.

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
		boundaries := boundaryOf(scope.BoundaryType)
		if scope.Boundary != nil || boundaries == 0 {
			boundaries = finegrained.AllBoundaries
		}
		groups = append(groups, requirement{perms: sorted(scope.Permissions), any: boundaries})
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
